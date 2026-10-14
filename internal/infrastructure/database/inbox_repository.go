package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrInboxMessageNotFound = errors.New("inbox message not found")

type InboxMessage struct {
	ConsumerName string
	MessageID    string
	PayloadHash  string
	ReceivedAt   time.Time
	ProcessedAt  *time.Time
}

type InboxRepository struct{}

func NewInboxRepository() *InboxRepository {
	return &InboxRepository{}
}

// TryRecord tenta registrar uma mensagem na inbox.
// Retorna true se a mensagem é inédita e foi inserida com sucesso.
// Retorna false se a mensagem já existia (chave duplicada no par consumer_name + message_id).
func (r *InboxRepository) TryRecord(
	ctx context.Context,
	tx pgx.Tx,
	consumerName string,
	messageID string,
	payloadHash string,
) (bool, error) {
	query := `
		INSERT INTO inbox_messages (
			consumer_name,
			message_id,
			payload_hash,
			received_at
		) VALUES (
			$1, $2, $3, NOW()
		)
		ON CONFLICT (consumer_name, message_id) DO NOTHING
	`

	tag, err := tx.Exec(ctx, query, consumerName, messageID, payloadHash)
	if err != nil {
		return false, fmt.Errorf("erro ao registrar mensagem na inbox: %w", err)
	}

	return tag.RowsAffected() == 1, nil
}

func (r *InboxRepository) Get(
	ctx context.Context,
	tx pgx.Tx,
	consumerName string,
	messageID string,
) (*InboxMessage, error) {
	query := `
		SELECT
			consumer_name,
			message_id,
			payload_hash,
			received_at,
			processed_at
		FROM inbox_messages
		WHERE consumer_name = $1 AND message_id = $2
	`

	var msg InboxMessage
	err := tx.QueryRow(ctx, query, consumerName, messageID).Scan(
		&msg.ConsumerName,
		&msg.MessageID,
		&msg.PayloadHash,
		&msg.ReceivedAt,
		&msg.ProcessedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInboxMessageNotFound
		}

		return nil, fmt.Errorf("erro ao buscar mensagem na inbox: %w", err)
	}

	return &msg, nil
}

func (r *InboxRepository) MarkProcessed(
	ctx context.Context,
	tx pgx.Tx,
	consumerName string,
	messageID string,
) error {
	query := `
		UPDATE inbox_messages
		SET processed_at = NOW()
		WHERE consumer_name = $1 AND message_id = $2
	`

	tag, err := tx.Exec(ctx, query, consumerName, messageID)
	if err != nil {
		return fmt.Errorf("erro ao marcar mensagem como processada na inbox: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrInboxMessageNotFound
	}

	return nil
}
