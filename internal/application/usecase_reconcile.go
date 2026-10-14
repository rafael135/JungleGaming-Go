package application

import (
	"context"
	"log/slog"

	"jungle/internal/domain"

	"github.com/jackc/pgx/v5"
)

type ReconciliationMetrics interface {
	RecordReconciliation(walletID string, consistent bool, differenceCents int64)
}

type noopReconciliationMetrics struct{}

func (n *noopReconciliationMetrics) RecordReconciliation(walletID string, consistent bool, differenceCents int64) {
}

type ReconcileWalletInput struct {
	WalletID string
}

type ReconcileWalletOutput struct {
	WalletID          string        `json:"walletId"`
	StoredBalance     WalletBalance `json:"storedBalance"`
	CalculatedBalance WalletBalance `json:"calculatedBalance"`
	Difference        WalletBalance `json:"difference"`
	Consistent        bool          `json:"consistent"`
	CheckedEntries    int           `json:"checkedEntries"`
}

type ReconcileWalletUseCase struct {
	txManager  TransactionManager
	walletRepo WalletRepository
	ledgerRepo LedgerRepository
	metrics    ReconciliationMetrics
}

func NewReconcileWalletUseCase(
	txManager TransactionManager,
	walletRepo WalletRepository,
	ledgerRepo LedgerRepository,
	metrics ReconciliationMetrics,
) *ReconcileWalletUseCase {
	if metrics == nil {
		metrics = &noopReconciliationMetrics{}
	}
	return &ReconcileWalletUseCase{
		txManager:  txManager,
		walletRepo: walletRepo,
		ledgerRepo: ledgerRepo,
		metrics:    metrics,
	}
}

func (uc *ReconcileWalletUseCase) Execute(ctx context.Context, input ReconcileWalletInput) (*ReconcileWalletOutput, error) {
	if input.WalletID == "" {
		return nil, domain.ErrInvalidWalletId
	}

	var output *ReconcileWalletOutput

	err := uc.txManager.Execute(ctx, func(tx pgx.Tx) error {
		// 1. Obtém visão consistente da carteira (sem lock exclusivo)
		wallet, err := uc.walletRepo.GetByID(ctx, tx, input.WalletID, false)
		if err != nil {
			return ErrWalletNotFound
		}

		// 2. Obtém todos os lançamentos do ledger da carteira
		entries, err := uc.ledgerRepo.ListByWalletID(ctx, tx, input.WalletID)
		if err != nil {
			return err
		}

		// 3. Reconstrói o saldo acumulando créditos e débitos
		var calculatedCents int64
		for _, entry := range entries {
			switch entry.Direction() {
			case domain.LedgerDirectionCredit:
				calculatedCents += entry.Amount().Cents()
			case domain.LedgerDirectionDebit:
				calculatedCents -= entry.Amount().Cents()
			}
		}

		calculatedMoney, err := domain.NewMoney(calculatedCents, wallet.Currency())
		if err != nil {
			return err
		}

		// difference = storedBalance - calculatedBalance
		storedMoney := wallet.Balance()
		differenceMoney, err := storedMoney.Subtract(calculatedMoney)
		if err != nil {
			return err
		}

		consistent := differenceMoney.Cents() == 0

		// 4. Reporta divergências nos logs e em métricas se inconsistente
		if !consistent {
			slog.WarnContext(ctx, "divergencia detectada na reconciliacao da carteira",
				slog.String("wallet_id", wallet.ID()),
				slog.String("stored_balance", storedMoney.String()),
				slog.String("calculated_balance", calculatedMoney.String()),
				slog.String("difference", differenceMoney.String()),
				slog.Int("checked_entries", len(entries)),
			)
		}

		uc.metrics.RecordReconciliation(wallet.ID(), consistent, differenceMoney.Cents())

		output = &ReconcileWalletOutput{
			WalletID: wallet.ID(),
			StoredBalance: WalletBalance{
				Amount:   storedMoney.String(),
				Currency: storedMoney.Currency(),
			},
			CalculatedBalance: WalletBalance{
				Amount:   calculatedMoney.String(),
				Currency: calculatedMoney.Currency(),
			},
			Difference: WalletBalance{
				Amount:   differenceMoney.String(),
				Currency: differenceMoney.Currency(),
			},
			Consistent:     consistent,
			CheckedEntries: len(entries),
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return output, nil
}
