package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"jungle/internal/application"
)

type PendingReferenceWorkerConfig struct {
	BatchSize    int
	PollInterval time.Duration
	TTL          time.Duration
}

func DefaultPendingReferenceWorkerConfig() PendingReferenceWorkerConfig {
	return PendingReferenceWorkerConfig{
		BatchSize:    50,
		PollInterval: 5 * time.Second,
		TTL:          24 * time.Hour,
	}
}

type PendingReferenceWorker struct {
	useCase *application.ResolvePendingReferencesUseCase
	cfg     PendingReferenceWorkerConfig
	logger  *slog.Logger

	wg     sync.WaitGroup
	stopCh chan struct{}
}

func NewPendingReferenceWorker(
	useCase *application.ResolvePendingReferencesUseCase,
	cfg PendingReferenceWorkerConfig,
	logger *slog.Logger,
) *PendingReferenceWorker {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &PendingReferenceWorker{
		useCase: useCase,
		cfg:     cfg,
		logger:  logger,
		stopCh:  make(chan struct{}),
	}
}

func (w *PendingReferenceWorker) Start(ctx context.Context) error {
	w.logger.Info("iniciando worker de referencias pendentes",
		slog.Duration("interval", w.cfg.PollInterval),
		slog.Duration("ttl", w.cfg.TTL),
	)

	w.wg.Add(1)
	go w.pollLoop(ctx)

	return nil
}

func (w *PendingReferenceWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	w.logger.Info("worker de referencias pendentes finalizado")
}

// ProcessBatch executa um ciclo de resolucao (util para testes unitarios e integrados).
func (w *PendingReferenceWorker) ProcessBatch(ctx context.Context) (*application.ResolvePendingReferencesOutput, error) {
	return w.useCase.Execute(ctx, application.ResolvePendingReferencesInput{
		Limit: w.cfg.BatchSize,
		TTL:   w.cfg.TTL,
	})
}

func (w *PendingReferenceWorker) pollLoop(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			output, err := w.ProcessBatch(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Error("erro no ciclo de resolucao de referencias pendentes", slog.Any("error", err))
				continue
			}

			if output != nil && (output.ResolvedCount > 0 || output.RejectedCount > 0) {
				w.logger.Info("ciclo de referencias pendentes concluido",
					slog.Int("checked", output.CheckedCount),
					slog.Int("resolved", output.ResolvedCount),
					slog.Int("rejected", output.RejectedCount),
					slog.Int("remaining", output.RemainingCount),
				)
			}
		}
	}
}

