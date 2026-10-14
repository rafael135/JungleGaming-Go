package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresTransactionManager struct {
	pool *pgxpool.Pool
}

func NewPostgresTransactionManager(pool *pgxpool.Pool) *PostgresTransactionManager {
	return &PostgresTransactionManager{pool: pool}
}

func (tm *PostgresTransactionManager) Execute(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := tm.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("erro ao iniciar transacao no postgres: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("erro ao comitar transacao no postgres: %w", err)
	}

	return nil
}

