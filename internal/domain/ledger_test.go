package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewWalletLedgerEntry(t *testing.T) {
	brl100, _ := NewMoney(10000, "BRL")
	brl60, _ := NewMoney(6000, "BRL")
	brl50, _ := NewMoney(5000, "BRL")
	brl40, _ := NewMoney(4000, "BRL")
	brl30, _ := NewMoney(3000, "BRL")
	brl80, _ := NewMoney(8000, "BRL")
	brl90, _ := NewMoney(9000, "BRL")
	brlZero, _ := ZeroMoney("BRL")
	brlNegative, _ := NewMoney(-1000, "BRL")
	usd40, _ := NewMoney(4000, "USD")
	usd80, _ := NewMoney(8000, "USD")

	tests := []struct {
		name          string
		id            string
		walletID      string
		transactionID string
		direction     LedgerDirection
		amount        Money
		balanceBefore Money
		balanceAfter  Money
		expectedErr   error
	}{
		{
			name:          "deve criar lancamento de DEBIT valido",
			id:            "l-001",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirectionDebit,
			amount:        brl40,
			balanceBefore: brl100,
			balanceAfter:  brl60,
			expectedErr:   nil,
		},
		{
			name:          "deve criar lancamento de CREDIT valido",
			id:            "l-002",
			walletID:      "w-001",
			transactionID: "t-002",
			direction:     LedgerDirectionCredit,
			amount:        brl30,
			balanceBefore: brl50,
			balanceAfter:  brl80,
			expectedErr:   nil,
		},
		{
			name:          "deve rejeitar DEBIT com matematica incorreta",
			id:            "l-003",
			walletID:      "w-001",
			transactionID: "t-003",
			direction:     LedgerDirectionDebit,
			amount:        brl40,
			balanceBefore: brl100,
			balanceAfter:  brl50, // 100 - 40 != 50
			expectedErr:   ErrInvalidLedgerMath,
		},
		{
			name:          "deve rejeitar CREDIT com matematica incorreta",
			id:            "l-004",
			walletID:      "w-001",
			transactionID: "t-004",
			direction:     LedgerDirectionCredit,
			amount:        brl30,
			balanceBefore: brl50,
			balanceAfter:  brl90, // 50 + 30 != 90
			expectedErr:   ErrInvalidLedgerMath,
		},
		{
			name:          "deve rejeitar se id for vazio",
			id:            "",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirectionDebit,
			amount:        brl40,
			balanceBefore: brl100,
			balanceAfter:  brl60,
			expectedErr:   ErrInvalidLedgerId,
		},
		{
			name:          "deve rejeitar se wallet id for vazio",
			id:            "l-001",
			walletID:      "",
			transactionID: "t-001",
			direction:     LedgerDirectionDebit,
			amount:        brl40,
			balanceBefore: brl100,
			balanceAfter:  brl60,
			expectedErr:   ErrInvalidWalletId,
		},
		{
			name:          "deve rejeitar se transaction id for vazio",
			id:            "l-001",
			walletID:      "w-001",
			transactionID: "",
			direction:     LedgerDirectionDebit,
			amount:        brl40,
			balanceBefore: brl100,
			balanceAfter:  brl60,
			expectedErr:   ErrInvalidTransactionId,
		},
		{
			name:          "deve rejeitar se direcao for invalida",
			id:            "l-001",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirection("TRANSFER"),
			amount:        brl40,
			balanceBefore: brl100,
			balanceAfter:  brl60,
			expectedErr:   ErrInvalidLedgerDirection,
		},
		{
			name:          "deve rejeitar se amount for zero",
			id:            "l-001",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirectionDebit,
			amount:        brlZero,
			balanceBefore: brl100,
			balanceAfter:  brl100,
			expectedErr:   ErrNegativeAmount,
		},
		{
			name:          "deve rejeitar se amount for negativo",
			id:            "l-001",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirectionDebit,
			amount:        brlNegative,
			balanceBefore: brl100,
			balanceAfter:  brl100,
			expectedErr:   ErrNegativeAmount,
		},
		{
			name:          "deve rejeitar se moeda do amount for divergente",
			id:            "l-001",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirectionDebit,
			amount:        usd40,
			balanceBefore: brl100,
			balanceAfter:  brl60,
			expectedErr:   ErrCurrencyMismatch,
		},
		{
			name:          "deve rejeitar se moeda do balanceAfter for divergente",
			id:            "l-001",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirectionCredit,
			amount:        brl30,
			balanceBefore: brl50,
			balanceAfter:  usd80,
			expectedErr:   ErrCurrencyMismatch,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewWalletLedgerEntry(
				tc.id,
				tc.walletID,
				tc.transactionID,
				tc.direction,
				tc.amount,
				tc.balanceBefore,
				tc.balanceAfter,
			)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result.ID() != tc.id {
					t.Errorf("esperava id %s, mas recebeu %s", tc.id, result.ID())
				}
				if result.WalletID() != tc.walletID {
					t.Errorf("esperava walletID %s, mas recebeu %s", tc.walletID, result.WalletID())
				}
				if result.TransactionID() != tc.transactionID {
					t.Errorf("esperava transactionID %s, mas recebeu %s", tc.transactionID, result.TransactionID())
				}
				if result.Direction() != tc.direction {
					t.Errorf("esperava direction %s, mas recebeu %s", tc.direction, result.Direction())
				}
				if result.Amount().Cents() != tc.amount.Cents() {
					t.Errorf("esperava amount %d, mas recebeu %d", tc.amount.Cents(), result.Amount().Cents())
				}
				if result.BalanceBefore().Cents() != tc.balanceBefore.Cents() {
					t.Errorf("esperava balanceBefore %d, mas recebeu %d", tc.balanceBefore.Cents(), result.BalanceBefore().Cents())
				}
				if result.BalanceAfter().Cents() != tc.balanceAfter.Cents() {
					t.Errorf("esperava balanceAfter %d, mas recebeu %d", tc.balanceAfter.Cents(), result.BalanceAfter().Cents())
				}
				if result.CreatedAt().IsZero() {
					t.Errorf("createdAt nao deve ser zero")
				}
			}
		})
	}
}

func TestRehydrateWalletLedgerEntry(t *testing.T) {
	amount, _ := NewMoney(2500, "BRL")
	balanceBefore, _ := NewMoney(5000, "BRL")
	balanceAfter, _ := NewMoney(7500, "BRL")
	fixedCreatedAt := time.Date(2026, 2, 1, 15, 30, 0, 0, time.UTC)

	tests := []struct {
		name          string
		id            string
		walletID      string
		transactionID string
		direction     LedgerDirection
		amount        Money
		balanceBefore Money
		balanceAfter  Money
		createdAt     time.Time
		expectedErr   error
	}{
		{
			name:          "deve reidratar com sucesso e manter timestamp original",
			id:            "l-rehydrate-1",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirectionCredit,
			amount:        amount,
			balanceBefore: balanceBefore,
			balanceAfter:  balanceAfter,
			createdAt:     fixedCreatedAt,
			expectedErr:   nil,
		},
		{
			name:          "deve rejeitar se id for vazio",
			id:            "",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirectionCredit,
			amount:        amount,
			balanceBefore: balanceBefore,
			balanceAfter:  balanceAfter,
			createdAt:     fixedCreatedAt,
			expectedErr:   ErrInvalidLedgerId,
		},
		{
			name:          "deve rejeitar se wallet id for vazio",
			id:            "l-rehydrate-1",
			walletID:      "",
			transactionID: "t-001",
			direction:     LedgerDirectionCredit,
			amount:        amount,
			balanceBefore: balanceBefore,
			balanceAfter:  balanceAfter,
			createdAt:     fixedCreatedAt,
			expectedErr:   ErrInvalidWalletId,
		},
		{
			name:          "deve rejeitar se transaction id for vazio",
			id:            "l-rehydrate-1",
			walletID:      "w-001",
			transactionID: "",
			direction:     LedgerDirectionCredit,
			amount:        amount,
			balanceBefore: balanceBefore,
			balanceAfter:  balanceAfter,
			createdAt:     fixedCreatedAt,
			expectedErr:   ErrInvalidTransactionId,
		},
		{
			name:          "deve rejeitar se direcao for invalida",
			id:            "l-rehydrate-1",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirection("INVALID"),
			amount:        amount,
			balanceBefore: balanceBefore,
			balanceAfter:  balanceAfter,
			createdAt:     fixedCreatedAt,
			expectedErr:   ErrInvalidLedgerDirection,
		},
		{
			name:          "deve rejeitar se amount for menor ou igual a zero",
			id:            "l-rehydrate-1",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirectionCredit,
			amount:        Money{amountCents: 0, currency: "BRL"},
			balanceBefore: balanceBefore,
			balanceAfter:  balanceAfter,
			createdAt:     fixedCreatedAt,
			expectedErr:   ErrNegativeAmount,
		},
		{
			name:          "deve rejeitar se matematica for incoerente",
			id:            "l-rehydrate-1",
			walletID:      "w-001",
			transactionID: "t-001",
			direction:     LedgerDirectionCredit,
			amount:        amount,
			balanceBefore: balanceBefore,
			balanceAfter:  balanceBefore, // 5000 + 2500 != 5000
			createdAt:     fixedCreatedAt,
			expectedErr:   ErrInvalidLedgerMath,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := RehydrateWalletLedgerEntry(
				tc.id,
				tc.walletID,
				tc.transactionID,
				tc.direction,
				tc.amount,
				tc.balanceBefore,
				tc.balanceAfter,
				tc.createdAt,
			)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result.ID() != tc.id {
					t.Errorf("esperava id %s, mas recebeu %s", tc.id, result.ID())
				}
				if !result.CreatedAt().Equal(tc.createdAt) {
					t.Errorf("esperava createdAt %v, mas recebeu %v", tc.createdAt, result.CreatedAt())
				}
			}
		})
	}
}
