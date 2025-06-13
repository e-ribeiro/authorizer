package awslambda

import (
	"authorizer/internal/core/domain"
	"authorizer/internal/core/service"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
)

// LambdaHandler é o handler principal para AWS Lambda
type LambdaHandler struct {
	transacaoService service.TransacaoService
	logger           domain.Logger
	tracer           domain.DistributedTracer
	metricsCollector domain.MetricsCollector
}

// TransacaoRequest representa o payload da requisição
type TransacaoRequest struct {
	ClienteID string  `json:"cliente_id"`
	Valor     float64 `json:"valor"`
}

// TransacaoResponse representa a resposta da API
type TransacaoResponse struct {
	TransacaoID   string    `json:"transacao_id"`
	Status        string    `json:"status"`
	ClienteID     string    `json:"cliente_id"`
	Valor         float64   `json:"valor"`
	Timestamp     time.Time `json:"timestamp"`
	CorrelationID string    `json:"correlation_id"`
}

// ErrorResponse representa uma resposta de erro
type ErrorResponse struct {
	Error         string `json:"error"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlation_id"`
	Timestamp     string `json:"timestamp"`
}

// Dependências injetadas via construtor
func NewLambdaHandler(
	transacaoService *service.TransacaoService,
	logger domain.Logger,
	tracer domain.DistributedTracer,
	metricsCollector domain.MetricsCollector,
) *LambdaHandler {
	return &LambdaHandler{
		transacaoService: *transacaoService,
		logger:           logger,
		tracer:           tracer,
		metricsCollector: metricsCollector,
	}
}

// HandleRequest é o ponto de entrada principal do Lambda
func (h *LambdaHandler) HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	startTime := time.Now()

	// Gera correlation ID a partir do trace ID ou cria um novo
	correlationID := h.extractOrGenerateCorrelationID(request)
	ctx = context.WithValue(ctx, "correlation_id", correlationID)

	// Inicia span de tracing distribuído
	ctx, span := h.tracer.StartSpan(ctx, "lambda.handle_request")
	defer h.tracer.FinishSpan(span, nil)

	h.tracer.AddTag(span, "http.method", request.HTTPMethod)
	h.tracer.AddTag(span, "http.path", request.Path)
	h.tracer.AddTag(span, "correlation_id", correlationID)

	// Log da requisição
	h.logger.Info(ctx, "requisição recebida", map[string]interface{}{
		"method":    request.HTTPMethod,
		"path":      request.Path,
		"source_ip": request.RequestContext.Identity.SourceIP,
	})

	// Roteamento baseado no método e path
	var response events.APIGatewayProxyResponse
	var err error

	switch {
	case request.HTTPMethod == "POST" && request.Path == "/transacoes":
		response, err = h.handlePostTransacoes(ctx, request)
	case request.HTTPMethod == "GET" && request.Path == "/health":
		response, err = h.handleHealthCheck(ctx)
	default:
		response = h.createErrorResponse(http.StatusNotFound, "endpoint_not_found", "Endpoint não encontrado", correlationID)
	}

	// Registra métricas de latência
	duration := time.Since(startTime).Seconds()
	h.metricsCollector.RecordTransactionLatency(duration)

	// Log da resposta
	h.logger.Info(ctx, "resposta enviada", map[string]interface{}{
		"status_code": response.StatusCode,
		"duration_ms": duration * 1000,
	})

	return response, err
}

// handlePostTransacoes processa POST /transacoes
func (h *LambdaHandler) handlePostTransacoes(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	ctx, span := h.tracer.StartSpan(ctx, "handler.post_transacoes")
	defer h.tracer.FinishSpan(span, nil)

	correlationID := ctx.Value("correlation_id").(string)

	// Parse do JSON
	var req TransacaoRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		h.logger.Warn(ctx, "erro ao fazer parse do JSON", map[string]interface{}{
			"error": err.Error(),
			"body":  request.Body,
		})
		h.metricsCollector.IncrementErrorCounter("json_parse_error")
		return h.createErrorResponse(http.StatusBadRequest, "invalid_json", "JSON inválido", correlationID), nil
	}

	h.tracer.AddTag(span, "cliente_id", req.ClienteID)
	h.tracer.AddTag(span, "valor", req.Valor)

	// Cria transação
	transacao := domain.NewTransacao(req.ClienteID, req.Valor, correlationID)

	// Processa transação
	err := h.transacaoService.AutorizarTransacao(ctx, transacao)
	if err != nil {
		// Determina o tipo de erro e status HTTP
		statusCode, errorCode, message := h.categorizeError(err)

		h.logger.Warn(ctx, "transação rejeitada", map[string]interface{}{
			"transacao_id": transacao.ID,
			"error":        err.Error(),
			"error_code":   errorCode,
		})

		return h.createErrorResponse(statusCode, errorCode, message, correlationID), nil
	}

	// Resposta de sucesso
	response := TransacaoResponse{
		TransacaoID:   transacao.ID,
		Status:        transacao.Status,
		ClienteID:     transacao.ClienteID,
		Valor:         transacao.Valor,
		Timestamp:     transacao.Timestamp,
		CorrelationID: correlationID,
	}

	responseBody, _ := json.Marshal(response)

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":     "application/json",
			"X-Correlation-ID": correlationID,
			"X-Response-Time":  fmt.Sprintf("%.3fms", time.Since(transacao.Timestamp).Seconds()*1000),
		},
		Body: string(responseBody),
	}, nil
}

// handleHealthCheck responde ao health check
func (h *LambdaHandler) handleHealthCheck(ctx context.Context) (events.APIGatewayProxyResponse, error) {
	healthResponse := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0",
		"service":   "transaction-authorizer",
	}

	responseBody, _ := json.Marshal(healthResponse)

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(responseBody),
	}, nil
}

// categorizeError categoriza erros em códigos HTTP e tipos de erro
func (h *LambdaHandler) categorizeError(err error) (int, string, string) {
	switch {
	case err == domain.ErrLimiteInsuficiente:
		return http.StatusUnprocessableEntity, "insufficient_limit", "Limite insuficiente"
	case err == domain.ErrClienteNaoEncontrado:
		return http.StatusNotFound, "client_not_found", "Cliente não encontrado"
	case err == domain.ErrValorNegativo || err == domain.ErrValorZero:
		return http.StatusBadRequest, "invalid_amount", "Valor inválido"
	case err == domain.ErrClienteInvalido:
		return http.StatusBadRequest, "invalid_client", "Cliente inválido"
	default:
		return http.StatusInternalServerError, "internal_error", "Erro interno do servidor"
	}
}

// createErrorResponse cria uma resposta de erro padronizada
func (h *LambdaHandler) createErrorResponse(statusCode int, errorCode, message, correlationID string) events.APIGatewayProxyResponse {
	errorResponse := ErrorResponse{
		Error:         errorCode,
		Message:       message,
		CorrelationID: correlationID,
		Timestamp:     time.Now().Format(time.RFC3339),
	}

	responseBody, _ := json.Marshal(errorResponse)

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":     "application/json",
			"X-Correlation-ID": correlationID,
		},
		Body: string(responseBody),
	}
}

// extractOrGenerateCorrelationID extrai correlation ID do header ou gera um novo
func (h *LambdaHandler) extractOrGenerateCorrelationID(request events.APIGatewayProxyRequest) string {
	// Tenta extrair do header
	if correlationID := request.Headers["X-Correlation-ID"]; correlationID != "" {
		return correlationID
	}

	// Tenta extrair do request ID do API Gateway
	if requestID := request.RequestContext.RequestID; requestID != "" {
		return requestID
	}

	// Gera novo UUID
	return uuid.New().String()
}
