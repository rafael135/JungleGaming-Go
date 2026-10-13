package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewWagerTransaction(t *testing.T) {
	amount50, _ := NewMoney(5000, "BRL")
	amount100, _ := NewMoney(10000, "BRL")
	amountZero, _ := ZeroMoney("BRL")

	tests := []struct {
		name                           string
		id                             string
		externalTransactionID          string
		providerID                     string
		idempotencyKey                 string
		payloadHash                    string
		walletID                       string
		playerID                       string
		gameID                         string
		roundID                        string
		transactionType                TransactionType
		amount                         Money
		referenceExternalTransactionID string
		expectedErr                    error
	}{
		{
			name:                           "deve criar transacao BET valida",
			id:                             "tx-1",
			externalTransactionID:          "ext-1",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-1",
			payloadHash:                    "hash-1",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeBet,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    nil,
		},
		{
			name:                           "deve criar transacao WIN valida",
			id:                             "tx-2",
			externalTransactionID:          "ext-2",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-2",
			payloadHash:                    "hash-2",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeWin,
			amount:                         amount100,
			referenceExternalTransactionID: "",
			expectedErr:                    nil,
		},
		{
			name:                           "deve criar transacao LOSS com valor zero",
			id:                             "tx-3",
			externalTransactionID:          "ext-3",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-3",
			payloadHash:                    "hash-3",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeLoss,
			amount:                         amountZero,
			referenceExternalTransactionID: "",
			expectedErr:                    nil,
		},
		{
			name:                           "deve criar transacao REFUND valida com referencia",
			id:                             "tx-4",
			externalTransactionID:          "ext-4",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-4",
			payloadHash:                    "hash-4",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeRefund,
			amount:                         amount50,
			referenceExternalTransactionID: "ext-1",
			expectedErr:                    nil,
		},
		{
			name:                           "deve criar transacao ROLLBACK valida com referencia",
			id:                             "tx-5",
			externalTransactionID:          "ext-5",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-5",
			payloadHash:                    "hash-5",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeRollback,
			amount:                         amount50,
			referenceExternalTransactionID: "ext-1",
			expectedErr:                    nil,
		},
		{
			name:                           "deve rejeitar REFUND sem referencia externa",
			id:                             "tx-6",
			externalTransactionID:          "ext-6",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-6",
			payloadHash:                    "hash-6",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeRefund,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidReferenceExternalTransactionId,
		},
		{
			name:                           "deve rejeitar ROLLBACK sem referencia externa",
			id:                             "tx-7",
			externalTransactionID:          "ext-7",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-7",
			payloadHash:                    "hash-7",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeRollback,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidReferenceExternalTransactionId,
		},
		{
			name:                           "deve rejeitar LOSS com valor maior que zero",
			id:                             "tx-8",
			externalTransactionID:          "ext-8",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-8",
			payloadHash:                    "hash-8",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeLoss,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidAmountFormat,
		},
		{
			name:                           "deve rejeitar BET com valor zero",
			id:                             "tx-9",
			externalTransactionID:          "ext-9",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-9",
			payloadHash:                    "hash-9",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeBet,
			amount:                         amountZero,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrNegativeAmount,
		},
		{
			name:                           "deve rejeitar se id for vazio",
			id:                             "",
			externalTransactionID:          "ext-1",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-1",
			payloadHash:                    "hash-1",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeBet,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidTransactionId,
		},
		{
			name:                           "deve rejeitar se externalTransactionID for vazio",
			id:                             "tx-1",
			externalTransactionID:          "",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-1",
			payloadHash:                    "hash-1",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeBet,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidExternalTransactionId,
		},
		{
			name:                           "deve rejeitar se providerID for vazio",
			id:                             "tx-1",
			externalTransactionID:          "ext-1",
			providerID:                     "",
			idempotencyKey:                 "idem-1",
			payloadHash:                    "hash-1",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeBet,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidProviderId,
		},
		{
			name:                           "deve rejeitar se idempotencyKey for vazio",
			id:                             "tx-1",
			externalTransactionID:          "ext-1",
			providerID:                     "provider-a",
			idempotencyKey:                 "",
			payloadHash:                    "hash-1",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeBet,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidIdempotencyKey,
		},
		{
			name:                           "deve rejeitar se payloadHash for vazio",
			id:                             "tx-1",
			externalTransactionID:          "ext-1",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-1",
			payloadHash:                    "",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeBet,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidPayloadHash,
		},
		{
			name:                           "deve rejeitar se walletID for vazio",
			id:                             "tx-1",
			externalTransactionID:          "ext-1",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-1",
			payloadHash:                    "hash-1",
			walletID:                       "",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeBet,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidWalletId,
		},
		{
			name:                           "deve rejeitar se playerID for vazio",
			id:                             "tx-1",
			externalTransactionID:          "ext-1",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-1",
			payloadHash:                    "hash-1",
			walletID:                       "w-1",
			playerID:                       "",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionTypeBet,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidPlayerId,
		},
		{
			name:                           "deve rejeitar se gameID for vazio",
			id:                             "tx-1",
			externalTransactionID:          "ext-1",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-1",
			payloadHash:                    "hash-1",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "",
			roundID:                        "round-1",
			transactionType:                TransactionTypeBet,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidGameId,
		},
		{
			name:                           "deve rejeitar se roundID for vazio",
			id:                             "tx-1",
			externalTransactionID:          "ext-1",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-1",
			payloadHash:                    "hash-1",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "",
			transactionType:                TransactionTypeBet,
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidRoundId,
		},
		{
			name:                           "deve rejeitar se tipo de transacao for desconhecido",
			id:                             "tx-1",
			externalTransactionID:          "ext-1",
			providerID:                     "provider-a",
			idempotencyKey:                 "idem-1",
			payloadHash:                    "hash-1",
			walletID:                       "w-1",
			playerID:                       "p-1",
			gameID:                         "game-1",
			roundID:                        "round-1",
			transactionType:                TransactionType("ROULETTE"),
			amount:                         amount50,
			referenceExternalTransactionID: "",
			expectedErr:                    ErrInvalidTransactionType,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewWagerTransaction(
				tc.id,
				tc.externalTransactionID,
				tc.providerID,
				tc.idempotencyKey,
				tc.payloadHash,
				tc.walletID,
				tc.playerID,
				tc.gameID,
				tc.roundID,
				tc.transactionType,
				tc.amount,
				tc.referenceExternalTransactionID,
			)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result.Status() != TransactionStatusPending {
					t.Errorf("esperava status inicial PENDING, mas recebeu %s", result.Status())
				}
				if result.FailureCode() != "" {
					t.Errorf("failureCode inicial deve ser vazio, recebeu %s", result.FailureCode())
				}
				if result.ID() != tc.id {
					t.Errorf("esperava id %s, recebeu %s", tc.id, result.ID())
				}
				if result.CreatedAt().IsZero() || result.UpdatedAt().IsZero() {
					t.Errorf("timestamps nao devem ser zero")
				}
			}
		})
	}
}

func TestNewOpeningTransaction(t *testing.T) {
	amount100, _ := NewMoney(10000, "BRL")
	amountZero, _ := ZeroMoney("BRL")
	amountNegative, _ := NewMoney(-500, "BRL")

	tests := []struct {
		name        string
		id          string
		walletID    string
		playerID    string
		amount      Money
		expectedErr error
	}{
		{
			name:        "deve criar transacao de abertura valida",
			id:          "tx-op-1",
			walletID:    "w-1",
			playerID:    "p-1",
			amount:      amount100,
			expectedErr: nil,
		},
		{
			name:        "deve aceitar abertura com valor zero",
			id:          "tx-op-2",
			walletID:    "w-1",
			playerID:    "p-1",
			amount:      amountZero,
			expectedErr: nil,
		},
		{
			name:        "deve rejeitar abertura com saldo negativo",
			id:          "tx-op-3",
			walletID:    "w-1",
			playerID:    "p-1",
			amount:      amountNegative,
			expectedErr: ErrNegativeAmount,
		},
		{
			name:        "deve rejeitar se id for vazio",
			id:          "",
			walletID:    "w-1",
			playerID:    "p-1",
			amount:      amount100,
			expectedErr: ErrInvalidTransactionId,
		},
		{
			name:        "deve rejeitar se walletID for vazio",
			id:          "tx-op-1",
			walletID:    "",
			playerID:    "p-1",
			amount:      amount100,
			expectedErr: ErrInvalidWalletId,
		},
		{
			name:        "deve rejeitar se playerID for vazio",
			id:          "tx-op-1",
			walletID:    "w-1",
			playerID:    "",
			amount:      amount100,
			expectedErr: ErrInvalidPlayerId,
		},
		{
			name:        "deve rejeitar se moeda for invalida",
			id:          "tx-op-1",
			walletID:    "w-1",
			playerID:    "p-1",
			amount:      Money{amountCents: 1000, currency: "BR"},
			expectedErr: ErrInvalidCurrency,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewOpeningTransaction(tc.id, tc.walletID, tc.playerID, tc.amount)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result.Status() != TransactionStatusProcessed {
					t.Errorf("abertura deve nascer PROCESSED, mas recebeu %s", result.Status())
				}
				if result.TransactionType() != TransactionTypeOpening {
					t.Errorf("tipo deve ser OPENING, mas recebeu %s", result.TransactionType())
				}
			}
		})
	}
}

func TestWagerTransaction_StateMachine(t *testing.T) {
	amount, _ := NewMoney(5000, "BRL")

	createTx := func() *WagerTransaction {
		tx, _ := NewWagerTransaction(
			"tx-test",
			"ext-1",
			"provider-a",
			"idem-1",
			"hash-1",
			"w-1",
			"p-1",
			"game-1",
			"round-1",
			TransactionTypeBet,
			amount,
			"",
		)
		return tx
	}

	t.Run("PENDING -> PROCESSED", func(t *testing.T) {
		tx := createTx()
		err := tx.MarkProcessed()
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu %v", err)
		}
		if tx.Status() != TransactionStatusProcessed {
			t.Errorf("esperava PROCESSED, recebeu %s", tx.Status())
		}
		if !tx.IsTerminal() {
			t.Errorf("PROCESSED deve ser considerado terminal")
		}
	})

	t.Run("PENDING -> PENDING_REFERENCE -> PROCESSED", func(t *testing.T) {
		tx := createTx()

		err := tx.MarkPendingReference()
		if err != nil {
			t.Fatalf("esperava sucesso ao marcar PENDING_REFERENCE, recebeu %v", err)
		}
		if tx.Status() != TransactionStatusPendingReference {
			t.Errorf("esperava PENDING_REFERENCE, recebeu %s", tx.Status())
		}
		if tx.IsTerminal() {
			t.Errorf("PENDING_REFERENCE nao deve ser terminal")
		}

		err = tx.MarkProcessed()
		if err != nil {
			t.Fatalf("esperava sucesso ao resolver para PROCESSED, recebeu %v", err)
		}
		if tx.Status() != TransactionStatusProcessed {
			t.Errorf("esperava PROCESSED, recebeu %s", tx.Status())
		}
	})

	t.Run("PENDING -> REJECTED grava failureCode", func(t *testing.T) {
		tx := createTx()
		err := tx.MarkRejected("INSUFFICIENT_FUNDS")
		if err != nil {
			t.Fatalf("esperava sucesso ao marcar REJECTED, recebeu %v", err)
		}
		if tx.Status() != TransactionStatusRejected {
			t.Errorf("esperava REJECTED, recebeu %s", tx.Status())
		}
		if tx.FailureCode() != "INSUFFICIENT_FUNDS" {
			t.Errorf("esperava failureCode INSUFFICIENT_FUNDS, recebeu %s", tx.FailureCode())
		}
		if !tx.IsTerminal() {
			t.Errorf("REJECTED deve ser terminal")
		}
	})

	t.Run("PENDING -> FAILED grava failureCode", func(t *testing.T) {
		tx := createTx()
		err := tx.MarkFailed("DATABASE_TIMEOUT")
		if err != nil {
			t.Fatalf("esperava sucesso ao marcar FAILED, recebeu %v", err)
		}
		if tx.Status() != TransactionStatusFailed {
			t.Errorf("esperava FAILED, recebeu %s", tx.Status())
		}
		if tx.FailureCode() != "DATABASE_TIMEOUT" {
			t.Errorf("esperava failureCode DATABASE_TIMEOUT, recebeu %s", tx.FailureCode())
		}
		if !tx.IsTerminal() {
			t.Errorf("FAILED deve ser terminal")
		}
	})

	t.Run("Estado Terminal PROCESSED nao deve sofrer novas transicoes", func(t *testing.T) {
		tx := createTx()
		_ = tx.MarkProcessed()

		if err := tx.MarkProcessed(); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
		if err := tx.MarkPendingReference(); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
		if err := tx.MarkRejected("TEST"); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
		if err := tx.MarkFailed("TEST"); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
	})

	t.Run("Estado Terminal REJECTED nao deve sofrer novas transicoes", func(t *testing.T) {
		tx := createTx()
		_ = tx.MarkRejected("DUPLICATE_BET")

		if err := tx.MarkProcessed(); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
		if err := tx.MarkPendingReference(); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
		if err := tx.MarkRejected("ANOTHER"); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
		if err := tx.MarkFailed("ANOTHER"); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
	})

	t.Run("Estado Terminal FAILED nao deve sofrer novas transicoes", func(t *testing.T) {
		tx := createTx()
		_ = tx.MarkFailed("FATAL_ERROR")

		if err := tx.MarkProcessed(); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
		if err := tx.MarkPendingReference(); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
		if err := tx.MarkRejected("TEST"); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
		if err := tx.MarkFailed("TEST"); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
	})

	t.Run("PENDING_REFERENCE nao pode transitar para si mesmo", func(t *testing.T) {
		tx := createTx()
		_ = tx.MarkPendingReference()

		if err := tx.MarkPendingReference(); !errors.Is(err, ErrInvalidStateTransition) {
			t.Errorf("esperava ErrInvalidStateTransition, recebeu %v", err)
		}
	})
}

func TestRehydrateWagerTransaction(t *testing.T) {
	amount, _ := NewMoney(5000, "BRL")
	now := time.Now().UTC()

	result, err := RehydrateWagerTransaction(
		"tx-rehydrate-1",
		"ext-1",
		"provider-a",
		"idem-1",
		"hash-1",
		"w-1",
		"p-1",
		"game-1",
		"round-1",
		TransactionTypeBet,
		amount,
		"",
		TransactionStatusProcessed,
		"",
		now,
		now,
	)

	if err != nil {
		t.Fatalf("esperava sucesso na reidratacao, recebeu %v", err)
	}

	if result.ID() != "tx-rehydrate-1" {
		t.Errorf("esperava ID tx-rehydrate-1, recebeu %s", result.ID())
	}
	if result.Status() != TransactionStatusProcessed {
		t.Errorf("esperava status PROCESSED, recebeu %s", result.Status())
	}
	if result.PayloadHash() != "hash-1" {
		t.Errorf("esperava payloadHash hash-1, recebeu %s", result.PayloadHash())
	}
	if result.UpdatedAt() != now {
		t.Errorf("esperava updatedAt %v, recebeu %v", now, result.UpdatedAt())
	}
}
