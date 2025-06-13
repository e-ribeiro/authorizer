package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewTransacao(t *testing.T) {
	clienteID := "12345"
	valor := 99.90
	correlationID := "test-correlation"

	transacao := NewTransacao(clienteID, valor, correlationID)

	// Verifica se os campos foram preenchidos corretamente
	if transacao.ClienteID != clienteID {
		t.Errorf("ClienteID esperado %s, got %s", clienteID, transacao.ClienteID)
	}

	if transacao.Valor != valor {
		t.Errorf("Valor esperado %.2f, got %.2f", valor, transacao.Valor)
	}

	if transacao.CorrelationID != correlationID {
		t.Errorf("CorrelationID esperado %s, got %s", correlationID, transacao.CorrelationID)
	}

	// Verifica se o ID foi gerado
	if transacao.ID == "" {
		t.Error("ID não deve estar vazio")
	}

	// Verifica se é um UUID válido
	if _, err := uuid.Parse(transacao.ID); err != nil {
		t.Errorf("ID deve ser um UUID válido: %v", err)
	}

	// Verifica status inicial
	if transacao.Status != StatusPendente {
		t.Errorf("Status inicial esperado %s, got %s", StatusPendente, transacao.Status)
	}

	// Verifica se timestamp foi definido
	if transacao.Timestamp.IsZero() {
		t.Error("Timestamp não deve estar zerado")
	}
}

func TestTransacao_Valida(t *testing.T) {
	tests := []struct {
		name        string
		transacao   *Transacao
		expectedErr error
	}{
		{
			name: "transação válida",
			transacao: &Transacao{
				ClienteID: "12345",
				Valor:     99.90,
			},
			expectedErr: nil,
		},
		{
			name: "valor negativo",
			transacao: &Transacao{
				ClienteID: "12345",
				Valor:     -10.0,
			},
			expectedErr: ErrValorNegativo,
		},
		{
			name: "valor zero",
			transacao: &Transacao{
				ClienteID: "12345",
				Valor:     0.0,
			},
			expectedErr: ErrValorZero,
		},
		{
			name: "cliente inválido",
			transacao: &Transacao{
				ClienteID: "",
				Valor:     99.90,
			},
			expectedErr: ErrClienteInvalido,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.transacao.Valida()

			if err != tt.expectedErr {
				t.Errorf("Erro esperado %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestTransacao_Aprovar(t *testing.T) {
	transacao := NewTransacao("12345", 99.90, "test")

	transacao.Aprovar()

	if transacao.Status != StatusAprovada {
		t.Errorf("Status esperado %s, got %s", StatusAprovada, transacao.Status)
	}
}

func TestTransacao_Rejeitar(t *testing.T) {
	transacao := NewTransacao("12345", 99.90, "test")

	transacao.Rejeitar()

	if transacao.Status != StatusRejeitada {
		t.Errorf("Status esperado %s, got %s", StatusRejeitada, transacao.Status)
	}
}

func TestTransacao_ToEvento(t *testing.T) {
	transacao := NewTransacao("12345", 99.90, "test-correlation")
	transacao.Aprovar()

	evento := transacao.ToEvento()

	// Verifica campos do evento
	if evento.Evento != EventoTransacaoAprovada {
		t.Errorf("Evento esperado %s, got %s", EventoTransacaoAprovada, evento.Evento)
	}

	if evento.TransacaoID != transacao.ID {
		t.Errorf("TransacaoID esperado %s, got %s", transacao.ID, evento.TransacaoID)
	}

	if evento.ClienteID != transacao.ClienteID {
		t.Errorf("ClienteID esperado %s, got %s", transacao.ClienteID, evento.ClienteID)
	}

	if evento.Valor != transacao.Valor {
		t.Errorf("Valor esperado %.2f, got %.2f", transacao.Valor, evento.Valor)
	}

	if evento.CorrelationID != transacao.CorrelationID {
		t.Errorf("CorrelationID esperado %s, got %s", transacao.CorrelationID, evento.CorrelationID)
	}
}

func TestTransacao_ToEvento_Rejeitada(t *testing.T) {
	transacao := NewTransacao("12345", 99.90, "test-correlation")
	transacao.Rejeitar()

	evento := transacao.ToEvento()

	if evento.Evento != EventoTransacaoRejeitada {
		t.Errorf("Evento esperado %s, got %s", EventoTransacaoRejeitada, evento.Evento)
	}
}

// Benchmarks para performance
func BenchmarkNewTransacao(b *testing.B) {
	clienteID := "12345"
	valor := 99.90
	correlationID := "test-correlation"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewTransacao(clienteID, valor, correlationID)
	}
}

func BenchmarkTransacao_Valida(b *testing.B) {
	transacao := NewTransacao("12345", 99.90, "test")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		transacao.Valida()
	}
}

// Testes de propriedades (Property-based testing)
func TestTransacao_Properties(t *testing.T) {
	// Teste: Uma transação sempre deve ter timestamp maior que zero
	transacao := NewTransacao("test", 100.0, "correlation")

	if !transacao.Timestamp.After(time.Time{}) {
		t.Error("Transação deve sempre ter timestamp válido")
	}

	// Teste: Status inicial sempre deve ser PENDENTE
	if transacao.Status != StatusPendente {
		t.Error("Status inicial sempre deve ser PENDENTE")
	}

	// Teste: ID sempre deve ser único
	transacao2 := NewTransacao("test", 100.0, "correlation")
	if transacao.ID == transacao2.ID {
		t.Error("IDs de transações devem ser únicos")
	}
}
