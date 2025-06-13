package logger

import (
	"context"
	"log/slog"
	"os"
)

// StructuredLogger implementa domain.Logger usando log/slog
type StructuredLogger struct {
	logger *slog.Logger
}

func NewStructuredLogger() *StructuredLogger {
	// Configuração do logger estruturado
	opts := &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	}

	// Handler JSON para produção
	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)

	return &StructuredLogger{
		logger: logger,
	}
}

// NewStructuredLoggerWithLevel cria logger com nível específico
func NewStructuredLoggerWithLevel(level slog.Level) *StructuredLogger {
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)

	return &StructuredLogger{
		logger: logger,
	}
}

// Info registra log de informação
func (l *StructuredLogger) Info(ctx context.Context, msg string, fields map[string]interface{}) {
	l.logWithFields(ctx, slog.LevelInfo, msg, fields)
}

// Error registra log de erro
func (l *StructuredLogger) Error(ctx context.Context, msg string, err error, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["error"] = err.Error()
	l.logWithFields(ctx, slog.LevelError, msg, fields)
}

// Warn registra log de warning
func (l *StructuredLogger) Warn(ctx context.Context, msg string, fields map[string]interface{}) {
	l.logWithFields(ctx, slog.LevelWarn, msg, fields)
}

// Debug registra log de debug
func (l *StructuredLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {
	l.logWithFields(ctx, slog.LevelDebug, msg, fields)
}

// logWithFields é método auxiliar para logar com campos estruturados
func (l *StructuredLogger) logWithFields(ctx context.Context, level slog.Level, msg string, fields map[string]interface{}) {
	// Extrai correlation_id do contexto se disponível
	correlationID := extractCorrelationID(ctx)
	if correlationID != "" {
		if fields == nil {
			fields = make(map[string]interface{})
		}
		fields["correlation_id"] = correlationID
	}

	// Converte map para slog.Attr
	attrs := make([]slog.Attr, 0, len(fields))
	for key, value := range fields {
		attrs = append(attrs, slog.Any(key, value))
	}

	l.logger.LogAttrs(ctx, level, msg, attrs...)
}

// extractCorrelationID extrai correlation ID do contexto
func extractCorrelationID(ctx context.Context) string {
	if value := ctx.Value("correlation_id"); value != nil {
		if strValue, ok := value.(string); ok {
			return strValue
		}
	}
	return ""
}

// WithCorrelationID adiciona correlation ID ao contexto
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, "correlation_id", correlationID)
}
