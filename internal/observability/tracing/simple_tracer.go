package tracing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SimpleTracer implementa domain.DistributedTracer de forma simplificada
type SimpleTracer struct {
	serviceName string
}

// SimpleSpan representa um span de tracing simplificado
type SimpleSpan struct {
	TraceID       string                 `json:"trace_id"`
	SpanID        string                 `json:"span_id"`
	OperationName string                 `json:"operation_name"`
	StartTime     time.Time              `json:"start_time"`
	EndTime       *time.Time             `json:"end_time,omitempty"`
	Tags          map[string]interface{} `json:"tags"`
	Events        []SpanEvent            `json:"events"`
	Status        string                 `json:"status"`
	Error         *string                `json:"error,omitempty"`
}

// SpanEvent representa um evento dentro de um span
type SpanEvent struct {
	Name       string                 `json:"name"`
	Timestamp  time.Time              `json:"timestamp"`
	Attributes map[string]interface{} `json:"attributes"`
}

func NewSimpleTracer(serviceName string) *SimpleTracer {
	return &SimpleTracer{
		serviceName: serviceName,
	}
}

// StartSpan inicia um novo span de tracing
func (t *SimpleTracer) StartSpan(ctx context.Context, operationName string) (context.Context, interface{}) {
	// Gera IDs únicos
	traceID := generateTraceID(ctx)
	spanID := uuid.New().String()

	span := &SimpleSpan{
		TraceID:       traceID,
		SpanID:        spanID,
		OperationName: operationName,
		StartTime:     time.Now(),
		Tags: map[string]interface{}{
			"service.name":    t.serviceName,
			"service.version": "1.0.0",
		},
		Events: make([]SpanEvent, 0),
		Status: "started",
	}

	// Injeta span no contexto
	spanCtx := context.WithValue(ctx, "span", span)
	spanCtx = context.WithValue(spanCtx, "trace_id", traceID)

	return spanCtx, span
}

// FinishSpan finaliza o span
func (t *SimpleTracer) FinishSpan(span interface{}, err error) {
	if simpleSpan, ok := span.(*SimpleSpan); ok {
		now := time.Now()
		simpleSpan.EndTime = &now

		if err != nil {
			simpleSpan.Status = "error"
			errMsg := err.Error()
			simpleSpan.Error = &errMsg
		} else {
			simpleSpan.Status = "completed"
		}

		// Em produção, aqui enviaria para sistema de tracing (Jaeger, Zipkin, etc.)
		t.logSpan(simpleSpan)
	}
}

// AddTag adiciona uma tag/atributo ao span
func (t *SimpleTracer) AddTag(span interface{}, key string, value interface{}) {
	if simpleSpan, ok := span.(*SimpleSpan); ok {
		simpleSpan.Tags[key] = value
	}
}

// AddEvent adiciona um evento ao span
func (t *SimpleTracer) AddEvent(span interface{}, name string, attributes map[string]interface{}) {
	if simpleSpan, ok := span.(*SimpleSpan); ok {
		event := SpanEvent{
			Name:       name,
			Timestamp:  time.Now(),
			Attributes: attributes,
		}
		simpleSpan.Events = append(simpleSpan.Events, event)
	}
}

// ExtractTraceID extrai o trace ID do contexto
func (t *SimpleTracer) ExtractTraceID(ctx context.Context) string {
	if value := ctx.Value("trace_id"); value != nil {
		if traceID, ok := value.(string); ok {
			return traceID
		}
	}
	return ""
}

// InjectCorrelationID injeta correlation ID no contexto baseado no trace ID
func (t *SimpleTracer) InjectCorrelationID(ctx context.Context) context.Context {
	traceID := t.ExtractTraceID(ctx)
	if traceID != "" {
		return context.WithValue(ctx, "correlation_id", traceID)
	}
	return ctx
}

// generateTraceID gera ou extrai trace ID do contexto
func generateTraceID(ctx context.Context) string {
	// Verifica se já existe um trace ID no contexto
	if existing := ctx.Value("trace_id"); existing != nil {
		if traceID, ok := existing.(string); ok {
			return traceID
		}
	}

	// Gera novo trace ID
	return uuid.New().String()
}

// logSpan simula envio para sistema de tracing
func (t *SimpleTracer) logSpan(span *SimpleSpan) {
	// Em produção, isso seria enviado para Jaeger, Zipkin, AWS X-Ray, etc.
	duration := time.Since(span.StartTime)

	fmt.Printf("TRACE [%s] %s %s - %dms %s\n",
		span.TraceID[:8],
		span.OperationName,
		span.Status,
		duration.Milliseconds(),
		func() string {
			if span.Error != nil {
				return fmt.Sprintf("ERROR: %s", *span.Error)
			}
			return ""
		}(),
	)
}
