package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"itau/authorizer/internal/core/domain"
	"itau/authorizer/internal/core/service"
	awslambda "itau/authorizer/internal/handler/lambda"
	"itau/authorizer/internal/observability/logger"
	"itau/authorizer/internal/observability/tracing"
	dynamorepo "itau/authorizer/internal/repository/dynamodb"
)

func main() {
	// Clientes AWS (configuração simplificada)
	dynamoClient := &dynamodb.Client{} // Em produção, seria configurado com credenciais

	// Configurações do ambiente
	clientesTableName := getEnvOrDefault("CLIENTES_TABLE_NAME", "clientes")
	transacoesTableName := getEnvOrDefault("TRANSACOES_TABLE_NAME", "transacoes")
	snsTopicArn := getEnvOrDefault("SNS_TOPIC_ARN", "arn:aws:sns:us-east-1:123456789012:transacoes")

	// Inicialização dos componentes de observabilidade
	structuredLogger := logger.NewStructuredLogger()
	simpleTracer := tracing.NewSimpleTracer("itau-authorizer")

	// Inicialização dos repositórios
	limiteRepository := dynamorepo.NewLimiteRepository(dynamoClient, clientesTableName)
	transacaoRepository := dynamorepo.NewTransacaoRepository(dynamoClient, transacoesTableName)
	eventPublisher := &SimpleEventPublisher{topicArn: snsTopicArn}

	// Métricas collector simplificado
	metricsCollector := &SimpleMetricsCollector{}

	// Inicialização do serviço principal
	transacaoService := service.NewTransacaoService(
		limiteRepository,
		transacaoRepository,
		eventPublisher,
		metricsCollector,
		simpleTracer,
		structuredLogger,
	)

	// Inicialização do handler Lambda
	handler := awslambda.NewLambdaHandler(
		transacaoService,
		structuredLogger,
		simpleTracer,
		metricsCollector,
	)

	// Inicia o Lambda
	lambda.Start(handler.HandleRequest)
}

// getEnvOrDefault retorna variável de ambiente ou valor padrão
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// SimpleMetricsCollector implementação simplificada para metrics
type SimpleMetricsCollector struct{}

func (s *SimpleMetricsCollector) IncrementTransactionCounter(status string) {
	log.Printf("METRIC: transaction_count{status=%s} +1", status)
}

func (s *SimpleMetricsCollector) RecordTransactionLatency(duration float64) {
	log.Printf("METRIC: transaction_duration %.3fms", duration*1000)
}

func (s *SimpleMetricsCollector) RecordBusinessMetric(metricName string, value float64, labels map[string]string) {
	log.Printf("METRIC: %s{%v} %.2f", metricName, labels, value)
}

func (s *SimpleMetricsCollector) IncrementErrorCounter(errorType string) {
	log.Printf("METRIC: error_count{type=%s} +1", errorType)
}

// SimpleEventPublisher implementação simplificada para eventos
type SimpleEventPublisher struct {
	topicArn string
}

func (s *SimpleEventPublisher) PublishTransacaoAprovada(ctx context.Context, evento *domain.TransacaoEvento) error {
	log.Printf("EVENT: Transação aprovada - Cliente: %s, Valor: %.2f, ID: %s",
		evento.ClienteID, evento.Valor, evento.TransacaoID)
	return nil
}

func (s *SimpleEventPublisher) PublishTransacaoRejeitada(ctx context.Context, evento *domain.TransacaoEvento) error {
	log.Printf("EVENT: Transação rejeitada - Cliente: %s, Valor: %.2f, ID: %s",
		evento.ClienteID, evento.Valor, evento.TransacaoID)
	return nil
}
