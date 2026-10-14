package database

import (
	"context"
	"errors"
	"fmt"
	"jungle/internal/domain"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrLedgerNotFound = errors.New("ledger not found")

type WalletLedgerEntryRepository struct{}

func NewWalletLedgerEntryRepository() *WalletLedgerEntryRepository {
	return &WalletLedgerEntryRepository{}
}

func (wl *WalletLedgerEntryRepository) GetByTransactionID(
	ctx context.Context,
	tx pgx.Tx,
	id string,
) (*domain.WalletLedgerEntry, error) {
	query := `
		SELECT
			id,
			wallet_id,
			transaction_id,
			direction,
			amount_cents,
			currency,
			balance_before_cents,
			balance_after_cents,
			created_at
		FROM wallet_ledger_entries
		WHERE transaction_id = $1
	`

	var (
		ledgerId       string
		walletID       string
		transactionID  string
		direction      string
		currentBalance int64
		currency       string
		balanceBefore  int64
		balanceAfter   int64
		createdAt      time.Time
	)

	err := tx.QueryRow(ctx, query, id).Scan(
		&ledgerId,
		&walletID,
		&transactionID,
		&direction,
		&currentBalance,
		&currency,
		&balanceBefore,
		&balanceAfter,
		&createdAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrLedgerNotFound
		}

		return nil, fmt.Errorf("erro ao buscar ledger: %w", err)
	}

	currentMoney, err := domain.NewMoney(currentBalance, currency)

	if err != nil {
		return nil, fmt.Errorf("erro ao criar money: %w", err)
	}

	balanceBeforeMoney, err := domain.NewMoney(balanceBefore, currency)

	if err != nil {
		return nil, fmt.Errorf("erro ao criar money: %w", err)
	}

	balanceAfterMoney, err := domain.NewMoney(balanceAfter, currency)

	if err != nil {
		return nil, fmt.Errorf("erro ao criar money: %w", err)
	}

	ledgerDirection := domain.LedgerDirection(direction)

	ledger, err := domain.RehydrateWalletLedgerEntry(
		ledgerId,
		walletID,
		transactionID,
		ledgerDirection,
		currentMoney,
		balanceBeforeMoney,
		balanceAfterMoney,
		createdAt,
	)

	if err != nil {
		return nil, fmt.Errorf("erro ao rehidratar ledger: %w", err)
	}

	return ledger, nil
}

func (wl *WalletLedgerEntryRepository) ListByWalletID(
	ctx context.Context,
	tx pgx.Tx,
	walletID string,
) ([]*domain.WalletLedgerEntry, error) {
	query := `
		SELECT
			id,
			wallet_id,
			transaction_id,
			direction,
			amount_cents,
			currency,
			balance_before_cents,
			balance_after_cents,
			created_at
		FROM wallet_ledger_entries
		WHERE wallet_id = $1
		ORDER BY created_at
	`

	rows, err := tx.Query(ctx, query, walletID)

	if err != nil {
		return nil, fmt.Errorf("erro ao buscar ledger: %w", err)
	}

	defer rows.Close()

	var ledgers = make([]*domain.WalletLedgerEntry, 0)

	for rows.Next() {
		var (
			ledgerId       string
			walletID       string
			transactionID  string
			direction      string
			currentBalance int64
			currency       string
			balanceBefore  int64
			balanceAfter   int64
			createdAt      time.Time
		)

		err := rows.Scan(
			&ledgerId,
			&walletID,
			&transactionID,
			&direction,
			&currentBalance,
			&currency,
			&balanceBefore,
			&balanceAfter,
			&createdAt,
		)

		if err != nil {
			return nil, fmt.Errorf("erro ao buscar ledger: %w", err)
		}

		currentMoney, err := domain.NewMoney(currentBalance, currency)

		if err != nil {
			return nil, fmt.Errorf("erro ao criar money: %w", err)
		}

		balanceBeforeMoney, err := domain.NewMoney(balanceBefore, currency)

		if err != nil {
			return nil, fmt.Errorf("erro ao criar money: %w", err)
		}

		balanceAfterMoney, err := domain.NewMoney(balanceAfter, currency)

		if err != nil {
			return nil, fmt.Errorf("erro ao criar money: %w", err)
		}

		ledgerDirection := domain.LedgerDirection(direction)

		ledger, err := domain.RehydrateWalletLedgerEntry(
			ledgerId,
			walletID,
			transactionID,
			ledgerDirection,
			currentMoney,
			balanceBeforeMoney,
			balanceAfterMoney,
			createdAt,
		)

		if err != nil {
			return nil, fmt.Errorf("erro ao rehidratar ledger: %w", err)
		}

		ledgers = append(ledgers, ledger)
	}

	err = rows.Err()

	if err != nil {
		return nil, fmt.Errorf("erro ao buscar ledger: %w", err)
	}

	return ledgers, nil
}

func (wl *WalletLedgerEntryRepository) Create(
	ctx context.Context,
	tx pgx.Tx,
	l *domain.WalletLedgerEntry,
) error {
	query := `
		INSERT INTO wallet_ledger_entries (
			id,
			wallet_id,
			transaction_id,
			direction,
			amount_cents,
			currency,
			balance_before_cents,
			balance_after_cents,
			created_at
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9
		)
	`

	_, err := tx.Exec(ctx, query,
		l.ID(),
		l.WalletID(),
		l.TransactionID(),
		l.Direction(),
		l.Amount().Cents(),
		l.Amount().Currency(),
		l.BalanceBefore().Cents(),
		l.BalanceAfter().Cents(),
		l.CreatedAt(),
	)

	if err != nil {
		return fmt.Errorf("erro ao criar ledger: %w", err)
	}

	return nil
}
