package main

import (
	"context"
	"errors"
	"log/slog"
	stdhttp "net/http"
	"os"
	"strings"
	"time"

	"jungle/internal/application"
	"jungle/internal/infrastructure/auth"
	"jungle/internal/infrastructure/aws"
	"jungle/internal/infrastructure/config"
	"jungle/internal/infrastructure/database"
	infrahttp "jungle/internal/infrastructure/http"
	"jungle/internal/infrastructure/messaging"
	"jungle/internal/infrastructure/worker"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
)

var AppModule = fx.Options(
	// 1. Config & Logger
	fx.Provide(
		config.LoadConfig,
		provideLogger,
	),

	// 2. Infraestrutura: Banco de Dados, Transaction Manager & Repositories
	fx.Provide(
		providePostgresPool,
		fx.Annotate(
			database.NewPostgresTransactionManager,
			fx.As(new(application.TransactionManager)),
		),
		fx.Annotate(
			database.NewWalletRepository,
			fx.As(new(application.WalletRepository)),
		),
		fx.Annotate(
			database.NewWalletLedgerEntryRepository,
			fx.As(new(application.LedgerRepository)),
		),
		fx.Annotate(
			database.NewWagerTransactionRepository,
			fx.As(new(application.WagerTransactionRepository)),
		),
		fx.Annotate(
			database.NewOutboxRepository,
			fx.As(new(application.OutboxRepository)),
		),
		fx.Annotate(
			database.NewInboxRepository,
			fx.As(new(application.InboxRepository)),
		),
	),

	// 3. AWS SQS Client, Publishers & Consumers
	fx.Provide(
		provideSQSClient,
		func(c *sqs.Client) messaging.SQSClientAPI { return c },
		messaging.NewSQSPublisher,
		provideSQSConsumer,
	),

	// 4. Background Workers
	fx.Provide(
		provideOutboxPublisher,
		providePendingWorker,
	),

	// 5. Autenticação & Autorização (OIDC Keycloak)
	fx.Provide(
		provideAuthValidator,
	),

	// 6. Casos de Uso (Application Layer)
	fx.Provide(
		application.NewCreateWalletUseCase,
		application.NewGetWalletUseCase,
		application.NewGetWalletLedgerUseCase,
		application.NewProcessWagerUseCase,
		application.NewGetWagerTransactionUseCase,
		provideReconcileUseCase,
		application.NewResolvePendingReferencesUseCase,
	),

	// 7. Camada HTTP (Handlers & Router)
	fx.Provide(
		infrahttp.NewWalletHandler,
		infrahttp.NewWagerHandler,
		infrahttp.NewReconcileHandler,
		infrahttp.NewHealthHandler,
		provideRouter,
	),

	// 8. Registro de Ciclo de Vida (Lifecycle Hooks)
	fx.Invoke(
		registerHTTPServer,
		registerWorkers,
	),
)

// --- PROVIDER FUNCTIONS ---

func provideLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	if strings.ToLower(cfg.LogLevel) == "debug" {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{
		Level: level,
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

func providePostgresPool(lc fx.Lifecycle, cfg *config.Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("fechando pool de conexoes postgres")
			pool.Close()
			return nil
		},
	})

	return pool, nil
}

func provideSQSClient(cfg *config.Config) (*sqs.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return aws.NewSQSClient(ctx, aws.Config{
		Region:   cfg.AWSRegion,
		Endpoint: cfg.AWSEndpoint,
	})
}

func provideAuthValidator(cfg *config.Config) auth.TokenValidator {
	return auth.NewKeycloakOIDCValidator(cfg.KeycloakIssuer, nil)
}

func provideReconcileUseCase(
	tm application.TransactionManager,
	wr application.WalletRepository,
	lr application.LedgerRepository,
) *application.ReconcileWalletUseCase {
	return application.NewReconcileWalletUseCase(tm, wr, lr, nil)
}

func provideRouter(
	walletHandler *infrahttp.WalletHandler,
	wagerHandler *infrahttp.WagerHandler,
	reconcileHandler *infrahttp.ReconcileHandler,
	healthHandler *infrahttp.HealthHandler,
	authValidator auth.TokenValidator,
	logger *slog.Logger,
) stdhttp.Handler {
	return infrahttp.NewRouter(infrahttp.RouterParams{
		WalletHandler:    walletHandler,
		WagerHandler:     wagerHandler,
		ReconcileHandler: reconcileHandler,
		HealthHandler:    healthHandler,
		AuthValidator:    authValidator,
		Logger:           logger,
	})
}

func provideSQSConsumer(
	client *sqs.Client,
	useCase *application.ProcessWagerUseCase,
	cfg *config.Config,
	logger *slog.Logger,
) *messaging.SQSConsumer {
	consumerCfg := messaging.DefaultConsumerConfig(cfg.SQSQueueURL)
	return messaging.NewSQSConsumer(client, useCase, consumerCfg, logger)
}

func provideOutboxPublisher(
	tm application.TransactionManager,
	outboxRepo application.OutboxRepository,
	pub *messaging.SQSPublisher,
	cfg *config.Config,
	logger *slog.Logger,
) *worker.OutboxPublisher {
	outboxCfg := worker.DefaultOutboxPublisherConfig(cfg.SQSEventsQueueURL)
	return worker.NewOutboxPublisher(tm, outboxRepo, pub, outboxCfg, logger)
}

func providePendingWorker(
	useCase *application.ResolvePendingReferencesUseCase,
	logger *slog.Logger,
) *worker.PendingReferenceWorker {
	pendingCfg := worker.DefaultPendingReferenceWorkerConfig()
	return worker.NewPendingReferenceWorker(useCase, pendingCfg, logger)
}

// --- LIFECYCLE REGISTRATIONS ---

func registerHTTPServer(lc fx.Lifecycle, handler stdhttp.Handler, cfg *config.Config, logger *slog.Logger) {
	server := &stdhttp.Server{
		Addr:         cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("servidor HTTP iniciando", slog.String("addr", cfg.Port))
			go func() {
				if err := server.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
					logger.Error("erro fatal no servidor HTTP", slog.String("erro", err.Error()))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("encerrando servidor HTTP graciosamente")
			return server.Shutdown(ctx)
		},
	})
}

func registerWorkers(
	lc fx.Lifecycle,
	consumer *messaging.SQSConsumer,
	outboxPublisher *worker.OutboxPublisher,
	pendingWorker *worker.PendingReferenceWorker,
	logger *slog.Logger,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("iniciando background workers")
			go consumer.Start(context.Background())
			go outboxPublisher.Start(context.Background())
			go pendingWorker.Start(context.Background())
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("solicitando parada segura dos background workers")
			consumer.Stop()
			outboxPublisher.Stop()
			pendingWorker.Stop()
			logger.Info("todos os workers foram finalizados com sucesso")
			return nil
		},
	})
}
