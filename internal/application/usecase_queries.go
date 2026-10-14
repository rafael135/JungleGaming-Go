package application

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"jungle/internal/domain"

	"github.com/jackc/pgx/v5"
)

func EncodeLedgerCursor(createdAt time.Time, id string) string {
	raw := fmt.Sprintf("%s|%s", createdAt.UTC().Format(time.RFC3339Nano), id)
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func DecodeLedgerCursor(cursor string) (*time.Time, string, error) {
	if cursor == "" {
		return nil, "", nil
	}

	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, "", ErrInvalidCursor
	}

	parts := strings.Split(string(data), "|")
	if len(parts) != 2 {
		return nil, "", ErrInvalidCursor
	}

	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, "", ErrInvalidCursor
	}

	id := parts[1]
	if id == "" {
		return nil, "", ErrInvalidCursor
	}

	return &t, id, nil
}

// --- GET WALLET USE CASE ---

type GetWalletInput struct {
	WalletID string
}

type GetWalletOutput struct {
	ID       string        `json:"id"`
	PlayerID string        `json:"playerId"`
	Balance  WalletBalance `json:"balance"`
	Version  int64         `json:"version"`
}

type GetWalletUseCase struct {
	txManager  TransactionManager
	walletRepo WalletRepository
}

func NewGetWalletUseCase(txManager TransactionManager, walletRepo WalletRepository) *GetWalletUseCase {
	return &GetWalletUseCase{
		txManager:  txManager,
		walletRepo: walletRepo,
	}
}

func (uc *GetWalletUseCase) Execute(ctx context.Context, input GetWalletInput) (*GetWalletOutput, error) {
	if input.WalletID == "" {
		return nil, domain.ErrInvalidWalletId
	}

	var output *GetWalletOutput
	err := uc.txManager.Execute(ctx, func(tx pgx.Tx) error {
		wallet, err := uc.walletRepo.GetByID(ctx, tx, input.WalletID, false)
		if err != nil {
			return ErrWalletNotFound
		}

		output = &GetWalletOutput{
			ID:       wallet.ID(),
			PlayerID: wallet.PlayerID(),
			Balance: WalletBalance{
				Amount:   wallet.Balance().String(),
				Currency: wallet.Currency(),
			},
			Version: wallet.Version(),
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return output, nil
}

// --- GET WALLET LEDGER USE CASE ---

type GetWalletLedgerInput struct {
	WalletID string
	Limit    int
	Cursor   string
}

type WalletLedgerItem struct {
	ID            string        `json:"id"`
	WalletID      string        `json:"walletId"`
	TransactionID string        `json:"transactionId"`
	Direction     string        `json:"direction"`
	Amount        WalletBalance `json:"amount"`
	BalanceBefore WalletBalance `json:"balanceBefore"`
	BalanceAfter  WalletBalance `json:"balanceAfter"`
	CreatedAt     time.Time     `json:"createdAt"`
}

type GetWalletLedgerOutput struct {
	Items      []WalletLedgerItem `json:"items"`
	NextCursor string             `json:"nextCursor,omitempty"`
}

type GetWalletLedgerUseCase struct {
	txManager  TransactionManager
	walletRepo WalletRepository
	ledgerRepo LedgerRepository
}

func NewGetWalletLedgerUseCase(
	txManager TransactionManager,
	walletRepo WalletRepository,
	ledgerRepo LedgerRepository,
) *GetWalletLedgerUseCase {
	return &GetWalletLedgerUseCase{
		txManager:  txManager,
		walletRepo: walletRepo,
		ledgerRepo: ledgerRepo,
	}
}

func (uc *GetWalletLedgerUseCase) Execute(ctx context.Context, input GetWalletLedgerInput) (*GetWalletLedgerOutput, error) {
	if input.WalletID == "" {
		return nil, domain.ErrInvalidWalletId
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	cursorTime, cursorID, err := DecodeLedgerCursor(input.Cursor)
	if err != nil {
		return nil, err
	}

	var output *GetWalletLedgerOutput
	err = uc.txManager.Execute(ctx, func(tx pgx.Tx) error {
		// Valida que a carteira existe
		_, err := uc.walletRepo.GetByID(ctx, tx, input.WalletID, false)
		if err != nil {
			return ErrWalletNotFound
		}

		// Busca limit + 1 para determinar se existe próxima página sem query de count
		entries, err := uc.ledgerRepo.ListPaginated(ctx, tx, input.WalletID, limit+1, cursorTime, cursorID)
		if err != nil {
			return err
		}

		var nextCursor string
		if len(entries) > limit {
			lastItem := entries[limit-1]
			nextCursor = EncodeLedgerCursor(lastItem.CreatedAt(), lastItem.ID())
			entries = entries[:limit]
		}

		items := make([]WalletLedgerItem, len(entries))
		for i, entry := range entries {
			items[i] = WalletLedgerItem{
				ID:            entry.ID(),
				WalletID:      entry.WalletID(),
				TransactionID: entry.TransactionID(),
				Direction:     string(entry.Direction()),
				Amount: WalletBalance{
					Amount:   entry.Amount().String(),
					Currency: entry.Amount().Currency(),
				},
				BalanceBefore: WalletBalance{
					Amount:   entry.BalanceBefore().String(),
					Currency: entry.BalanceBefore().Currency(),
				},
				BalanceAfter: WalletBalance{
					Amount:   entry.BalanceAfter().String(),
					Currency: entry.BalanceAfter().Currency(),
				},
				CreatedAt: entry.CreatedAt(),
			}
		}

		output = &GetWalletLedgerOutput{
			Items:      items,
			NextCursor: nextCursor,
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return output, nil
}

// --- GET WAGER TRANSACTION USE CASE ---

type GetWagerTransactionInput struct {
	TransactionID         string
	ProviderID            string
	ExternalTransactionID string
}

type GetWagerTransactionOutput struct {
	ID                             string        `json:"id"`
	ExternalTransactionID          string        `json:"externalTransactionId,omitempty"`
	ProviderID                     string        `json:"providerId,omitempty"`
	IdempotencyKey                 string        `json:"idempotencyKey,omitempty"`
	WalletID                       string        `json:"walletId"`
	PlayerID                       string        `json:"playerId"`
	GameID                         string        `json:"gameId,omitempty"`
	RoundID                        string        `json:"roundId,omitempty"`
	TransactionType                string        `json:"transactionType"`
	Amount                         WalletBalance `json:"amount"`
	ReferenceExternalTransactionID string        `json:"referenceExternalTransactionId,omitempty"`
	Status                         string        `json:"status"`
	FailureCode                    string        `json:"failureCode,omitempty"`
	CreatedAt                      time.Time     `json:"createdAt"`
	UpdatedAt                      time.Time     `json:"updatedAt"`
}

type GetWagerTransactionUseCase struct {
	txManager TransactionManager
	wagerRepo WagerTransactionRepository
}

func NewGetWagerTransactionUseCase(
	txManager TransactionManager,
	wagerRepo WagerTransactionRepository,
) *GetWagerTransactionUseCase {
	return &GetWagerTransactionUseCase{
		txManager: txManager,
		wagerRepo: wagerRepo,
	}
}

func (uc *GetWagerTransactionUseCase) Execute(ctx context.Context, input GetWagerTransactionInput) (*GetWagerTransactionOutput, error) {
	var (
		txObj  *domain.WagerTransaction
		output *GetWagerTransactionOutput
	)

	err := uc.txManager.Execute(ctx, func(tx pgx.Tx) error {
		var err error
		if input.TransactionID != "" {
			txObj, err = uc.wagerRepo.GetByID(ctx, tx, input.TransactionID)
		} else if input.ProviderID != "" && input.ExternalTransactionID != "" {
			txObj, err = uc.wagerRepo.GetByExternalID(ctx, tx, input.ProviderID, input.ExternalTransactionID)
		} else {
			return ErrInvalidTransactionQuery
		}

		if err != nil || txObj == nil {
			return ErrTransactionNotFound
		}

		output = &GetWagerTransactionOutput{
			ID:                    txObj.ID(),
			ExternalTransactionID: txObj.ExternalTransactionID(),
			ProviderID:            txObj.ProviderID(),
			IdempotencyKey:        txObj.IdempotencyKey(),
			WalletID:              txObj.WalletID(),
			PlayerID:              txObj.PlayerID(),
			GameID:                txObj.GameID(),
			RoundID:               txObj.RoundID(),
			TransactionType:       string(txObj.TransactionType()),
			Amount: WalletBalance{
				Amount:   txObj.Amount().String(),
				Currency: txObj.Amount().Currency(),
			},
			ReferenceExternalTransactionID: txObj.ReferenceExternalTransactionID(),
			Status:                         string(txObj.Status()),
			FailureCode:                    txObj.FailureCode(),
			CreatedAt:                      txObj.CreatedAt(),
			UpdatedAt:                      txObj.UpdatedAt(),
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return output, nil
}
