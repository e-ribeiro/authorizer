package domain

import "context"

// LimiteRepository gerencia os limites de crédito dos clientes
type LimiteRepository interface {
	GetCliente(ctx context.Context, clienteID string) (*Cliente, error)
	UpdateLimite(ctx context.Context, clienteID string, novoLimite int) error
	// Operação atômica para debitar limite com verificação de race condition
	DebitarLimiteAtomica(ctx context.Context, clienteID string, valor int) error
}

// TransacaoRepository gerencia as transações
type TransacaoRepository interface {
	Save(ctx context.Context, transacao *Transacao) error
	GetByID(ctx context.Context, transacaoID string) (*Transacao, error)
	GetByClienteID(ctx context.Context, clienteID string, limit int) ([]*Transacao, error)
}

// EventPublisher publica eventos de transação para sistemas downstream
type EventPublisher interface {
	PublishTransacaoAprovada(ctx context.Context, evento *TransacaoEvento) error
	PublishTransacaoRejeitada(ctx context.Context, evento *TransacaoEvento) error
}

// MetricsCollector coleta métricas para observabilidade
type MetricsCollector interface {
	IncrementTransactionCounter(status string)
	RecordTransactionLatency(duration float64)
	RecordBusinessMetric(metricName string, value float64, labels map[string]string)
	IncrementErrorCounter(errorType string)
}

// DistributedTracer gerencia tracing distribuído
type DistributedTracer interface {
	StartSpan(ctx context.Context, operationName string) (context.Context, interface{})
	FinishSpan(span interface{}, err error)
	AddTag(span interface{}, key string, value interface{})
}

// Logger interface para logging estruturado
type Logger interface {
	Info(ctx context.Context, msg string, fields map[string]interface{})
	Error(ctx context.Context, msg string, err error, fields map[string]interface{})
	Warn(ctx context.Context, msg string, fields map[string]interface{})
	Debug(ctx context.Context, msg string, fields map[string]interface{})
}
