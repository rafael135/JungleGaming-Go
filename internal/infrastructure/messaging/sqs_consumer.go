package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"jungle/internal/application"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type SQSClientAPI interface {
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	ChangeMessageVisibility(ctx context.Context, params *sqs.ChangeMessageVisibilityInput, optFns ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityOutput, error)
}

type ConsumerConfig struct {
	QueueURL          string
	ConsumerName      string
	MaxMessages       int32
	WaitTimeSeconds   int32
	VisibilityTimeout int32
}

func DefaultConsumerConfig(queueURL string) ConsumerConfig {
	return ConsumerConfig{
		QueueURL:          queueURL,
		ConsumerName:      "wager-transactions-consumer",
		MaxMessages:       10,
		WaitTimeSeconds:   20,
		VisibilityTimeout: 30,
	}
}

type SQSConsumer struct {
	client  SQSClientAPI
	useCase *application.ProcessWagerUseCase
	cfg     ConsumerConfig
	logger  *slog.Logger

	wg     sync.WaitGroup
	stopCh chan struct{}
}

func NewSQSConsumer(
	client SQSClientAPI,
	useCase *application.ProcessWagerUseCase,
	cfg ConsumerConfig,
	logger *slog.Logger,
) *SQSConsumer {
	if cfg.ConsumerName == "" {
		cfg.ConsumerName = "wager-transactions-consumer"
	}
	if cfg.MaxMessages <= 0 {
		cfg.MaxMessages = 10
	}
	if cfg.WaitTimeSeconds <= 0 {
		cfg.WaitTimeSeconds = 20
	}
	if cfg.VisibilityTimeout <= 0 {
		cfg.VisibilityTimeout = 30
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &SQSConsumer{
		client:  client,
		useCase: useCase,
		cfg:     cfg,
		logger:  logger,
		stopCh:  make(chan struct{}),
	}
}

func (c *SQSConsumer) Start(ctx context.Context) error {
	c.logger.Info("iniciando consumidor SQS FIFO",
		slog.String("queue_url", c.cfg.QueueURL),
		slog.String("consumer", c.cfg.ConsumerName),
	)

	c.wg.Add(1)
	go c.pollLoop(ctx)

	return nil
}

func (c *SQSConsumer) Stop() {
	close(c.stopCh)
	c.wg.Wait()
	c.logger.Info("consumidor SQS FIFO finalizado com sucesso")
}

func (c *SQSConsumer) pollLoop(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("context cancelado, parando polling do consumidor SQS")
			return
		case <-c.stopCh:
			c.logger.Info("sinal de parada recebido, parando polling do consumidor SQS")
			return
		default:
			output, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
				QueueUrl:            aws.String(c.cfg.QueueURL),
				MaxNumberOfMessages: c.cfg.MaxMessages,
				WaitTimeSeconds:     c.cfg.WaitTimeSeconds,
				VisibilityTimeout:   c.cfg.VisibilityTimeout,
			})

			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				c.logger.Error("erro ao receber mensagens do SQS", slog.Any("error", err))
				time.Sleep(1 * time.Second)
				continue
			}

			if len(output.Messages) == 0 {
				continue
			}

			for _, msg := range output.Messages {
				c.processSingleMessage(ctx, msg)
			}
		}
	}
}

// ProcessSingleMessage processa uma mensagem isolada (exposta para testes e execuções manuais).
func (c *SQSConsumer) ProcessSingleMessage(ctx context.Context, msg types.Message) {
	c.processSingleMessage(ctx, msg)
}

func (c *SQSConsumer) processSingleMessage(ctx context.Context, msg types.Message) {
	if msg.Body == nil {
		c.logger.Warn("mensagem SQS recebida sem corpo")
		return
	}

	var envelope SQSRequestedMessageEnvelope
	if err := json.Unmarshal([]byte(*msg.Body), &envelope); err != nil {
		c.logger.Error("mensagem SQS com payload JSON invalido",
			slog.Any("error", err),
			slog.String("message_id", aws.ToString(msg.MessageId)),
		)
		return
	}

	msgID := envelope.MessageID
	if msgID == "" {
		msgID = aws.ToString(msg.MessageId)
	}

	input := application.ProcessWagerInput{
		IdempotencyKey:                 envelope.Data.IdempotencyKey,
		ProviderID:                     envelope.Data.ProviderID,
		ExternalTransactionID:          envelope.Data.ExternalTransactionID,
		PlayerID:                       envelope.Data.PlayerID,
		WalletID:                       envelope.Data.WalletID,
		GameID:                         envelope.Data.GameID,
		RoundID:                        envelope.Data.RoundID,
		Kind:                           envelope.Data.Kind,
		Amount:                         envelope.Data.Money.Amount,
		Currency:                       envelope.Data.Money.Currency,
		ReferenceExternalTransactionID: envelope.Data.ReferenceExternalTransactionID,
		ConsumerName:                   c.cfg.ConsumerName,
		MessageID:                      msgID,
	}

	output, err := c.useCase.Execute(ctx, input)
	if err != nil {
		c.logger.Error("falha ao processar mensagem de aposta do SQS",
			slog.Any("error", err),
			slog.String("message_id", msgID),
			slog.String("idempotency_key", input.IdempotencyKey),
		)
		return
	}

	c.logger.Info("mensagem SQS processada com sucesso",
		slog.String("message_id", msgID),
		slog.String("status", output.Status),
		slog.Bool("idempotent_replay", output.IdempotentReplay),
	)

	_, delErr := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.cfg.QueueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if delErr != nil {
		c.logger.Error("erro ao deletar mensagem da fila SQS apos commit",
			slog.Any("error", delErr),
			slog.String("message_id", msgID),
		)
	}
}

