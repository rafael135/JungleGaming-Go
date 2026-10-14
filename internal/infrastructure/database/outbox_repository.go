package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrOutboxMessageNotFound = errors.New("outbox message not found")

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusFailed    = "FAILED"
)

type OutboxMessage struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	Status        string
	RetryCount    int
	NextAttemptAt time.Time
	CreatedAt     time.Time
	PublishedAt   *time.Time
}

type OutboxRepository struct{}

func NewOutboxRepository() *OutboxRepository {
	return &OutboxRepository{}
}

func (r *OutboxRepository) Create(
	ctx context.Context,
	tx pgx.Tx,
	msg *OutboxMessage,
) error {
	query := `
		INSERT INTO outbox_messages (
			id,
			aggregate_type,
			aggregate_id,
			event_type,
			payload,
			status,
			retry_count,
			next_attempt_at,
			created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`

	status := msg.Status
	if status == "" {
		status = OutboxStatusPending
	}

	createdAt := msg.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	nextAttemptAt := msg.NextAttemptAt
	if nextAttemptAt.IsZero() {
		nextAttemptAt = createdAt
	}

	_, err := tx.Exec(ctx, query,
		msg.ID,
		msg.AggregateType,
		msg.AggregateID,
		msg.EventType,
		msg.Payload,
		status,
		msg.RetryCount,
		nextAttemptAt,
		createdAt,
	)

	if err != nil {
		return fmt.Errorf("erro ao inserir mensagem na outbox: %w", err)
	}

	return nil
}

// FetchPending recupera mensagens pendentes prontas para envio.
// Utiliza SELECT ... FOR UPDATE SKIP LOCKED para permitir que múltiplos
// publishers processem mensagens em paralelo sem disputa ou bloqueio.
func (r *OutboxRepository) FetchPending(
	ctx context.Context,
	tx pgx.Tx,
	limit int,
) ([]*OutboxMessage, error) {
	query := `
		SELECT
			id,
			aggregate_type,
			aggregate_id,
			event_type,
			payload,
			status,
			retry_count,
			next_attempt_at,
			created_at,
			published_at
		FROM outbox_messages
		WHERE status = 'PENDING' AND next_attempt_at <= NOW()
		ORDER BY next_attempt_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := tx.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar mensagens pendentes na outbox: %w", err)
	}
	defer rows.Close()

	messages := make([]*OutboxMessage, 0)
	for rows.Next() {
		var msg OutboxMessage
		err := rows.Scan(
			&msg.ID,
			&msg.AggregateType,
			&msg.AggregateID,
			&msg.EventType,
			&msg.Payload,
			&msg.Status,
			&msg.RetryCount,
			&msg.NextAttemptAt,
			&msg.CreatedAt,
			&msg.PublishedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("erro ao escanear mensagem da outbox: %w", err)
		}

		messages = append(messages, &msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante iteracao das mensagens da outbox: %w", err)
	}

	return messages, nil
}

func (r *OutboxRepository) MarkPublished(
	ctx context.Context,
	tx pgx.Tx,
	id string,
) error {
	query := `
		UPDATE outbox_messages
		SET
			status = 'PUBLISHED',
			published_at = NOW()
		WHERE id = $1
	`

	tag, err := tx.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("erro ao marcar mensagem como publicada na outbox: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrOutboxMessageNotFound
	}

	return nil
}

func (r *OutboxRepository) ScheduleRetry(
	ctx context.Context,
	tx pgx.Tx,
	id string,
	nextAttemptAt time.Time,
) error {
	query := `
		UPDATE outbox_messages
		SET
			retry_count = retry_count + 1,
			next_attempt_at = $2
		WHERE id = $1
	`

	tag, err := tx.Exec(ctx, query, id, nextAttemptAt)
	if err != nil {
		return fmt.Errorf("erro ao agendar retry na outbox: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrOutboxMessageNotFound
	}

	return nil
}

func (r *OutboxRepository) MarkFailed(
	ctx context.Context,
	tx pgx.Tx,
	id string,
) error {
	query := `
		UPDATE outbox_messages
		SET
			status = 'FAILED',
			retry_count = retry_count + 1
		WHERE id = $1
	`

	tag, err := tx.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("erro ao marcar mensagem como falha na outbox: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrOutboxMessageNotFound
	}

	return nil
}

