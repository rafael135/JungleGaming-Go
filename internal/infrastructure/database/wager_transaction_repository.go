package database

import (
	"context"
	"errors"
	"fmt"
	"jungle/internal/domain"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrTransactionNotFound = errors.New("transaction not found")

type WagerTransactionRepository struct{}

func NewWagerTransactionRepository() *WagerTransactionRepository {
	return &WagerTransactionRepository{}
}

const selectWagerTransactionSQL = `
	SELECT
		id,
		external_transaction_id,
		provider_id,
		idempotency_key,
		payload_hash,
		wallet_id,
		player_id,
		game_id,
		round_id,
		transaction_type,
		amount_cents,
		currency,
		reference_external_transaction_id,
		status,
		failure_code,
		created_at,
		updated_at
	FROM wager_transactions
`

func (r *WagerTransactionRepository) Create(
	ctx context.Context,
	tx pgx.Tx,
	t *domain.WagerTransaction,
) error {
	query := `
		INSERT INTO wager_transactions (
			id,
			external_transaction_id,
			provider_id,
			idempotency_key,
			payload_hash,
			wallet_id,
			player_id,
			game_id,
			round_id,
			transaction_type,
			amount_cents,
			currency,
			reference_external_transaction_id,
			status,
			failure_code,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
		)
	`

	_, err := tx.Exec(ctx, query,
		t.ID(),
		toNullString(t.ExternalTransactionID()),
		toNullString(t.ProviderID()),
		toNullString(t.IdempotencyKey()),
		toNullString(t.PayloadHash()),
		t.WalletID(),
		t.PlayerID(),
		toNullString(t.GameID()),
		toNullString(t.RoundID()),
		string(t.TransactionType()),
		t.Amount().Cents(),
		t.Amount().Currency(),
		toNullString(t.ReferenceExternalTransactionID()),
		string(t.Status()),
		toNullString(t.FailureCode()),
		t.CreatedAt(),
		t.UpdatedAt(),
	)

	if err != nil {
		return fmt.Errorf("erro ao criar transacao: %w", err)
	}

	return nil
}

func (r *WagerTransactionRepository) Update(
	ctx context.Context,
	tx pgx.Tx,
	t *domain.WagerTransaction,
) error {
	query := `
		UPDATE wager_transactions
		SET
			status = $1,
			failure_code = $2,
			updated_at = $3
		WHERE
			id = $4
	`

	tag, err := tx.Exec(ctx, query,
		string(t.Status()),
		toNullString(t.FailureCode()),
		t.UpdatedAt(),
		t.ID(),
	)

	if err != nil {
		return fmt.Errorf("erro ao atualizar transacao: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrTransactionNotFound
	}

	return nil
}

func (r *WagerTransactionRepository) GetByID(
	ctx context.Context,
	tx pgx.Tx,
	id string,
) (*domain.WagerTransaction, error) {
	query := selectWagerTransactionSQL + " WHERE id = $1"
	row := tx.QueryRow(ctx, query, id)
	return r.scanRow(row)
}

func (r *WagerTransactionRepository) GetByIdempotencyKey(
	ctx context.Context,
	tx pgx.Tx,
	providerID string,
	idempotencyKey string,
) (*domain.WagerTransaction, error) {
	query := selectWagerTransactionSQL + " WHERE provider_id = $1 AND idempotency_key = $2"
	row := tx.QueryRow(ctx, query, providerID, idempotencyKey)
	return r.scanRow(row)
}

func (r *WagerTransactionRepository) GetByExternalID(
	ctx context.Context,
	tx pgx.Tx,
	providerID string,
	externalID string,
) (*domain.WagerTransaction, error) {
	query := selectWagerTransactionSQL + " WHERE provider_id = $1 AND external_transaction_id = $2"
	row := tx.QueryRow(ctx, query, providerID, externalID)
	return r.scanRow(row)
}

func (r *WagerTransactionRepository) scanRow(row pgx.Row) (*domain.WagerTransaction, error) {
	var (
		id                             string
		externalTransactionID          *string
		providerID                     *string
		idempotencyKey                 *string
		payloadHash                    *string
		walletID                       string
		playerID                       string
		gameID                         *string
		roundID                        *string
		transactionType                string
		amountCents                    int64
		currency                       string
		referenceExternalTransactionID *string
		status                         string
		failureCode                    *string
		createdAt                      time.Time
		updatedAt                      time.Time
	)

	err := row.Scan(
		&id,
		&externalTransactionID,
		&providerID,
		&idempotencyKey,
		&payloadHash,
		&walletID,
		&playerID,
		&gameID,
		&roundID,
		&transactionType,
		&amountCents,
		&currency,
		&referenceExternalTransactionID,
		&status,
		&failureCode,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTransactionNotFound
		}

		return nil, fmt.Errorf("erro ao escanear transacao: %w", err)
	}

	money, err := domain.NewMoney(amountCents, currency)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar money da transacao: %w", err)
	}

	tx, err := domain.RehydrateWagerTransaction(
		id,
		fromNullString(externalTransactionID),
		fromNullString(providerID),
		fromNullString(idempotencyKey),
		fromNullString(payloadHash),
		walletID,
		playerID,
		fromNullString(gameID),
		fromNullString(roundID),
		domain.TransactionType(transactionType),
		money,
		fromNullString(referenceExternalTransactionID),
		domain.TransactionStatus(status),
		fromNullString(failureCode),
		createdAt,
		updatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("erro ao reidratar transacao: %w", err)
	}

	return tx, nil
}

func toNullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func fromNullString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (r *WagerTransactionRepository) FetchPendingReferences(
	ctx context.Context,
	tx pgx.Tx,
	limit int,
) ([]*domain.WagerTransaction, error) {
	if limit <= 0 {
		limit = 50
	}

	query := selectWagerTransactionSQL + `
		WHERE status = 'PENDING_REFERENCE'
		ORDER BY created_at ASC
		LIMIT $1
	`
	rows, err := tx.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar transacoes com referencia pendente: %w", err)
	}
	defer rows.Close()

	var transactions []*domain.WagerTransaction
	for rows.Next() {
		t, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, t)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("erro na iteracao de transacoes pendentes: %w", err)
	}

	return transactions, nil
}
