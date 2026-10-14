package messaging_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"jungle/internal/application"
	"jungle/internal/domain"
	awsclient "jungle/internal/infrastructure/aws"
	"jungle/internal/infrastructure/database"
	"jungle/internal/infrastructure/messaging"
	"jungle/internal/infrastructure/worker"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://jungle:jungle_secret@localhost:5432/jungle_db?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, connStr)
	if err != nil {
		t.Skipf("PostgreSQL nao disponivel para teste de integracao: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

func getIntegrationSQS(t *testing.T) (*sqs.Client, string, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	endpoint := os.Getenv("LOCALSTACK_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:4566"
	}

	client, err := awsclient.NewSQSClient(ctx, awsclient.Config{
		Region:   "us-east-1",
		Endpoint: endpoint,
	})
	if err != nil {
		t.Skipf("LocalStack SQS nao configuravel: %v", err)
	}

	// Obtém URL da fila de entrada
	inUrlOut, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String("wager-transactions.fifo"),
	})
	if err != nil {
		t.Skipf("fila wager-transactions.fifo nao encontrada no LocalStack: %v", err)
	}

	// Obtém URL da fila de saída
	outUrlOut, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String("wager-events.fifo"),
	})
	if err != nil {
		t.Skipf("fila wager-events.fifo nao encontrada no LocalStack: %v", err)
	}

	return client, *inUrlOut.QueueUrl, *outUrlOut.QueueUrl
}

func TestMessagingAndOutbox_LiveIntegration(t *testing.T) {
	pool := getIntegrationPool(t)
	sqsClient, inQueueURL, outQueueURL := getIntegrationSQS(t)
	ctx := context.Background()
	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 1. Cria carteira inicial de 100.00 BRL no PostgreSQL
	playerID := uuid.NewString()
	walletID := uuid.NewString()
	m, _ := domain.NewMoney(10000, "BRL")
	wallet, _ := domain.NewWallet(walletID, playerID, m)

	walletRepo := database.NewWalletRepository()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro ao iniciar tx: %v", err)
	}
	if err := walletRepo.Create(ctx, tx, wallet); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("erro ao criar carteira: %v", err)
	}
	_ = tx.Commit(ctx)

	// 2. Monta casos de uso com banco real
	txMgr := database.NewPostgresTransactionManager(pool)
	ledgerRepo := database.NewWalletLedgerEntryRepository()
	wagerRepo := database.NewWagerTransactionRepository()
	outboxRepo := database.NewOutboxRepository()
	inboxRepo := database.NewInboxRepository()

	processWagerUseCase := application.NewProcessWagerUseCase(
		txMgr,
		walletRepo,
		ledgerRepo,
		wagerRepo,
		outboxRepo,
		inboxRepo,
	)

	// 3. Envia mensagem real para a fila SQS FIFO wager-transactions.fifo no LocalStack
	msgID := fmt.Sprintf("msg-int-%s", uuid.NewString()[:8])
	idemKey := fmt.Sprintf("prov-int:%s", msgID)
	extTxID := fmt.Sprintf("ext-int-%s", msgID)

	envelope := messaging.SQSRequestedMessageEnvelope{
		MessageID:  msgID,
		Type:       "WagerTransactionRequested",
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
		Data: messaging.SQSRequestedData{
			ProviderID:            "provider-live",
			ExternalTransactionID: extTxID,
			IdempotencyKey:        idemKey,
			PlayerID:              playerID,
			WalletID:              walletID,
			RoundID:               "round-live-1",
			GameID:                "fortune-live",
			Kind:                  "BET",
			Money: messaging.SQSRequestedMoney{
				Amount:   "25.00",
				Currency: "BRL",
			},
		},
	}

	bodyBytes, _ := json.Marshal(envelope)
	sqsPublisher := messaging.NewSQSPublisher(sqsClient)
	err = sqsPublisher.PublishFIFO(ctx, inQueueURL, string(bodyBytes), walletID, msgID)
	if err != nil {
		t.Fatalf("erro ao enviar mensagem para LocalStack: %v", err)
	}

	// 4. Inicia SQSConsumer para consumir da fila real
	consumer := messaging.NewSQSConsumer(
		sqsClient,
		processWagerUseCase,
		messaging.ConsumerConfig{
			QueueURL:          inQueueURL,
			ConsumerName:      "integration-consumer",
			MaxMessages:       1,
			WaitTimeSeconds:   2,
			VisibilityTimeout: 10,
		},
		discardLogger,
	)

	// Recebe a mensagem do SQS diretamente para verificação determinística
	receiveOut, err := sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(inQueueURL),
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     5,
		VisibilityTimeout:   10,
	})
	if err != nil || len(receiveOut.Messages) == 0 {
		t.Fatalf("esperava receber mensagem da fila SQS: %v", err)
	}

	// Processa mensagem
	consumer.ProcessSingleMessage(ctx, receiveOut.Messages[0])

	// 5. Verifica no banco se a carteira foi debitada para 75.00 BRL
	txCheck, _ := pool.Begin(ctx)
	defer txCheck.Rollback(ctx)

	updatedWallet, err := walletRepo.GetByID(ctx, txCheck, walletID, false)
	if err != nil {
		t.Fatalf("erro ao buscar carteira atualizada: %v", err)
	}
	if updatedWallet.Balance().String() != "75.00" {
		t.Errorf("saldo da carteira esperado 75.00, recebeu %s", updatedWallet.Balance().String())
	}

	// 6. Verifica se o evento está na outbox com status PENDING
	pendingEvents, err := outboxRepo.FetchPending(ctx, txCheck, 10)
	if err != nil {
		t.Fatalf("erro ao buscar outbox: %v", err)
	}
	if len(pendingEvents) < 2 {
		t.Fatalf("esperava pelo menos 2 eventos na outbox, recebeu %d", len(pendingEvents))
	}
	// Libera o lock FOR UPDATE para que o worker da outbox possa capturá-los sem ser bloqueado pelo SKIP LOCKED
	_ = txCheck.Rollback(ctx)

	// 7. Roda o OutboxPublisher para publicar no SQS FIFO wager-events.fifo
	outboxWorker := worker.NewOutboxPublisher(
		txMgr,
		outboxRepo,
		sqsPublisher,
		worker.OutboxPublisherConfig{
			QueueURL:  outQueueURL,
			BatchSize: 10,
		},
		discardLogger,
	)

	publishedCount, err := outboxWorker.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("erro ao publicar eventos da outbox: %v", err)
	}
	if publishedCount < 2 {
		t.Errorf("esperava publicar pelo menos 2 eventos, publicou %d", publishedCount)
	}

	// 8. Consome da fila de saída wager-events.fifo no LocalStack e valida o evento
	eventReceiveOut, err := sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(outQueueURL),
		MaxNumberOfMessages: 2,
		WaitTimeSeconds:     5,
		VisibilityTimeout:   10,
	})
	if err != nil {
		t.Fatalf("erro ao ler eventos publicados no SQS: %v", err)
	}
	if len(eventReceiveOut.Messages) == 0 {
		t.Fatalf("nenhum evento encontrado na fila de saida wager-events.fifo")
	}

	// Limpa fila de saída
	for _, m := range eventReceiveOut.Messages {
		_, _ = sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(outQueueURL),
			ReceiptHandle: m.ReceiptHandle,
		})
	}
}
