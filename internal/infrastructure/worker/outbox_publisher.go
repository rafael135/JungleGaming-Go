package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"jungle/internal/application"

	"github.com/jackc/pgx/v5"
)

type EventPublisherAPI interface {
	PublishFIFO(ctx context.Context, queueURL string, body string, messageGroupID string, messageDeduplicationID string) error
}

type OutboxPublisherConfig struct {
	QueueURL     string
	BatchSize    int
	PollInterval time.Duration
	MaxRetries   int
}

func DefaultOutboxPublisherConfig(queueURL string) OutboxPublisherConfig {
	return OutboxPublisherConfig{
		QueueURL:     queueURL,
		BatchSize:    20,
		PollInterval: 500 * time.Millisecond,
		MaxRetries:   5,
	}
}

type OutboxPublisher struct {
	txManager  application.TransactionManager
	outboxRepo application.OutboxRepository
	publisher  EventPublisherAPI
	cfg        OutboxPublisherConfig
	logger     *slog.Logger

	wg     sync.WaitGroup
	stopCh chan struct{}
}

func NewOutboxPublisher(
	txManager application.TransactionManager,
	outboxRepo application.OutboxRepository,
	publisher EventPublisherAPI,
	cfg OutboxPublisherConfig,
	logger *slog.Logger,
) *OutboxPublisher {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 20
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &OutboxPublisher{
		txManager:  txManager,
		outboxRepo: outboxRepo,
		publisher:  publisher,
		cfg:        cfg,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

func (p *OutboxPublisher) Start(ctx context.Context) error {
	p.logger.Info("iniciando worker outbox publisher",
		slog.String("queue_url", p.cfg.QueueURL),
		slog.Int("batch_size", p.cfg.BatchSize),
	)

	p.wg.Add(1)
	go p.pollLoop(ctx)

	return nil
}

func (p *OutboxPublisher) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("worker outbox publisher finalizado")
}

// ProcessBatch executa um ciclo único de publicação (útil para testes e acionamento síncrono).
func (p *OutboxPublisher) ProcessBatch(ctx context.Context) (int, error) {
	var count int

	err := p.txManager.Execute(ctx, func(tx pgx.Tx) error {
		messages, err := p.outboxRepo.FetchPending(ctx, tx, p.cfg.BatchSize)
		if err != nil {
			return err
		}

		if len(messages) == 0 {
			return nil
		}

		count = len(messages)

		for _, msg := range messages {
			err := p.publisher.PublishFIFO(
				ctx,
				p.cfg.QueueURL,
				string(msg.Payload),
				msg.AggregateID,
				msg.ID,
			)

			if err != nil {
				p.logger.Error("falha ao publicar evento da outbox no SQS",
					slog.String("event_id", msg.ID),
					slog.String("event_type", msg.EventType),
					slog.Int("current_retry", msg.RetryCount),
					slog.Any("error", err),
				)

				if msg.RetryCount+1 >= p.cfg.MaxRetries {
					if markErr := p.outboxRepo.MarkFailed(ctx, tx, msg.ID); markErr != nil {
						p.logger.Error("erro ao marcar evento como FAILED", slog.Any("error", markErr))
					}
				} else {
					backoffSeconds := 1 << msg.RetryCount
					if backoffSeconds > 300 {
						backoffSeconds = 300
					}
					nextAttempt := time.Now().UTC().Add(time.Duration(backoffSeconds) * time.Second)

					if retryErr := p.outboxRepo.ScheduleRetry(ctx, tx, msg.ID, nextAttempt); retryErr != nil {
						p.logger.Error("erro ao agendar retry do evento", slog.Any("error", retryErr))
					}
				}
			} else {
				if markErr := p.outboxRepo.MarkPublished(ctx, tx, msg.ID); markErr != nil {
					return markErr
				}
				p.logger.Debug("evento da outbox publicado com sucesso",
					slog.String("event_id", msg.ID),
					slog.String("event_type", msg.EventType),
				)
			}
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	return count, nil
}

func (p *OutboxPublisher) pollLoop(ctx context.Context) {
	defer p.wg.Done()

	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			count, err := p.ProcessBatch(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				p.logger.Error("erro no ciclo de publicacao da outbox", slog.Any("error", err))
			}
			if count >= p.cfg.BatchSize {
				select {
				case <-ctx.Done():
					return
				case <-p.stopCh:
					return
				default:
					_, _ = p.ProcessBatch(ctx)
				}
			}
		}
	}
}
