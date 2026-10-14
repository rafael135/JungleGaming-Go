package application

import (
	"encoding/json"
	"fmt"
	"time"

	"jungle/internal/infrastructure/database"

	"github.com/google/uuid"
)

const (
	EventTypeWagerTransactionProcessed        = "WagerTransactionProcessed"
	EventTypeWagerTransactionRejected         = "WagerTransactionRejected"
	EventTypeWalletBalanceChanged             = "WalletBalanceChanged"
	EventTypeWagerTransactionPendingReference = "WagerTransactionPendingReference"
)

type EventMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type EventEnvelope struct {
	EventID       string      `json:"eventId"`
	EventType     string      `json:"eventType"`
	AggregateID   string      `json:"aggregateId"`
	CorrelationID string      `json:"correlationId"`
	CausationID   string      `json:"causationId,omitempty"`
	OccurredAt    string      `json:"occurredAt"`
	Version       int         `json:"version"`
	Data          interface{} `json:"data"`
}

type WalletBalanceChangedData struct {
	WalletID      string     `json:"walletId"`
	TransactionID string     `json:"transactionId"`
	Direction     string     `json:"direction"`
	Money         EventMoney `json:"money"`
	BalanceBefore EventMoney `json:"balanceBefore"`
	BalanceAfter  EventMoney `json:"balanceAfter"`
	WalletVersion int64      `json:"walletVersion"`
}

type WagerTransactionProcessedData struct {
	TransactionID                  string     `json:"transactionId"`
	WalletID                       string     `json:"walletId"`
	PlayerID                       string     `json:"playerId"`
	TransactionType                string     `json:"transactionType"`
	Amount                         EventMoney `json:"amount"`
	Status                         string     `json:"status"`
	ProviderID                     string     `json:"providerId,omitempty"`
	ExternalTransactionID          string     `json:"externalTransactionId,omitempty"`
	ReferenceExternalTransactionID string     `json:"referenceExternalTransactionId,omitempty"`
	GameID                         string     `json:"gameId,omitempty"`
	RoundID                        string     `json:"roundId,omitempty"`
}

type WagerTransactionRejectedData struct {
	TransactionID         string     `json:"transactionId"`
	WalletID              string     `json:"walletId"`
	PlayerID              string     `json:"playerId"`
	TransactionType       string     `json:"transactionType"`
	Amount                EventMoney `json:"amount"`
	Status                string     `json:"status"`
	FailureCode           string     `json:"failureCode"`
	ProviderID            string     `json:"providerId,omitempty"`
	ExternalTransactionID string     `json:"externalTransactionId,omitempty"`
}

type WagerTransactionPendingReferenceData struct {
	TransactionID                  string     `json:"transactionId"`
	WalletID                       string     `json:"walletId"`
	PlayerID                       string     `json:"playerId"`
	TransactionType                string     `json:"transactionType"`
	Amount                         EventMoney `json:"amount"`
	Status                         string     `json:"status"`
	ProviderID                     string     `json:"providerId"`
	ExternalTransactionID          string     `json:"externalTransactionId"`
	ReferenceExternalTransactionID string     `json:"referenceExternalTransactionId"`
}

// NewWalletBalanceChangedOutboxMessage cria uma mensagem de outbox com o evento tipado WalletBalanceChanged.
func NewWalletBalanceChangedOutboxMessage(
	correlationID string,
	causationID string,
	data WalletBalanceChangedData,
) (*database.OutboxMessage, error) {
	now := time.Now().UTC()
	eventID := uuid.NewString()

	envelope := EventEnvelope{
		EventID:       eventID,
		EventType:     EventTypeWalletBalanceChanged,
		AggregateID:   data.WalletID,
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    now.Format(time.RFC3339),
		Version:       1,
		Data:          data,
	}

	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar evento WalletBalanceChanged: %w", err)
	}

	return &database.OutboxMessage{
		ID:            eventID,
		AggregateType: "Wallet",
		AggregateID:   data.WalletID,
		EventType:     EventTypeWalletBalanceChanged,
		Payload:       payloadBytes,
		Status:        database.OutboxStatusPending,
		RetryCount:    0,
		NextAttemptAt: now,
		CreatedAt:     now,
	}, nil
}

// NewWagerTransactionProcessedOutboxMessage cria uma mensagem de outbox com o evento tipado WagerTransactionProcessed.
func NewWagerTransactionProcessedOutboxMessage(
	correlationID string,
	causationID string,
	data WagerTransactionProcessedData,
) (*database.OutboxMessage, error) {
	now := time.Now().UTC()
	eventID := uuid.NewString()

	envelope := EventEnvelope{
		EventID:       eventID,
		EventType:     EventTypeWagerTransactionProcessed,
		AggregateID:   data.TransactionID,
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    now.Format(time.RFC3339),
		Version:       1,
		Data:          data,
	}

	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar evento WagerTransactionProcessed: %w", err)
	}

	return &database.OutboxMessage{
		ID:            eventID,
		AggregateType: "WagerTransaction",
		AggregateID:   data.TransactionID,
		EventType:     EventTypeWagerTransactionProcessed,
		Payload:       payloadBytes,
		Status:        database.OutboxStatusPending,
		RetryCount:    0,
		NextAttemptAt: now,
		CreatedAt:     now,
	}, nil
}

// NewWagerTransactionRejectedOutboxMessage cria uma mensagem de outbox com o evento tipado WagerTransactionRejected.
func NewWagerTransactionRejectedOutboxMessage(
	correlationID string,
	causationID string,
	data WagerTransactionRejectedData,
) (*database.OutboxMessage, error) {
	now := time.Now().UTC()
	eventID := uuid.NewString()

	envelope := EventEnvelope{
		EventID:       eventID,
		EventType:     EventTypeWagerTransactionRejected,
		AggregateID:   data.TransactionID,
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    now.Format(time.RFC3339),
		Version:       1,
		Data:          data,
	}

	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar evento WagerTransactionRejected: %w", err)
	}

	return &database.OutboxMessage{
		ID:            eventID,
		AggregateType: "WagerTransaction",
		AggregateID:   data.TransactionID,
		EventType:     EventTypeWagerTransactionRejected,
		Payload:       payloadBytes,
		Status:        database.OutboxStatusPending,
		RetryCount:    0,
		NextAttemptAt: now,
		CreatedAt:     now,
	}, nil
}

// NewWagerTransactionPendingReferenceOutboxMessage cria uma mensagem de outbox com o evento tipado WagerTransactionPendingReference.
func NewWagerTransactionPendingReferenceOutboxMessage(
	correlationID string,
	causationID string,
	data WagerTransactionPendingReferenceData,
) (*database.OutboxMessage, error) {
	now := time.Now().UTC()
	eventID := uuid.NewString()

	envelope := EventEnvelope{
		EventID:       eventID,
		EventType:     EventTypeWagerTransactionPendingReference,
		AggregateID:   data.TransactionID,
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    now.Format(time.RFC3339),
		Version:       1,
		Data:          data,
	}

	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar evento WagerTransactionPendingReference: %w", err)
	}

	return &database.OutboxMessage{
		ID:            eventID,
		AggregateType: "WagerTransaction",
		AggregateID:   data.TransactionID,
		EventType:     EventTypeWagerTransactionPendingReference,
		Payload:       payloadBytes,
		Status:        database.OutboxStatusPending,
		RetryCount:    0,
		NextAttemptAt: now,
		CreatedAt:     now,
	}, nil
}
