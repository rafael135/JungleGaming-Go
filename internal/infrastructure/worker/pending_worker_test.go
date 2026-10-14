package worker_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"jungle/internal/application"
	"jungle/internal/domain"
	"jungle/internal/infrastructure/worker"

	"github.com/jackc/pgx/v5"
)

type mockWagerRepoForPending struct {
	pending []*domain.WagerTransaction
}

func (m *mockWagerRepoForPending) Create(ctx context.Context, tx pgx.Tx, t *domain.WagerTransaction) error {
	m.pending = append(m.pending, t)
	return nil
}
func (m *mockWagerRepoForPending) Update(ctx context.Context, tx pgx.Tx, t *domain.WagerTransaction) error {
	return nil
}
func (m *mockWagerRepoForPending) GetByID(ctx context.Context, tx pgx.Tx, id string) (*domain.WagerTransaction, error) {
	return nil, nil
}
func (m *mockWagerRepoForPending) GetByIdempotencyKey(ctx context.Context, tx pgx.Tx, providerID string, idempotencyKey string) (*domain.WagerTransaction, error) {
	return nil, nil
}
func (m *mockWagerRepoForPending) GetByExternalID(ctx context.Context, tx pgx.Tx, providerID string, externalID string) (*domain.WagerTransaction, error) {
	return nil, nil
}
func (m *mockWagerRepoForPending) FetchPendingReferences(ctx context.Context, tx pgx.Tx, limit int) ([]*domain.WagerTransaction, error) {
	return m.pending, nil
}

type mockWalletRepoForPending struct{}

func (m *mockWalletRepoForPending) GetByID(ctx context.Context, tx pgx.Tx, id string, forUpdate bool) (*domain.Wallet, error) {
	return nil, nil
}
func (m *mockWalletRepoForPending) Create(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error {
	return nil
}
func (m *mockWalletRepoForPending) Update(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error {
	return nil
}

type mockLedgerRepoForPending struct{}

func (m *mockLedgerRepoForPending) Create(ctx context.Context, tx pgx.Tx, l *domain.WalletLedgerEntry) error {
	return nil
}
func (m *mockLedgerRepoForPending) GetByTransactionID(ctx context.Context, tx pgx.Tx, id string) (*domain.WalletLedgerEntry, error) {
	return nil, nil
}
func (m *mockLedgerRepoForPending) ListByWalletID(ctx context.Context, tx pgx.Tx, walletID string) ([]*domain.WalletLedgerEntry, error) {
	return nil, nil
}
func (m *mockLedgerRepoForPending) ListPaginated(ctx context.Context, tx pgx.Tx, walletID string, limit int, cursorCreatedAt *time.Time, cursorID string) ([]*domain.WalletLedgerEntry, error) {
	return nil, nil
}

func TestPendingReferenceWorker(t *testing.T) {
	ctx := context.Background()
	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	txMgr := &mockTxManager{}
	wagerRepo := &mockWagerRepoForPending{}
	outboxRepo := newMockOutboxRepo()
	walletRepo := &mockWalletRepoForPending{}
	ledgerRepo := &mockLedgerRepoForPending{}

	useCase := application.NewResolvePendingReferencesUseCase(
		txMgr,
		walletRepo,
		ledgerRepo,
		wagerRepo,
		outboxRepo,
	)

	t.Run("sucesso: executa lote do worker de referencias pendentes", func(t *testing.T) {
		w := worker.NewPendingReferenceWorker(
			useCase,
			worker.PendingReferenceWorkerConfig{
				BatchSize: 10,
				TTL:       24 * time.Hour,
			},
			discardLogger,
		)

		output, err := w.ProcessBatch(ctx)
		if err != nil {
			t.Fatalf("esperava sucesso no lote, recebeu: %v", err)
		}
		if output.CheckedCount != 0 {
			t.Errorf("esperava 0 transacoes pendentes, recebeu %d", output.CheckedCount)
		}
	})

	t.Run("sucesso: inicio e parada graciosa do worker", func(t *testing.T) {
		w := worker.NewPendingReferenceWorker(
			useCase,
			worker.PendingReferenceWorkerConfig{
				PollInterval: 10 * time.Millisecond,
			},
			discardLogger,
		)

		runCtx, cancel := context.WithCancel(ctx)
		if err := w.Start(runCtx); err != nil {
			t.Fatalf("erro ao iniciar worker: %v", err)
		}

		time.Sleep(30 * time.Millisecond)
		cancel()
		w.Stop()
	})
}
