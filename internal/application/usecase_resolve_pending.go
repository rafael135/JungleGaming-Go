package application

import (
	"context"
	"fmt"
	"time"

	"jungle/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	DefaultPendingReferenceTTL = 24 * time.Hour
)

type ResolvePendingReferencesInput struct {
	Limit int
	TTL   time.Duration
}

type ResolvePendingReferencesOutput struct {
	CheckedCount   int `json:"checkedCount"`
	ResolvedCount  int `json:"resolvedCount"`
	RejectedCount  int `json:"rejectedCount"`
	RemainingCount int `json:"remainingCount"`
}

type ResolvePendingReferencesUseCase struct {
	txManager  TransactionManager
	walletRepo WalletRepository
	ledgerRepo LedgerRepository
	wagerRepo  WagerTransactionRepository
	outboxRepo OutboxRepository
}

func NewResolvePendingReferencesUseCase(
	txManager TransactionManager,
	walletRepo WalletRepository,
	ledgerRepo LedgerRepository,
	wagerRepo WagerTransactionRepository,
	outboxRepo OutboxRepository,
) *ResolvePendingReferencesUseCase {
	return &ResolvePendingReferencesUseCase{
		txManager:  txManager,
		walletRepo: walletRepo,
		ledgerRepo: ledgerRepo,
		wagerRepo:  wagerRepo,
		outboxRepo: outboxRepo,
	}
}

func (uc *ResolvePendingReferencesUseCase) Execute(ctx context.Context, input ResolvePendingReferencesInput) (*ResolvePendingReferencesOutput, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	ttl := input.TTL
	if ttl <= 0 {
		ttl = DefaultPendingReferenceTTL
	}

	var pendingTxs []*domain.WagerTransaction

	// 1. Busca lote de transações PENDING_REFERENCE
	err := uc.txManager.Execute(ctx, func(tx pgx.Tx) error {
		var err error
		pendingTxs, err = uc.wagerRepo.FetchPendingReferences(ctx, tx, limit)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar transacoes com referencia pendente: %w", err)
	}

	output := &ResolvePendingReferencesOutput{
		CheckedCount: len(pendingTxs),
	}

	now := time.Now().UTC()

	// 2. Processa cada transação isoladamente
	for _, pendingTx := range pendingTxs {
		err := uc.txManager.Execute(ctx, func(tx pgx.Tx) error {
			// Verifica expiração de TTL
			if now.Sub(pendingTx.CreatedAt()) > ttl {
				return uc.rejectPending(ctx, tx, pendingTx, "REFERENCE_NOT_FOUND_EXPIRED", output)
			}

			// Busca transação referenciada
			refTx, err := uc.wagerRepo.GetByExternalID(ctx, tx, pendingTx.ProviderID(), pendingTx.ReferenceExternalTransactionID())
			if err != nil || refTx == nil {
				// Referência ainda não chegou; permanece em PENDING_REFERENCE
				output.RemainingCount++
				return nil
			}

			// Referência encontrada mas ainda em processamento intermediário
			if refTx.Status() == domain.TransactionStatusPending || refTx.Status() == domain.TransactionStatusPendingReference {
				output.RemainingCount++
				return nil
			}

			// Referência terminou sem sucesso (REJECTED ou FAILED)
			if refTx.Status() == domain.TransactionStatusRejected || refTx.Status() == domain.TransactionStatusFailed {
				return uc.rejectPending(ctx, tx, pendingTx, "REFERENCE_TERMINATED_UNSUCCESSFULLY", output)
			}

			// Referência está PROCESSED: valida compatibilidade
			if refTx.PlayerID() != pendingTx.PlayerID() ||
				refTx.WalletID() != pendingTx.WalletID() ||
				refTx.RoundID() != pendingTx.RoundID() ||
				refTx.Amount().Cents() != pendingTx.Amount().Cents() {
				return uc.rejectPending(ctx, tx, pendingTx, "REFERENCE_MISMATCH", output)
			}

			// Lock pessimista na carteira
			wallet, err := uc.walletRepo.GetByID(ctx, tx, pendingTx.WalletID(), true)
			if err != nil {
				return err
			}

			balanceBefore := wallet.Balance()

			// Executa a reversão conforme o tipo
			switch pendingTx.TransactionType() {
			case domain.TransactionTypeRefund:
				if refTx.TransactionType() != domain.TransactionTypeBet {
					return uc.rejectPending(ctx, tx, pendingTx, "REFERENCE_MISMATCH", output)
				}
				if err := wallet.Credit(pendingTx.Amount()); err != nil {
					return err
				}
				if err := uc.applyResolution(ctx, tx, wallet, pendingTx, refTx, domain.LedgerDirectionCredit, balanceBefore, output); err != nil {
					return err
				}

			case domain.TransactionTypeRollback:
				if refTx.TransactionType() == domain.TransactionTypeBet {
					if err := wallet.Credit(pendingTx.Amount()); err != nil {
						return err
					}
					if err := uc.applyResolution(ctx, tx, wallet, pendingTx, refTx, domain.LedgerDirectionCredit, balanceBefore, output); err != nil {
						return err
					}
				} else if refTx.TransactionType() == domain.TransactionTypeWin || refTx.TransactionType() == domain.TransactionTypeRefund {
					if wallet.Balance().Cents() < pendingTx.Amount().Cents() {
						return uc.rejectPending(ctx, tx, pendingTx, "INSUFFICIENT_FUNDS_FOR_ROLLBACK", output)
					}
					if err := wallet.Debit(pendingTx.Amount()); err != nil {
						return err
					}
					if err := uc.applyResolution(ctx, tx, wallet, pendingTx, refTx, domain.LedgerDirectionDebit, balanceBefore, output); err != nil {
						return err
					}
				} else {
					return uc.rejectPending(ctx, tx, pendingTx, "REFERENCE_MISMATCH", output)
				}
			}

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return output, nil
}

func (uc *ResolvePendingReferencesUseCase) rejectPending(
	ctx context.Context,
	tx pgx.Tx,
	pendingTx *domain.WagerTransaction,
	failureCode string,
	output *ResolvePendingReferencesOutput,
) error {
	if err := pendingTx.MarkRejected(failureCode); err != nil {
		return err
	}
	if err := uc.wagerRepo.Update(ctx, tx, pendingTx); err != nil {
		return err
	}

	event, err := NewWagerTransactionRejectedOutboxMessage(pendingTx.WalletID(), "", WagerTransactionRejectedData{
		TransactionID:         pendingTx.ID(),
		WalletID:              pendingTx.WalletID(),
		PlayerID:              pendingTx.PlayerID(),
		TransactionType:       string(pendingTx.TransactionType()),
		Amount:                EventMoney{Amount: pendingTx.Amount().String(), Currency: pendingTx.Amount().Currency()},
		Status:                domain.TransactionStatusRejected,
		FailureCode:           failureCode,
		ProviderID:            pendingTx.ProviderID(),
		ExternalTransactionID: pendingTx.ExternalTransactionID(),
	})
	if err != nil {
		return err
	}
	if err := uc.outboxRepo.Create(ctx, tx, event); err != nil {
		return err
	}

	output.RejectedCount++
	return nil
}

func (uc *ResolvePendingReferencesUseCase) applyResolution(
	ctx context.Context,
	tx pgx.Tx,
	wallet *domain.Wallet,
	pendingTx *domain.WagerTransaction,
	refTx *domain.WagerTransaction,
	direction domain.LedgerDirection,
	balanceBefore domain.Money,
	output *ResolvePendingReferencesOutput,
) error {
	if err := uc.walletRepo.Update(ctx, tx, wallet); err != nil {
		return err
	}

	if err := pendingTx.MarkProcessed(); err != nil {
		return err
	}
	if err := uc.wagerRepo.Update(ctx, tx, pendingTx); err != nil {
		return err
	}

	ledgerEntry, err := domain.NewWalletLedgerEntry(
		uuid.NewString(),
		wallet.ID(),
		pendingTx.ID(),
		direction,
		pendingTx.Amount(),
		balanceBefore,
		wallet.Balance(),
	)
	if err != nil {
		return err
	}
	if err := uc.ledgerRepo.Create(ctx, tx, ledgerEntry); err != nil {
		return err
	}

	processedEvent, err := NewWagerTransactionProcessedOutboxMessage(wallet.ID(), "", WagerTransactionProcessedData{
		TransactionID:                  pendingTx.ID(),
		WalletID:                       wallet.ID(),
		PlayerID:                       wallet.PlayerID(),
		TransactionType:                string(pendingTx.TransactionType()),
		Amount:                         EventMoney{Amount: pendingTx.Amount().String(), Currency: pendingTx.Amount().Currency()},
		Status:                         domain.TransactionStatusProcessed,
		ProviderID:                     pendingTx.ProviderID(),
		ExternalTransactionID:          pendingTx.ExternalTransactionID(),
		ReferenceExternalTransactionID: pendingTx.ReferenceExternalTransactionID(),
		GameID:                         pendingTx.GameID(),
		RoundID:                        pendingTx.RoundID(),
	})
	if err != nil {
		return err
	}
	if err := uc.outboxRepo.Create(ctx, tx, processedEvent); err != nil {
		return err
	}

	balanceEvent, err := NewWalletBalanceChangedOutboxMessage(wallet.ID(), "", WalletBalanceChangedData{
		WalletID:      wallet.ID(),
		TransactionID: pendingTx.ID(),
		Direction:     string(direction),
		Money:         EventMoney{Amount: pendingTx.Amount().String(), Currency: pendingTx.Amount().Currency()},
		BalanceBefore: EventMoney{Amount: balanceBefore.String(), Currency: balanceBefore.Currency()},
		BalanceAfter:  EventMoney{Amount: wallet.Balance().String(), Currency: wallet.Currency()},
		WalletVersion: wallet.Version(),
	})
	if err != nil {
		return err
	}
	if err := uc.outboxRepo.Create(ctx, tx, balanceEvent); err != nil {
		return err
	}

	output.ResolvedCount++
	return nil
}

