package messaging_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"jungle/internal/application"
	"jungle/internal/domain"
	"jungle/internal/infrastructure/database"
	"jungle/internal/infrastructure/messaging"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/jackc/pgx/v5"
)

// --- FAKE SQS CLIENT ---

type fakeSQSClient struct {
	receiveOutput   *sqs.ReceiveMessageOutput
	receiveErr      error
	deletedReceipts []string
	sentMessages    []*sqs.SendMessageInput
}

func (f *fakeSQSClient) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	if f.receiveErr != nil {
		return nil, f.receiveErr
	}
	if f.receiveOutput != nil {
		out := f.receiveOutput
		f.receiveOutput = &sqs.ReceiveMessageOutput{} // Retorna uma vez para não entrar em loop infinito no teste
		return out, nil
	}
	return &sqs.ReceiveMessageOutput{}, nil
}

func (f *fakeSQSClient) DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	f.deletedReceipts = append(f.deletedReceipts, aws.ToString(params.ReceiptHandle))
	return &sqs.DeleteMessageOutput{}, nil
}

func (f *fakeSQSClient) SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	f.sentMessages = append(f.sentMessages, params)
	return &sqs.SendMessageOutput{}, nil
}

func (f *fakeSQSClient) ChangeMessageVisibility(ctx context.Context, params *sqs.ChangeMessageVisibilityInput, optFns ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityOutput, error) {
	return &sqs.ChangeMessageVisibilityOutput{}, nil
}

// --- FAKE TX MANAGER & REPOSITORIES PARA O CONSUMIDOR ---

type testTxMgr struct{}

func (t *testTxMgr) Execute(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return fn(nil)
}

type testWalletRepo struct {
	wallet *domain.Wallet
}

func (r *testWalletRepo) GetByID(ctx context.Context, tx pgx.Tx, id string, forUpdate bool) (*domain.Wallet, error) {
	if r.wallet != nil && r.wallet.ID() == id {
		return r.wallet, nil
	}
	return nil, errors.New("wallet not found")
}

func (r *testWalletRepo) Create(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error {
	r.wallet = w
	return nil
}

func (r *testWalletRepo) Update(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error {
	r.wallet = w
	return nil
}

type testLedgerRepo struct{}

func (l *testLedgerRepo) Create(ctx context.Context, tx pgx.Tx, entry *domain.WalletLedgerEntry) error {
	return nil
}

func (l *testLedgerRepo) GetByTransactionID(ctx context.Context, tx pgx.Tx, id string) (*domain.WalletLedgerEntry, error) {
	return nil, nil
}

func (l *testLedgerRepo) ListByWalletID(ctx context.Context, tx pgx.Tx, walletID string) ([]*domain.WalletLedgerEntry, error) {
	return nil, nil
}

func (l *testLedgerRepo) ListPaginated(ctx context.Context, tx pgx.Tx, walletID string, limit int, cursorCreatedAt *time.Time, cursorID string) ([]*domain.WalletLedgerEntry, error) {
	return nil, nil
}

type testWagerRepo struct {
	byKey map[string]*domain.WagerTransaction
}

func (w *testWagerRepo) Create(ctx context.Context, tx pgx.Tx, t *domain.WagerTransaction) error {
	if w.byKey == nil {
		w.byKey = make(map[string]*domain.WagerTransaction)
	}
	w.byKey[t.IdempotencyKey()] = t
	return nil
}

func (w *testWagerRepo) Update(ctx context.Context, tx pgx.Tx, t *domain.WagerTransaction) error {
	return nil
}

func (w *testWagerRepo) GetByID(ctx context.Context, tx pgx.Tx, id string) (*domain.WagerTransaction, error) {
	return nil, nil
}

func (w *testWagerRepo) GetByIdempotencyKey(ctx context.Context, tx pgx.Tx, providerID string, idempotencyKey string) (*domain.WagerTransaction, error) {
	if w.byKey == nil {
		return nil, errors.New("not found")
	}
	t, exists := w.byKey[idempotencyKey]
	if !exists {
		return nil, errors.New("not found")
	}
	return t, nil
}

func (w *testWagerRepo) GetByExternalID(ctx context.Context, tx pgx.Tx, providerID string, externalID string) (*domain.WagerTransaction, error) {
	return nil, nil
}

func (w *testWagerRepo) FetchPendingReferences(ctx context.Context, tx pgx.Tx, limit int) ([]*domain.WagerTransaction, error) {
	return nil, nil
}

type testOutboxRepo struct{}

func (o *testOutboxRepo) Create(ctx context.Context, tx pgx.Tx, msg *database.OutboxMessage) error {
	return nil
}
func (o *testOutboxRepo) FetchPending(ctx context.Context, tx pgx.Tx, limit int) ([]*database.OutboxMessage, error) {
	return nil, nil
}
func (o *testOutboxRepo) MarkPublished(ctx context.Context, tx pgx.Tx, id string) error { return nil }
func (o *testOutboxRepo) ScheduleRetry(ctx context.Context, tx pgx.Tx, id string, at time.Time) error {
	return nil
}
func (o *testOutboxRepo) MarkFailed(ctx context.Context, tx pgx.Tx, id string) error { return nil }

type testInboxRepo struct {
	recorded map[string]string
}

func (i *testInboxRepo) TryRecord(ctx context.Context, tx pgx.Tx, consumerName string, messageID string, payloadHash string) (bool, error) {
	if i.recorded == nil {
		i.recorded = make(map[string]string)
	}
	if _, exists := i.recorded[messageID]; exists {
		return false, nil
	}
	i.recorded[messageID] = payloadHash
	return true, nil
}
func (i *testInboxRepo) Get(ctx context.Context, tx pgx.Tx, consumerName string, messageID string) (*database.InboxMessage, error) {
	return nil, nil
}
func (i *testInboxRepo) MarkProcessed(ctx context.Context, tx pgx.Tx, consumerName string, messageID string) error {
	return nil
}

func TestSQSConsumer(t *testing.T) {
	ctx := context.Background()
	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Prepara use case com carteira de 100.00 BRL
	initialMoney, _ := domain.NewMoney(10000, "BRL")
	wallet, _ := domain.NewWallet("wallet-sqs-test", "player-1", initialMoney)
	walletRepo := &testWalletRepo{wallet: wallet}

	useCase := application.NewProcessWagerUseCase(
		&testTxMgr{},
		walletRepo,
		&testLedgerRepo{},
		&testWagerRepo{},
		&testOutboxRepo{},
		&testInboxRepo{},
	)

	t.Run("sucesso: consome mensagem valida, executa caso de uso e remove mensagem da fila", func(t *testing.T) {
		fakeSQS := &fakeSQSClient{}

		envelope := messaging.SQSRequestedMessageEnvelope{
			MessageID:  "msg-sqs-100",
			Type:       "WagerTransactionRequested",
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Data: messaging.SQSRequestedData{
				ProviderID:            "provider-a",
				ExternalTransactionID: "tx-sqs-100",
				IdempotencyKey:        "prov-a:tx-sqs-100",
				PlayerID:              "player-1",
				WalletID:              "wallet-sqs-test",
				RoundID:               "round-1",
				GameID:                "fortune-tiger",
				Kind:                  "BET",
				Money: messaging.SQSRequestedMoney{
					Amount:   "25.00",
					Currency: "BRL",
				},
			},
		}

		bodyBytes, _ := json.Marshal(envelope)
		bodyStr := string(bodyBytes)

		msg := types.Message{
			MessageId:     aws.String("sqs-msg-id-1"),
			ReceiptHandle: aws.String("receipt-handle-123"),
			Body:          aws.String(bodyStr),
		}

		consumer := messaging.NewSQSConsumer(
			fakeSQS,
			useCase,
			messaging.ConsumerConfig{
				QueueURL:     "https://sqs.us-east-1.amazonaws.com/123/wager-transactions.fifo",
				ConsumerName: "test-consumer",
			},
			discardLogger,
		)

		// Processa mensagem diretamente
		consumer.ProcessSingleMessage(ctx, msg)

		// Deve ter chamado DeleteMessage com o receipt handle correspondente
		if len(fakeSQS.deletedReceipts) != 1 {
			t.Fatalf("esperava 1 mensagem deletada, recebeu %d", len(fakeSQS.deletedReceipts))
		}
		if fakeSQS.deletedReceipts[0] != "receipt-handle-123" {
			t.Errorf("receipt handle esperado receipt-handle-123, recebeu %s", fakeSQS.deletedReceipts[0])
		}

		// Valida que o saldo foi debitado para 75.00
		if wallet.Balance().String() != "75.00" {
			t.Errorf("saldo da carteira esperado 75.00, recebeu %s", wallet.Balance().String())
		}
	})

	t.Run("caso de borda: mensagem com JSON invalido nao eh deletada para redrive na DLQ", func(t *testing.T) {
		fakeSQS := &fakeSQSClient{}

		msg := types.Message{
			MessageId:     aws.String("sqs-msg-id-bad"),
			ReceiptHandle: aws.String("receipt-bad"),
			Body:          aws.String("payload-totalmente-quebrado-e-invalido"),
		}

		consumer := messaging.NewSQSConsumer(
			fakeSQS,
			useCase,
			messaging.ConsumerConfig{
				QueueURL: "https://sqs.us-east-1.amazonaws.com/123/wager-transactions.fifo",
			},
			discardLogger,
		)

		consumer.ProcessSingleMessage(ctx, msg)

		// Mensagem com payload inválido NÃO pode ser deletada silenciosamente
		if len(fakeSQS.deletedReceipts) != 0 {
			t.Errorf("mensagem com erro nao deve ser deletada, para que o broker aplique redrive para a DLQ")
		}
	})

	t.Run("sucesso: inicio e parada graciosa do consumidor", func(t *testing.T) {
		fakeSQS := &fakeSQSClient{}

		consumer := messaging.NewSQSConsumer(
			fakeSQS,
			useCase,
			messaging.ConsumerConfig{
				QueueURL:        "https://sqs.us-east-1.amazonaws.com/123/wager-transactions.fifo",
				WaitTimeSeconds: 1,
			},
			discardLogger,
		)

		runCtx, cancel := context.WithCancel(ctx)
		if err := consumer.Start(runCtx); err != nil {
			t.Fatalf("erro ao iniciar consumer: %v", err)
		}

		time.Sleep(50 * time.Millisecond)
		cancel()
		consumer.Stop()
	})
}
