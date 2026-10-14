package application

import (
	"context"
	"jungle/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type WalletBalance struct {
	Amount   string
	Currency string
}

type CreateWalletInput struct {
	PlayerID       string
	InitialBalance WalletBalance
}

type CreateWalletOutput struct {
	ID       string
	PlayerID string
	Balance  WalletBalance
	Version  int64
}

type CreateWalletUseCase struct {
	txManager        TransactionManager
	walletRepository WalletRepository
	ledgerRepository LedgerRepository
	wagerRepository  WagerTransactionRepository
	outboxRepository OutboxRepository
}

func NewCreateWalletUseCase(
	txManager TransactionManager,
	walletRepository WalletRepository,
	ledgerRepository LedgerRepository,
	wagerRepository WagerTransactionRepository,
	outboxRepository OutboxRepository,
) *CreateWalletUseCase {
	return &CreateWalletUseCase{
		txManager:        txManager,
		walletRepository: walletRepository,
		ledgerRepository: ledgerRepository,
		wagerRepository:  wagerRepository,
		outboxRepository: outboxRepository,
	}
}

func (w *CreateWalletUseCase) Execute(
	ctx context.Context,
	input CreateWalletInput,
) (*CreateWalletOutput, error) {
	var output *CreateWalletOutput

	balance, err := domain.ParseMoney(input.InitialBalance.Amount, input.InitialBalance.Currency)

	if err != nil {
		return nil, err
	}

	if input.PlayerID == "" {
		return nil, domain.ErrInvalidPlayerId
	}

	err = w.txManager.Execute(ctx, func(tx pgx.Tx) error {
		newWalletUuid, err := uuid.NewUUID()

		wallet, err := domain.NewWallet(
			newWalletUuid.String(),
			input.PlayerID,
			balance,
		)

		if err != nil {
			return err
		}

		err = w.walletRepository.Create(ctx, tx, wallet)

		if err != nil {
			return err
		}

		if wallet.Balance().Cents() > 0 {
			transactionId, err := uuid.NewUUID()

			if err != nil {
				return err
			}

			openingLedger, err := domain.NewOpeningTransaction(
				transactionId.String(),
				wallet.ID(),
				wallet.PlayerID(),
				wallet.Balance(),
			)

			if err != nil {
				return err
			}

			err = w.wagerRepository.Create(ctx, tx, openingLedger)

			if err != nil {
				return err
			}

			ledgerId, err := uuid.NewUUID()

			if err != nil {
				return err
			}

			beforeMoney, err := domain.ZeroMoney(wallet.Currency())

			if err != nil {
				return err
			}

			afterMoney := wallet.Balance()

			ledgerEntry, err := domain.NewWalletLedgerEntry(
				ledgerId.String(),
				wallet.ID(),
				transactionId.String(),
				domain.LedgerDirectionCredit,
				wallet.Balance(),
				beforeMoney,
				afterMoney,
			)

			if err != nil {
				return err
			}

			err = w.ledgerRepository.Create(ctx, tx, ledgerEntry)

			if err != nil {
				return err
			}

			// Emite evento para WagerTransactionProcessed "OPENING"
			openingEvent, err := NewWagerTransactionProcessedOutboxMessage(
				wallet.ID(),
				"",
				WagerTransactionProcessedData{
					TransactionID:   openingLedger.ID(),
					WalletID:        wallet.ID(),
					PlayerID:        wallet.PlayerID(),
					TransactionType: domain.TransactionTypeOpening,
					Amount: EventMoney{
						Amount:   wallet.Balance().String(),
						Currency: wallet.Currency(),
					},
					Status: domain.TransactionStatusProcessed,
				},
			)

			if err != nil {
				return err
			}

			if err := w.outboxRepository.Create(ctx, tx, openingEvent); err != nil {
				return err
			}

			// Emite evento para WalletBalanceChanged
			balanceChangedEvent, err := NewWalletBalanceChangedOutboxMessage(
				wallet.ID(),
				"",
				WalletBalanceChangedData{
					WalletID:      wallet.ID(),
					TransactionID: transactionId.String(),
					Direction:     string(domain.LedgerDirectionCredit),
					Money: EventMoney{
						Amount:   wallet.Balance().String(),
						Currency: wallet.Currency(),
					},
					BalanceBefore: EventMoney{
						Amount:   beforeMoney.String(),
						Currency: beforeMoney.Currency(),
					},
					BalanceAfter: EventMoney{
						Amount:   afterMoney.String(),
						Currency: afterMoney.Currency(),
					},
					WalletVersion: wallet.Version(),
				},
			)

			if err != nil {
				return err
			}

			if err := w.outboxRepository.Create(ctx, tx, balanceChangedEvent); err != nil {
				return err
			}
		}

		output = &CreateWalletOutput{
			ID:       wallet.ID(),
			PlayerID: wallet.PlayerID(),
			Version:  wallet.Version(),
			Balance: WalletBalance{
				Amount:   wallet.Balance().String(),
				Currency: wallet.Currency(),
			},
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return output, nil
}
