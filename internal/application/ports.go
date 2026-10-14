package application

import (
	"context"
	"jungle/internal/domain"
	"jungle/internal/infrastructure/database"
	"time"

	"github.com/jackc/pgx/v5"
)

type TransactionManager interface {
	Execute(ctx context.Context, fn func(tx pgx.Tx) error) error
}

type WalletRepository interface {
	GetByID(ctx context.Context, tx pgx.Tx, id string, forUpdate bool) (*domain.Wallet, error)
	Create(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error
	Update(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error
}

type LedgerRepository interface {
	GetByTransactionID(ctx context.Context, tx pgx.Tx, id string) (*domain.WalletLedgerEntry, error)
	ListByWalletID(ctx context.Context, tx pgx.Tx, walletID string) ([]*domain.WalletLedgerEntry, error)
	Create(ctx context.Context, tx pgx.Tx, l *domain.WalletLedgerEntry) error
}

type WagerTransactionRepository interface {
	GetByID(ctx context.Context, tx pgx.Tx, id string) (*domain.WagerTransaction, error)
	GetByIdempotencyKey(ctx context.Context, tx pgx.Tx, providerID string, idempotencyKey string) (*domain.WagerTransaction, error)
	GetByExternalID(ctx context.Context, tx pgx.Tx, providerID string, externalID string) (*domain.WagerTransaction, error)
	Create(ctx context.Context, tx pgx.Tx, t *domain.WagerTransaction) error
	Update(ctx context.Context, tx pgx.Tx, t *domain.WagerTransaction) error
}

type OutboxRepository interface {
	Create(ctx context.Context, tx pgx.Tx, msg *database.OutboxMessage) error
	FetchPending(ctx context.Context, tx pgx.Tx, limit int) ([]*database.OutboxMessage, error)
	MarkPublished(ctx context.Context, tx pgx.Tx, id string) error
	ScheduleRetry(ctx context.Context, tx pgx.Tx, id string, nextAttemptAt time.Time) error
	MarkFailed(ctx context.Context, tx pgx.Tx, id string) error
}

type InboxRepository interface {
	TryRecord(ctx context.Context, tx pgx.Tx, consumerName string, messageID string, payloadHash string) (bool, error)
	Get(ctx context.Context, tx pgx.Tx, consumerName string, messageID string) (*database.InboxMessage, error)
	MarkProcessed(ctx context.Context, tx pgx.Tx, consumerName string, messageID string) error
}
