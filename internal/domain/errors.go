package domain

import (
	"errors"
	"fmt"
)

type DomainError struct {
	Code    string
	Message string
	Err     error
}

var (
	// Valor vazio, com letras, NaN, Infinity ou notação científica
	ErrInvalidAmountFormat = errors.New("invalid amount format")
	// Escala decimal diferente de 2 casas
	ErrInvalidScale = errors.New("invalid scale")
	// Valor negativo
	ErrNegativeAmount = errors.New("negative amount")
	// Tentativa de qualquer operação com moeda diferente
	ErrCurrencyMismatch = errors.New("currency mismatch")
	// Moeda inválida(deve ser ISO 4217, ex: "BRL")
	ErrInvalidCurrency = errors.New("invalid currency")
	// Overflow no parsing ou operação
	ErrAmountOverflow = errors.New("amount overflow")
)

var (
	// Tentativa de operação com saldo insuficiente
	ErrInsufficientBalance = errors.New("insufficient balance")
	// Tentativa de operação com moeda diferente da carteira
	ErrWalletCurrencyMismatch = errors.New("wallet currency mismatch")
	// Tentativa de operação com carteira inválida
	ErrInvalidWalletId = errors.New("invalid wallet id")
)

var (
	// Regra imutável "balanceAfter == balanceBefore + amount" violada
	ErrInvalidLedgerMath = errors.New("invalid ledger math")
	// Direção inválida
	ErrInvalidLedgerDirection = errors.New("invalid ledger direction")
)

var (
	// Tentativa de mudar estado a partir de estado terminal ou inválido
	ErrInvalidStateTransition = errors.New("invalid state transition")
	// Tipo de transação desconhecido ou envio de "OPENING" via HTTP/SQS(exclusivo interno)
	ErrInvalidTransactionType = errors.New("invalid transaction type")
	// "REFUND" ou "ROLLBACK" sem o ID da transação referenciada
	ErrMissingReference = errors.New("missing reference")
	// Daddos de reversão não batem com o original
	ErrReferenceMismatch = errors.New("reference mismatch")
	// Transação originnal já revertida anteriormente
	ErrDuplicateReversal = errors.New("duplicate reversal")
	// Reversão exigiria debitar mais que o saldo disponível
	ErrReversalInsufficientBalance = errors.New("reversal insufficient balance")
)

func (e *DomainError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *DomainError) Unwrap() error {
	return e.Err
}
