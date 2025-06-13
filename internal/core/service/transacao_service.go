package service

import (
	"context"
	"errors"
	"itau/authorizer/internal/core/domain"
	"time"
)

type TransacaoService struct {
	limiteRepository    domain.LimiteRepository
	transacaoRepository domain.TransacaoRepository
	eventPublisher      domain.EventPublisher
	metricsCollector    domain.MetricsCollector
	tracer              domain.DistributedTracer
	logger              domain.Logger
}

func NewTransacaoService(
	limiteRepository domain.LimiteRepository,
	transacaoRepository domain.TransacaoRepository,
	eventPublisher domain.EventPublisher,
	metricsCollector domain.MetricsCollector,
	tracer domain.DistributedTracer,
	logger domain.Logger,
) *TransacaoService {
	return &TransacaoService{
		limiteRepository:    limiteRepository,
		transacaoRepository: transacaoRepository,
		eventPublisher:      eventPublisher,
		metricsCollector:    metricsCollector,
		tracer:              tracer,
		logger:              logger,
	}
}

// AutorizarTransacao implementa a lógica principal de autorização
// com observabilidade completa e gestão de eventos assíncronos
func (s *TransacaoService) AutorizarTransacao(ctx context.Context, transacao *domain.Transacao) error {
	startTime := time.Now()

	// Inicia span de tracing distribuído
	ctx, span := s.tracer.StartSpan(ctx, "TransacaoService.AutorizarTransacao")
	defer func() {
		// Registra latência da operação
		duration := time.Since(startTime).Seconds()
		s.metricsCollector.RecordTransactionLatency(duration)
		s.tracer.FinishSpan(span, nil)
	}()

	s.tracer.AddTag(span, "cliente_id", transacao.ClienteID)
	s.tracer.AddTag(span, "valor", transacao.Valor)
	s.tracer.AddTag(span, "correlation_id", transacao.CorrelationID)

	s.logger.Info(ctx, "iniciando autorização de transação", map[string]interface{}{
		"transacao_id":   transacao.ID,
		"cliente_id":     transacao.ClienteID,
		"valor":          transacao.Valor,
		"correlation_id": transacao.CorrelationID,
	})

	// 1. Validação de negócio
	if err := s.validarTransacao(ctx, transacao); err != nil {
		return s.rejeitarTransacao(ctx, transacao, err)
	}

	// 2. Verificação e débito atômico do limite
	if err := s.processarLimite(ctx, transacao); err != nil {
		return s.rejeitarTransacao(ctx, transacao, err)
	}

	// 3. Aprovação da transação
	return s.aprovarTransacao(ctx, transacao)
}

func (s *TransacaoService) validarTransacao(ctx context.Context, transacao *domain.Transacao) error {
	ctx, span := s.tracer.StartSpan(ctx, "TransacaoService.validarTransacao")
	defer s.tracer.FinishSpan(span, nil)

	if err := transacao.Valida(); err != nil {
		s.logger.Warn(ctx, "validação de transação falhou", map[string]interface{}{
			"transacao_id": transacao.ID,
			"erro":         err.Error(),
		})

		s.metricsCollector.IncrementErrorCounter("validation_error")
		return err
	}

	return nil
}

func (s *TransacaoService) processarLimite(ctx context.Context, transacao *domain.Transacao) error {
	ctx, span := s.tracer.StartSpan(ctx, "TransacaoService.processarLimite")
	defer s.tracer.FinishSpan(span, nil)

	// Converte para centavos para evitar problemas de ponto flutuante
	valorCentavos := int(transacao.Valor * 100)

	// Operação atômica: verifica limite E debita em uma única operação
	// Isso previne race conditions usando conditional writes do DynamoDB
	err := s.limiteRepository.DebitarLimiteAtomica(ctx, transacao.ClienteID, valorCentavos)
	if err != nil {
		if errors.Is(err, domain.ErrLimiteInsuficiente) {
			s.logger.Warn(ctx, "limite insuficiente", map[string]interface{}{
				"transacao_id": transacao.ID,
				"cliente_id":   transacao.ClienteID,
				"valor":        transacao.Valor,
			})

			s.metricsCollector.IncrementErrorCounter("insufficient_limit")
		} else {
			s.logger.Error(ctx, "erro ao debitar limite", err, map[string]interface{}{
				"transacao_id": transacao.ID,
				"cliente_id":   transacao.ClienteID,
			})

			s.metricsCollector.IncrementErrorCounter("limit_operation_error")
		}
		return err
	}

	return nil
}

func (s *TransacaoService) aprovarTransacao(ctx context.Context, transacao *domain.Transacao) error {
	ctx, span := s.tracer.StartSpan(ctx, "TransacaoService.aprovarTransacao")
	defer s.tracer.FinishSpan(span, nil)

	// Marca transação como aprovada
	transacao.Aprovar()

	// Persiste a transação
	if err := s.transacaoRepository.Save(ctx, transacao); err != nil {
		s.logger.Error(ctx, "erro ao salvar transação", err, map[string]interface{}{
			"transacao_id": transacao.ID,
		})
		s.metricsCollector.IncrementErrorCounter("transaction_save_error")
		return err
	}

	// Publica evento de forma assíncrona
	// Em uma implementação real, isso seria feito em uma goroutine ou queue
	go s.publicarEvento(context.Background(), transacao)

	s.logger.Info(ctx, "transação aprovada com sucesso", map[string]interface{}{
		"transacao_id": transacao.ID,
		"cliente_id":   transacao.ClienteID,
		"valor":        transacao.Valor,
	})

	s.metricsCollector.IncrementTransactionCounter(domain.StatusAprovada)
	s.metricsCollector.RecordBusinessMetric("transaction_value", transacao.Valor, map[string]string{
		"status":     domain.StatusAprovada,
		"cliente_id": transacao.ClienteID,
	})

	return nil
}

func (s *TransacaoService) rejeitarTransacao(ctx context.Context, transacao *domain.Transacao, motivo error) error {
	ctx, span := s.tracer.StartSpan(ctx, "TransacaoService.rejeitarTransacao")
	defer s.tracer.FinishSpan(span, nil)

	// Marca transação como rejeitada
	transacao.Rejeitar()

	// Persiste a transação rejeitada para auditoria
	if err := s.transacaoRepository.Save(ctx, transacao); err != nil {
		s.logger.Error(ctx, "erro ao salvar transação rejeitada", err, map[string]interface{}{
			"transacao_id": transacao.ID,
		})
	}

	// Publica evento de rejeição
	go s.publicarEventoRejeicao(context.Background(), transacao, motivo)

	s.logger.Info(ctx, "transação rejeitada", map[string]interface{}{
		"transacao_id": transacao.ID,
		"cliente_id":   transacao.ClienteID,
		"motivo":       motivo.Error(),
	})

	s.metricsCollector.IncrementTransactionCounter(domain.StatusRejeitada)

	return motivo
}

func (s *TransacaoService) publicarEvento(ctx context.Context, transacao *domain.Transacao) {
	ctx, span := s.tracer.StartSpan(ctx, "TransacaoService.publicarEvento")
	defer s.tracer.FinishSpan(span, nil)

	evento := transacao.ToEvento()

	if err := s.eventPublisher.PublishTransacaoAprovada(ctx, evento); err != nil {
		s.logger.Error(ctx, "falha ao publicar evento de transação aprovada", err, map[string]interface{}{
			"transacao_id": transacao.ID,
			"evento":       evento.Evento,
		})
		s.metricsCollector.IncrementErrorCounter("event_publish_error")
	} else {
		s.logger.Info(ctx, "evento de transação publicado", map[string]interface{}{
			"transacao_id": transacao.ID,
			"evento":       evento.Evento,
		})
	}
}

func (s *TransacaoService) publicarEventoRejeicao(ctx context.Context, transacao *domain.Transacao, motivo error) {
	ctx, span := s.tracer.StartSpan(ctx, "TransacaoService.publicarEventoRejeicao")
	defer s.tracer.FinishSpan(span, nil)

	evento := transacao.ToEvento()

	if err := s.eventPublisher.PublishTransacaoRejeitada(ctx, evento); err != nil {
		s.logger.Error(ctx, "falha ao publicar evento de transação rejeitada", err, map[string]interface{}{
			"transacao_id": transacao.ID,
			"motivo":       motivo.Error(),
		})
		s.metricsCollector.IncrementErrorCounter("event_publish_error")
	}
}
