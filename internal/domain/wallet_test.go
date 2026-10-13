package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewWallet(t *testing.T) {
	validMoney, _ := NewMoney(1000, "BRL")
	zeroMoney, _ := ZeroMoney("BRL")
	negativeMoney, _ := NewMoney(-500, "BRL")

	tests := []struct {
		name            string
		id              string
		playerID        string
		initialBalance  Money
		expectedErr     error
		expectedBalance int64
		expectedVersion int64
	}{
		{
			name:            "deve criar carteira com sucesso",
			id:              "w-123",
			playerID:        "p-456",
			initialBalance:  validMoney,
			expectedErr:     nil,
			expectedBalance: 1000,
			expectedVersion: 1,
		},
		{
			name:            "deve aceitar saldo inicial zero",
			id:              "w-123",
			playerID:        "p-456",
			initialBalance:  zeroMoney,
			expectedErr:     nil,
			expectedBalance: 0,
			expectedVersion: 1,
		},
		{
			name:            "deve rejeitar se id da carteira for vazio",
			id:              "",
			playerID:        "p-456",
			initialBalance:  validMoney,
			expectedErr:     ErrInvalidWalletId,
			expectedBalance: 0,
			expectedVersion: 0,
		},
		{
			name:            "deve rejeitar se player id for vazio",
			id:              "w-123",
			playerID:        "",
			initialBalance:  validMoney,
			expectedErr:     ErrInvalidPlayerId,
			expectedBalance: 0,
			expectedVersion: 0,
		},
		{
			name:            "deve rejeitar se saldo inicial for negativo",
			id:              "w-123",
			playerID:        "p-456",
			initialBalance:  negativeMoney,
			expectedErr:     ErrNegativeAmount,
			expectedBalance: 0,
			expectedVersion: 0,
		},
		{
			name:            "deve rejeitar se moeda for invalida",
			id:              "w-123",
			playerID:        "p-456",
			initialBalance:  Money{amountCents: 1000, currency: "BR"},
			expectedErr:     ErrInvalidCurrency,
			expectedBalance: 0,
			expectedVersion: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewWallet(tc.id, tc.playerID, tc.initialBalance)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result.ID() != tc.id {
					t.Errorf("esperava id %s, mas recebeu %s", tc.id, result.ID())
				}
				if result.PlayerID() != tc.playerID {
					t.Errorf("esperava playerID %s, mas recebeu %s", tc.playerID, result.PlayerID())
				}
				if result.Balance().Cents() != tc.expectedBalance {
					t.Errorf("esperava saldo %d, mas recebeu %d", tc.expectedBalance, result.Balance().Cents())
				}
				if result.Version() != tc.expectedVersion {
					t.Errorf("esperava versao %d, mas recebeu %d", tc.expectedVersion, result.Version())
				}
				if result.Currency() != tc.initialBalance.Currency() {
					t.Errorf("esperava moeda %s, mas recebeu %s", tc.initialBalance.Currency(), result.Currency())
				}
				if result.CreatedAt().IsZero() || result.UpdatedAt().IsZero() {
					t.Errorf("timestamps nao devem ser zero")
				}
			}
		})
	}
}

func TestRehydrateWallet(t *testing.T) {
	validBalance, _ := NewMoney(5000, "BRL")
	negativeBalance, _ := NewMoney(-500, "BRL")
	fixedCreatedAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	fixedUpdatedAt := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name            string
		id              string
		playerID        string
		currency        string
		balance         Money
		version         int64
		createdAt       time.Time
		updatedAt       time.Time
		expectedErr     error
		expectedVersion int64
	}{
		{
			name:            "deve reidratar carteira com sucesso preservando estado",
			id:              "w-999",
			playerID:        "p-888",
			currency:        "BRL",
			balance:         validBalance,
			version:         15,
			createdAt:       fixedCreatedAt,
			updatedAt:       fixedUpdatedAt,
			expectedErr:     nil,
			expectedVersion: 15,
		},
		{
			name:            "deve rejeitar se id for vazio",
			id:              "",
			playerID:        "p-888",
			currency:        "BRL",
			balance:         validBalance,
			version:         15,
			createdAt:       fixedCreatedAt,
			updatedAt:       fixedUpdatedAt,
			expectedErr:     ErrInvalidWalletId,
			expectedVersion: 0,
		},
		{
			name:            "deve rejeitar se player id for vazio",
			id:              "w-999",
			playerID:        "",
			currency:        "BRL",
			balance:         validBalance,
			version:         15,
			createdAt:       fixedCreatedAt,
			updatedAt:       fixedUpdatedAt,
			expectedErr:     ErrInvalidPlayerId,
			expectedVersion: 0,
		},
		{
			name:            "deve rejeitar se moeda for invalida",
			id:              "w-999",
			playerID:        "p-888",
			currency:        "BR",
			balance:         validBalance,
			version:         15,
			createdAt:       fixedCreatedAt,
			updatedAt:       fixedUpdatedAt,
			expectedErr:     ErrInvalidCurrency,
			expectedVersion: 0,
		},
		{
			name:            "deve rejeitar se moeda do saldo nao bater com moeda da carteira",
			id:              "w-999",
			playerID:        "p-888",
			currency:        "USD",
			balance:         validBalance,
			version:         15,
			createdAt:       fixedCreatedAt,
			updatedAt:       fixedUpdatedAt,
			expectedErr:     ErrWalletCurrencyMismatch,
			expectedVersion: 0,
		},
		{
			name:            "deve rejeitar se saldo for negativo",
			id:              "w-999",
			playerID:        "p-888",
			currency:        "BRL",
			balance:         negativeBalance,
			version:         15,
			createdAt:       fixedCreatedAt,
			updatedAt:       fixedUpdatedAt,
			expectedErr:     ErrNegativeAmount,
			expectedVersion: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := RehydrateWallet(tc.id, tc.playerID, tc.currency, tc.balance, tc.version, tc.createdAt, tc.updatedAt)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result.ID() != tc.id {
					t.Errorf("esperava id %s, mas recebeu %s", tc.id, result.ID())
				}
				if result.PlayerID() != tc.playerID {
					t.Errorf("esperava playerID %s, mas recebeu %s", tc.playerID, result.PlayerID())
				}
				if result.Version() != tc.expectedVersion {
					t.Errorf("esperava versao %d, mas recebeu %d", tc.expectedVersion, result.Version())
				}
				if !result.CreatedAt().Equal(tc.createdAt) {
					t.Errorf("esperava createdAt %v, mas recebeu %v", tc.createdAt, result.CreatedAt())
				}
				if !result.UpdatedAt().Equal(tc.updatedAt) {
					t.Errorf("esperava updatedAt %v, mas recebeu %v", tc.updatedAt, result.UpdatedAt())
				}
			}
		})
	}
}

func TestWallet_Debit(t *testing.T) {
	initialMoney, _ := NewMoney(10000, "BRL")
	debitValid, _ := NewMoney(4000, "BRL")
	debitExact, _ := NewMoney(10000, "BRL")
	debitExcessive, _ := NewMoney(15000, "BRL")
	debitDifferentCurrency, _ := NewMoney(4000, "USD")
	debitZero, _ := ZeroMoney("BRL")
	debitNegative, _ := NewMoney(-2000, "BRL")

	tests := []struct {
		name            string
		initialBalance  Money
		debitAmount     Money
		expectedErr     error
		expectedBalance int64
		expectedVersion int64
	}{
		{
			name:            "deve debitar com sucesso e incrementar versao",
			initialBalance:  initialMoney,
			debitAmount:     debitValid,
			expectedErr:     nil,
			expectedBalance: 6000,
			expectedVersion: 2,
		},
		{
			name:            "deve debitar saldo exato zerando a carteira",
			initialBalance:  initialMoney,
			debitAmount:     debitExact,
			expectedErr:     nil,
			expectedBalance: 0,
			expectedVersion: 2,
		},
		{
			name:            "deve rejeitar se saldo for insuficiente e manter versao e saldo intactos",
			initialBalance:  initialMoney,
			debitAmount:     debitExcessive,
			expectedErr:     ErrInsufficientBalance,
			expectedBalance: 10000,
			expectedVersion: 1,
		},
		{
			name:            "deve rejeitar se moeda for incompativel",
			initialBalance:  initialMoney,
			debitAmount:     debitDifferentCurrency,
			expectedErr:     ErrWalletCurrencyMismatch,
			expectedBalance: 10000,
			expectedVersion: 1,
		},
		{
			name:            "deve rejeitar debito com valor zero",
			initialBalance:  initialMoney,
			debitAmount:     debitZero,
			expectedErr:     ErrNegativeAmount,
			expectedBalance: 10000,
			expectedVersion: 1,
		},
		{
			name:            "deve rejeitar debito com valor negativo",
			initialBalance:  initialMoney,
			debitAmount:     debitNegative,
			expectedErr:     ErrNegativeAmount,
			expectedBalance: 10000,
			expectedVersion: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wallet, err := NewWallet("w-1", "p-1", tc.initialBalance)
			if err != nil {
				t.Fatalf("falha ao criar carteira para teste: %v", err)
			}

			err = wallet.Debit(tc.debitAmount)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if wallet.Balance().Cents() != tc.expectedBalance {
				t.Errorf("esperava saldo %d, mas recebeu %d", tc.expectedBalance, wallet.Balance().Cents())
			}

			if wallet.Version() != tc.expectedVersion {
				t.Errorf("esperava versao %d, mas recebeu %d", tc.expectedVersion, wallet.Version())
			}
		})
	}
}

func TestWallet_Credit(t *testing.T) {
	initialMoney, _ := NewMoney(5000, "BRL")
	creditValid, _ := NewMoney(2500, "BRL")
	creditDifferentCurrency, _ := NewMoney(2500, "USD")
	creditZero, _ := ZeroMoney("BRL")
	creditNegative, _ := NewMoney(-1000, "BRL")

	tests := []struct {
		name            string
		initialBalance  Money
		creditAmount    Money
		expectedErr     error
		expectedBalance int64
		expectedVersion int64
	}{
		{
			name:            "deve creditar com sucesso e incrementar versao",
			initialBalance:  initialMoney,
			creditAmount:    creditValid,
			expectedErr:     nil,
			expectedBalance: 7500,
			expectedVersion: 2,
		},
		{
			name:            "deve rejeitar credito com moeda diferente",
			initialBalance:  initialMoney,
			creditAmount:    creditDifferentCurrency,
			expectedErr:     ErrWalletCurrencyMismatch,
			expectedBalance: 5000,
			expectedVersion: 1,
		},
		{
			name:            "deve rejeitar credito com valor zero",
			initialBalance:  initialMoney,
			creditAmount:    creditZero,
			expectedErr:     ErrNegativeAmount,
			expectedBalance: 5000,
			expectedVersion: 1,
		},
		{
			name:            "deve rejeitar credito com valor negativo",
			initialBalance:  initialMoney,
			creditAmount:    creditNegative,
			expectedErr:     ErrNegativeAmount,
			expectedBalance: 5000,
			expectedVersion: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wallet, err := NewWallet("w-1", "p-1", tc.initialBalance)
			if err != nil {
				t.Fatalf("falha ao criar carteira para teste: %v", err)
			}

			err = wallet.Credit(tc.creditAmount)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if wallet.Balance().Cents() != tc.expectedBalance {
				t.Errorf("esperava saldo %d, mas recebeu %d", tc.expectedBalance, wallet.Balance().Cents())
			}

			if wallet.Version() != tc.expectedVersion {
				t.Errorf("esperava versao %d, mas recebeu %d", tc.expectedVersion, wallet.Version())
			}
		})
	}
}
