package database

import (
	"context"
	"errors"
	"fmt"
	"jungle/internal/domain"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrWalletNotFound = errors.New("wallet not found")

//type WalletRepository interface {
//	GetByIDForUpdate(ctx context.Context, tx pgx.Tx, id string) (*domain.Wallet, error)
//	Save(ctx context.Context, tx pgx.Tx, wallet *domain.Wallet) error
//}

type WalletRepository struct{}

func NewWalletRepository() *WalletRepository {
	return &WalletRepository{}
}

func (wr *WalletRepository) GetByID(
	ctx context.Context,
	tx pgx.Tx,
	id string,
	forUpdate bool,
) (*domain.Wallet, error) {

	query := ""

	if forUpdate {
		query = `
			SELECT
				id,
				player_id,
				currency,
				balance_cents,
				version,
				created_at,
				updated_at
			FROM wallets
			WHERE
				id = $1
			FOR UPDATE
		`
	} else {
		query = `
			SELECT
				id,
				player_id,
				currency,
				balance_cents,
				version,
				created_at,
				updated_at
			FROM wallets
			WHERE
				id = $1
		`
	}

	var (
		walletID  string
		playerID  string
		currency  string
		balance   int64
		version   int64
		createdAt time.Time
		updatedAt time.Time
	)

	row := tx.QueryRow(ctx, query, id)

	err := row.Scan(
		&walletID,
		&playerID,
		&currency,
		&balance,
		&version,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWalletNotFound
		}

		return nil, fmt.Errorf("erro ao buscar wallet por id: %w", err)
	}

	walletMoney, err := domain.NewMoney(balance, currency)

	if err != nil {
		return nil, err
	}

	rehydratedWallet, err := domain.RehydrateWallet(walletID, playerID, currency, walletMoney, version, createdAt, updatedAt)

	if err != nil {
		return nil, err
	}

	return rehydratedWallet, nil
}

func (wr *WalletRepository) Create(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error {
	query := `
		INSERT INTO wallets (
			id,
			player_id,
			currency,
			balance_cents,
			version,
			created_at,
			updated_at
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7
		)
	`

	_, err := tx.Exec(ctx, query,
		w.ID(),
		w.PlayerID(),
		w.Currency(),
		w.Balance().Cents(),
		w.Version(),
		w.CreatedAt(),
		w.UpdatedAt(),
	)

	if err != nil {
		return fmt.Errorf("erro ao inserir carteira: %w", err)
	}

	return nil
}

func (wr *WalletRepository) Update(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error {
	query := `
		UPDATE wallets
		SET
			balance_cents = $1,
			version = $2,
			updated_at = $3
		WHERE
			id = $4
	`

	_, err := tx.Exec(ctx, query,
		w.Balance().Cents(),
		w.Version(),
		w.UpdatedAt(),
		w.ID(),
	)

	if err != nil {
		return fmt.Errorf("erro ao atualizar carteira: %w", err)
	}

	return nil
}
