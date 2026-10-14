package worker_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"jungle/internal/infrastructure/database"
	"jungle/internal/infrastructure/worker"

	"github.com/jackc/pgx/v5"
)

type mockTxManager struct{}

func (m *mockTxManager) Execute(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return fn(nil)
}

type mockEventPublisher struct {
	publishedBodies []string
	groupIDs        []string
	dedupIDs        []string
	failErr         error
}

func (m *mockEventPublisher) PublishFIFO(ctx context.Context, queueURL string, body string, messageGroupID string, messageDeduplicationID string) error {
	if m.failErr != nil {
		return m.failErr
	}
	m.publishedBodies = append(m.publishedBodies, body)
	m.groupIDs = append(m.groupIDs, messageGroupID)
	m.dedupIDs = append(m.dedupIDs, messageDeduplicationID)
	return nil
}

type mockOutboxRepo struct {
	pending   []*database.OutboxMessage
	published []string
	retried   map[string]time.Time
	failed    []string
}

func newMockOutboxRepo() *mockOutboxRepo {
	return &mockOutboxRepo{
		retried: make(map[string]time.Time),
	}
}

func (r *mockOutboxRepo) Create(ctx context.Context, tx pgx.Tx, msg *database.OutboxMessage) error {
	r.pending = append(r.pending, msg)
	return nil
}

func (r *mockOutboxRepo) FetchPending(ctx context.Context, tx pgx.Tx, limit int) ([]*database.OutboxMessage, error) {
	if len(r.pending) > limit {
		res := r.pending[:limit]
		r.pending = r.pending[limit:]
		return res, nil
	}
	res := r.pending
	r.pending = nil
	return res, nil
}

func (r *mockOutboxRepo) MarkPublished(ctx context.Context, tx pgx.Tx, id string) error {
	r.published = append(r.published, id)
	return nil
}

func (r *mockOutboxRepo) ScheduleRetry(ctx context.Context, tx pgx.Tx, id string, nextAttemptAt time.Time) error {
	r.retried[id] = nextAttemptAt
	return nil
}

func (r *mockOutboxRepo) MarkFailed(ctx context.Context, tx pgx.Tx, id string) error {
	r.failed = append(r.failed, id)
	return nil
}

func TestOutboxPublisher(t *testing.T) {
	ctx := context.Background()
	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("sucesso: publica mensagens pendentes e marca como PUBLISHED", func(t *testing.T) {
		txMgr := &mockTxManager{}
		repo := newMockOutboxRepo()
		pub := &mockEventPublisher{}

		repo.pending = []*database.OutboxMessage{
			{
				ID:          "event-1",
				AggregateID: "wallet-1",
				EventType:   "WalletBalanceChanged",
				Payload:     []byte(`{"walletId":"wallet-1"}`),
			},
			{
				ID:          "event-2",
				AggregateID: "wallet-1",
				EventType:   "WagerTransactionProcessed",
				Payload:     []byte(`{"transactionId":"tx-1"}`),
			},
		}

		w := worker.NewOutboxPublisher(
			txMgr,
			repo,
			pub,
			worker.OutboxPublisherConfig{
				QueueURL:  "https://sqs.us-east-1.amazonaws.com/123/wager-events.fifo",
				BatchSize: 10,
			},
			discardLogger,
		)

		count, err := w.ProcessBatch(ctx)
		if err != nil {
			t.Fatalf("esperava sucesso no lote, recebeu: %v", err)
		}
		if count != 2 {
			t.Errorf("esperava 2 mensagens processadas, recebeu %d", count)
		}

		if len(pub.publishedBodies) != 2 {
			t.Fatalf("esperava 2 mensagens publicadas no SQS, recebeu %d", len(pub.publishedBodies))
		}
		if pub.groupIDs[0] != "wallet-1" || pub.dedupIDs[0] != "event-1" {
			t.Errorf("metadados FIFO incorretos: groupID=%s, dedupID=%s", pub.groupIDs[0], pub.dedupIDs[0])
		}

		if len(repo.published) != 2 {
			t.Fatalf("esperava 2 mensagens marcadas como published, recebeu %d", len(repo.published))
		}
	})

	t.Run("caso de borda: falha temporaria de publicacao agenda retry com backoff", func(t *testing.T) {
		txMgr := &mockTxManager{}
		repo := newMockOutboxRepo()
		pub := &mockEventPublisher{failErr: errors.New("timeout de rede com SQS")}

		repo.pending = []*database.OutboxMessage{
			{
				ID:          "event-fail-1",
				AggregateID: "wallet-1",
				EventType:   "WalletBalanceChanged",
				Payload:     []byte(`{"amount":"10.00"}`),
				RetryCount:  1,
			},
		}

		w := worker.NewOutboxPublisher(
			txMgr,
			repo,
			pub,
			worker.OutboxPublisherConfig{
				QueueURL:   "https://sqs.us-east-1.amazonaws.com/123/wager-events.fifo",
				MaxRetries: 5,
			},
			discardLogger,
		)

		count, err := w.ProcessBatch(ctx)
		if err != nil {
			t.Fatalf("ProcessBatch nao deve falhar a nivel de batch, recebeu: %v", err)
		}
		if count != 1 {
			t.Errorf("esperava 1 mensagem processada")
		}

		if len(repo.published) != 0 {
			t.Errorf("mensagem nao deveria ser marcada como published")
		}
		if nextAttempt, exists := repo.retried["event-fail-1"]; !exists || nextAttempt.Before(time.Now()) {
			t.Errorf("mensagem deveria estar agendada para retry no futuro")
		}
	})

	t.Run("caso de borda: esgotamento de maxRetries marca como FAILED", func(t *testing.T) {
		txMgr := &mockTxManager{}
		repo := newMockOutboxRepo()
		pub := &mockEventPublisher{failErr: errors.New("falha permanente")}

		repo.pending = []*database.OutboxMessage{
			{
				ID:          "event-exhausted",
				AggregateID: "wallet-1",
				EventType:   "WalletBalanceChanged",
				Payload:     []byte(`{}`),
				RetryCount:  4, // Já falhou 4 vezes, max é 5!
			},
		}

		w := worker.NewOutboxPublisher(
			txMgr,
			repo,
			pub,
			worker.OutboxPublisherConfig{
				QueueURL:   "https://sqs.us-east-1.amazonaws.com/123/wager-events.fifo",
				MaxRetries: 5,
			},
			discardLogger,
		)

		_, _ = w.ProcessBatch(ctx)

		if len(repo.failed) != 1 || repo.failed[0] != "event-exhausted" {
			t.Errorf("evento com retries esgotados deveria ter sido marcado como FAILED")
		}
	})

	t.Run("sucesso: inicio e parada graciosa do outbox publisher", func(t *testing.T) {
		txMgr := &mockTxManager{}
		repo := newMockOutboxRepo()
		pub := &mockEventPublisher{}

		w := worker.NewOutboxPublisher(
			txMgr,
			repo,
			pub,
			worker.OutboxPublisherConfig{
				QueueURL:     "https://sqs.us-east-1.amazonaws.com/123/wager-events.fifo",
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

