package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Transacao representa uma transação financeira
type Transacao struct {
	ID            string    `json:"id" dynamodbav:"id"`
	ClienteID     string    `json:"cliente_id" dynamodbav:"cliente_id"`
	Valor         float64   `json:"valor" dynamodbav:"valor"`
	Status        string    `json:"status" dynamodbav:"status"`
	Timestamp     time.Time `json:"timestamp" dynamodbav:"timestamp"`
	CorrelationID string    `json:"correlation_id" dynamodbav:"correlation_id"`
}

// Cliente representa um cliente no sistema
type Cliente struct {
	ID           string    `json:"id" dynamodbav:"id"`
	Nome         string    `json:"nome" dynamodbav:"nome"`
	Email        string    `json:"email" dynamodbav:"email"`
	LimiteCredit int       `json:"limite_credito" dynamodbav:"limite_credito"` // em centavos
	LimiteAtual  int       `json:"limite_atual" dynamodbav:"limite_atual"`     // em centavos
	CreatedAt    time.Time `json:"created_at" dynamodbav:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" dynamodbav:"updated_at"`
}

// TransacaoEvento representa um evento de transação para publicação
type TransacaoEvento struct {
	Evento        string    `json:"evento"`
	TransacaoID   string    `json:"transacao_id"`
	ClienteID     string    `json:"cliente_id"`
	Valor         float64   `json:"valor"`
	Timestamp     time.Time `json:"timestamp"`
	CorrelationID string    `json:"correlation_id"`
}

// Status de transação
const (
	StatusAprovada  = "APROVADA"
	StatusRejeitada = "REJEITADA"
	StatusPendente  = "PENDENTE"
)

// Tipos de evento
const (
	EventoTransacaoAprovada  = "TRANSACAO_APROVADA"
	EventoTransacaoRejeitada = "TRANSACAO_REJEITADA"
)

// Erros estruturados do domínio
var (
	ErrValorNegativo   = errors.New("o valor da transação não pode ser negativo")
	ErrValorZero       = errors.New("o valor da transação não pode ser zero")
	ErrClienteInvalido = errors.New("o ID do cliente é inválido ou não foi fornecido")
)

// NewTransacao cria uma nova transação com ID e timestamp
func NewTransacao(clienteID string, valor float64, correlationID string) *Transacao {
	return &Transacao{
		ID:            uuid.New().String(),
		ClienteID:     clienteID,
		Valor:         valor,
		Status:        StatusPendente,
		Timestamp:     time.Now(),
		CorrelationID: correlationID,
	}
}

// Valida verifica se a transação é válida
func (t *Transacao) Valida() error {
	if t.Valor <= 0 {
		if t.Valor < 0 {
			return ErrValorNegativo
		}

		if t.Valor == 0 {
			return ErrValorZero
		}
	}

	if t.ClienteID == "" {
		return ErrClienteInvalido
	}

	return nil
}

// Aprovar marca a transação como aprovada
func (t *Transacao) Aprovar() {
	t.Status = StatusAprovada
}

// Rejeitar marca a transação como rejeitada
func (t *Transacao) Rejeitar() {
	t.Status = StatusRejeitada
}

// ToEvento converte a transação em um evento para publicação
func (t *Transacao) ToEvento() *TransacaoEvento {
	var evento string
	switch t.Status {
	case StatusAprovada:
		evento = EventoTransacaoAprovada
	case StatusRejeitada:
		evento = EventoTransacaoRejeitada
	default:
		evento = "TRANSACAO_PROCESSADA"
	}

	return &TransacaoEvento{
		Evento:        evento,
		TransacaoID:   t.ID,
		ClienteID:     t.ClienteID,
		Valor:         t.Valor,
		Timestamp:     t.Timestamp,
		CorrelationID: t.CorrelationID,
	}
}
