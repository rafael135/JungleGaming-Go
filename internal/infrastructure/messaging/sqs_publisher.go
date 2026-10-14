package messaging

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type SQSPublisher struct {
	client SQSClientAPI
}

func NewSQSPublisher(client SQSClientAPI) *SQSPublisher {
	return &SQSPublisher{client: client}
}

func (p *SQSPublisher) PublishFIFO(
	ctx context.Context,
	queueURL string,
	body string,
	messageGroupID string,
	messageDeduplicationID string,
) error {
	if queueURL == "" {
		return fmt.Errorf("queueURL obrigatoria")
	}
	if messageGroupID == "" {
		return fmt.Errorf("messageGroupID obrigatorio para filas FIFO")
	}

	input := &sqs.SendMessageInput{
		QueueUrl:               aws.String(queueURL),
		MessageBody:            aws.String(body),
		MessageGroupId:         aws.String(messageGroupID),
		MessageDeduplicationId: aws.String(messageDeduplicationID),
	}

	_, err := p.client.SendMessage(ctx, input)
	if err != nil {
		return fmt.Errorf("erro ao enviar mensagem para SQS FIFO: %w", err)
	}

	return nil
}
