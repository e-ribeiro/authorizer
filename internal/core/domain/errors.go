package domain

import "errors"

var (
	ErrLimiteInsuficiente   = errors.New("limite insuficiente para autorizar a transação")
	ErrClienteNaoEncontrado = errors.New("cliente não encontrado")
	ErrTransacaoDuplicada   = errors.New("transação duplicada")
)
