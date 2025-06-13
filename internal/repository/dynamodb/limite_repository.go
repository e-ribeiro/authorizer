package dynamodb

import (
	"authorizer/internal/core/domain"
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type LimiteRepository struct {
	client    *dynamodb.Client
	tableName string
}

type ClienteItem struct {
	ID           string `dynamodbav:"id"`
	Nome         string `dynamodbav:"nome"`
	Email        string `dynamodbav:"email"`
	LimiteCredit int    `dynamodbav:"limite_credito"`
	LimiteAtual  int    `dynamodbav:"limite_atual"`
	CreatedAt    string `dynamodbav:"created_at"`
	UpdatedAt    string `dynamodbav:"updated_at"`
}

func NewLimiteRepository(client *dynamodb.Client, tableName string) *LimiteRepository {
	return &LimiteRepository{
		client:    client,
		tableName: tableName,
	}
}

// GetCliente busca um cliente pelo ID
func (r *LimiteRepository) GetCliente(ctx context.Context, clienteID string) (*domain.Cliente, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: clienteID},
		},
		// Leitura consistente para garantir os dados mais recentes
		ConsistentRead: aws.Bool(true),
	}

	result, err := r.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar cliente %s: %w", clienteID, err)
	}

	if result.Item == nil {
		return nil, domain.ErrClienteNaoEncontrado
	}

	var item ClienteItem
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return nil, fmt.Errorf("erro ao deserializar cliente: %w", err)
	}

	return r.itemToCliente(&item), nil
}

// UpdateLimite atualiza o limite atual do cliente
func (r *LimiteRepository) UpdateLimite(ctx context.Context, clienteID string, novoLimite int) error {
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: clienteID},
		},
		UpdateExpression: aws.String("SET limite_atual = :novo_limite, updated_at = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":novo_limite": &types.AttributeValueMemberN{Value: strconv.Itoa(novoLimite)},
			":now":         &types.AttributeValueMemberS{Value: fmt.Sprintf("%d", System.currentTimeMillis())},
		},
		// Verifica se o cliente existe antes de atualizar
		ConditionExpression: aws.String("attribute_exists(id)"),
	}

	_, err := r.client.UpdateItem(ctx, input)
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return domain.ErrClienteNaoEncontrado
		}
		return fmt.Errorf("erro ao atualizar limite do cliente %s: %w", clienteID, err)
	}

	return nil
}

// DebitarLimiteAtomica realiza a operação crítica de verificar limite E debitar
// em uma única operação atômica usando conditional writes do DynamoDB
func (r *LimiteRepository) DebitarLimiteAtomica(ctx context.Context, clienteID string, valor int) error {
	// Esta é a operação mais crítica do sistema
	// Usamos UpdateItem com ConditionExpression para garantir atomicidade
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: clienteID},
		},
		UpdateExpression: aws.String("SET limite_atual = limite_atual - :valor, updated_at = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":valor": &types.AttributeValueMemberN{Value: strconv.Itoa(valor)},
			":now":   &types.AttributeValueMemberS{Value: fmt.Sprintf("%d", System.currentTimeMillis())},
			":zero":  &types.AttributeValueMemberN{Value: "0"},
		},
		// Condições críticas:
		// 1. Cliente deve existir
		// 2. Limite atual deve ser >= valor da transação
		// 3. Limite atual não pode ficar negativo após a operação
		ConditionExpression: aws.String("attribute_exists(id) AND limite_atual >= :valor AND (limite_atual - :valor) >= :zero"),
		// Retorna os valores para debugging/auditoria
		ReturnValues: types.ReturnValueUpdatedNew,
	}

	result, err := r.client.UpdateItem(ctx, input)
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			// Se a condição falha, pode ser cliente inexistente OU limite insuficiente
			// Fazemos uma verificação adicional para distinguir
			cliente, getErr := r.GetCliente(ctx, clienteID)
			if getErr != nil {
				if errors.Is(getErr, domain.ErrClienteNaoEncontrado) {
					return domain.ErrClienteNaoEncontrado
				}
				// Se não conseguimos verificar, assumimos limite insuficiente
				return domain.ErrLimiteInsuficiente
			}

			// Cliente existe, então o problema é limite insuficiente
			if cliente.LimiteAtual < valor {
				return domain.ErrLimiteInsuficiente
			}

			// Caso raro: alguma outra condição falhou
			return fmt.Errorf("operação atômica falhou para cliente %s: %w", clienteID, err)
		}

		return fmt.Errorf("erro ao debitar limite do cliente %s: %w", clienteID, err)
	}

	// Log do resultado para auditoria (em produção, isso seria estruturado)
	if result.Attributes != nil {
		// Seria útil logar o novo limite para auditoria
		_ = result.Attributes // placeholder para implementação de auditoria
	}

	return nil
}

// Método auxiliar para converter item do DynamoDB para entidade de domínio
func (r *LimiteRepository) itemToCliente(item *ClienteItem) *domain.Cliente {
	return &domain.Cliente{
		ID:           item.ID,
		Nome:         item.Nome,
		Email:        item.Email,
		LimiteCredit: item.LimiteCredit,
		LimiteAtual:  item.LimiteAtual,
		// CreatedAt e UpdatedAt seriam convertidos de string para time.Time
		// em uma implementação real
	}
}

// CreateCliente cria um novo cliente (útil para testes e setup inicial)
func (r *LimiteRepository) CreateCliente(ctx context.Context, cliente *domain.Cliente) error {
	item := &ClienteItem{
		ID:           cliente.ID,
		Nome:         cliente.Nome,
		Email:        cliente.Email,
		LimiteCredit: cliente.LimiteCredit,
		LimiteAtual:  cliente.LimiteAtual,
		CreatedAt:    cliente.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    cliente.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("erro ao serializar cliente: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      av,
		// Evita sobrescrever cliente existente
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	}

	_, err = r.client.PutItem(ctx, input)
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return fmt.Errorf("cliente %s já existe", cliente.ID)
		}
		return fmt.Errorf("erro ao criar cliente: %w", err)
	}

	return nil
}

// currentTimeMillis simula System.currentTimeMillis() do Java
// Em uma implementação real, usaríamos time.Now().Unix() ou similar
var System = struct {
	currentTimeMillis func() int64
}{
	currentTimeMillis: func() int64 {
		return time.Now().Unix() * 1000
	},
}
