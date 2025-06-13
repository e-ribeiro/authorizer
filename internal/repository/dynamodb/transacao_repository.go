package dynamodb

import (
	"authorizer/internal/core/domain"
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type TransacaoRepository struct {
	client    *dynamodb.Client
	tableName string
}

type TransacaoItem struct {
	ID            string  `dynamodbav:"id"`
	ClienteID     string  `dynamodbav:"cliente_id"`
	Valor         float64 `dynamodbav:"valor"`
	Status        string  `dynamodbav:"status"`
	Timestamp     string  `dynamodbav:"timestamp"`
	CorrelationID string  `dynamodbav:"correlation_id"`
	TTL           int64   `dynamodbav:"ttl"` // Para limpeza automática de dados antigos
}

func NewTransacaoRepository(client *dynamodb.Client, tableName string) *TransacaoRepository {
	return &TransacaoRepository{
		client:    client,
		tableName: tableName,
	}
}

// Save persiste uma transação no DynamoDB
func (r *TransacaoRepository) Save(ctx context.Context, transacao *domain.Transacao) error {
	// TTL para 90 dias (limpeza automática de dados antigos)
	ttl := transacao.Timestamp.Unix() + (90 * 24 * 60 * 60)

	item := &TransacaoItem{
		ID:            transacao.ID,
		ClienteID:     transacao.ClienteID,
		Valor:         transacao.Valor,
		Status:        transacao.Status,
		Timestamp:     transacao.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		CorrelationID: transacao.CorrelationID,
		TTL:           ttl,
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("erro ao serializar transação: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      av,
		// Evita sobrescrever transação existente (idempotência)
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	}

	_, err = r.client.PutItem(ctx, input)
	if err != nil {
		// Se a transação já existe, não é um erro crítico (idempotência)
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return fmt.Errorf("transação %s já existe", transacao.ID)
		}
		return fmt.Errorf("erro ao salvar transação: %w", err)
	}

	return nil
}

// GetByID busca uma transação por ID
func (r *TransacaoRepository) GetByID(ctx context.Context, transacaoID string) (*domain.Transacao, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: transacaoID},
		},
		ConsistentRead: aws.Bool(true),
	}

	result, err := r.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar transação %s: %w", transacaoID, err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("transação %s não encontrada", transacaoID)
	}

	var item TransacaoItem
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return nil, fmt.Errorf("erro ao deserializar transação: %w", err)
	}

	return r.itemToTransacao(&item), nil
}

// GetByClienteID busca transações de um cliente específico (útil para auditoria)
func (r *TransacaoRepository) GetByClienteID(ctx context.Context, clienteID string, limit int) ([]*domain.Transacao, error) {
	// Assumindo que temos um GSI (Global Secondary Index) por cliente_id
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("cliente-id-index"), // GSI necessário
		KeyConditionExpression: aws.String("cliente_id = :cliente_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":cliente_id": &types.AttributeValueMemberS{Value: clienteID},
		},
		Limit:            aws.Int32(int32(limit)),
		ScanIndexForward: aws.Bool(false), // Ordem decrescente (mais recentes primeiro)
	}

	result, err := r.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar transações do cliente %s: %w", clienteID, err)
	}

	transacoes := make([]*domain.Transacao, 0, len(result.Items))
	for _, item := range result.Items {
		var transacaoItem TransacaoItem
		if err := attributevalue.UnmarshalMap(item, &transacaoItem); err != nil {
			// Log do erro, mas continua processando outras transações
			continue
		}
		transacoes = append(transacoes, r.itemToTransacao(&transacaoItem))
	}

	return transacoes, nil
}

// Converte item do DynamoDB para entidade de domínio
func (r *TransacaoRepository) itemToTransacao(item *TransacaoItem) *domain.Transacao {
	// Em uma implementação real, faria o parsing do timestamp
	// timestamp, _ := time.Parse("2006-01-02T15:04:05Z07:00", item.Timestamp)

	return &domain.Transacao{
		ID:            item.ID,
		ClienteID:     item.ClienteID,
		Valor:         item.Valor,
		Status:        item.Status,
		CorrelationID: item.CorrelationID,
		// Timestamp:     timestamp,
	}
}
