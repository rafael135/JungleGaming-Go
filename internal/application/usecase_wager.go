package application

import (
	"context"
	"fmt"
	"jungle/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ProcessWagerInput struct {
	IdempotencyKey                 string
	ProviderID                     string
	ExternalTransactionID          string
	PlayerID                       string
	WalletID                       string
	GameID                         string
	RoundID                        string
	Kind                           string
	Amount                         string
	Currency                       string
	ReferenceExternalTransactionID string

	// Metadados de transporte (preenchidos quando a operação chega via SQS)
	ConsumerName string
	MessageID    string
}

type ProcessWagerOutput struct {
	TransactionID    string        `json:"transactionId"`
	Status           string        `json:"status"`
	Balance          WalletBalance `json:"balance"`
	IdempotentReplay bool          `json:"idempotentReplay"`
	FailureCode      string        `json:"failureCode,omitempty"`
}

type ProcessWagerUseCase struct {
	txManager  TransactionManager
	walletRepo WalletRepository
	ledgerRepo LedgerRepository
	wagerRepo  WagerTransactionRepository
	outboxRepo OutboxRepository
	inboxRepo  InboxRepository
}

func NewProcessWagerUseCase(
	txManager TransactionManager,
	walletRepo WalletRepository,
	ledgerRepo LedgerRepository,
	wagerRepo WagerTransactionRepository,
	outboxRepo OutboxRepository,
	inboxRepo InboxRepository,
) *ProcessWagerUseCase {
	return &ProcessWagerUseCase{
		txManager:  txManager,
		walletRepo: walletRepo,
		ledgerRepo: ledgerRepo,
		wagerRepo:  wagerRepo,
		outboxRepo: outboxRepo,
		inboxRepo:  inboxRepo,
	}
}

func (uc *ProcessWagerUseCase) Execute(ctx context.Context, input ProcessWagerInput) (*ProcessWagerOutput, error) {
	// 1. Validações preliminares obrigatórias
	if input.Kind == domain.TransactionTypeOpening {
		return nil, ErrOpeningTransactionForbidden
	}
	if input.IdempotencyKey == "" {
		return nil, domain.ErrInvalidIdempotencyKey
	}
	if input.ProviderID == "" {
		return nil, domain.ErrInvalidProviderId
	}
	if input.ExternalTransactionID == "" {
		return nil, domain.ErrInvalidExternalTransactionId
	}
	if input.PlayerID == "" {
		return nil, domain.ErrInvalidPlayerId
	}
	if input.WalletID == "" {
		return nil, domain.ErrInvalidWalletId
	}
	if input.GameID == "" {
		return nil, domain.ErrInvalidGameId
	}
	if input.RoundID == "" {
		return nil, domain.ErrInvalidRoundId
	}

	txType := domain.TransactionType(input.Kind)
	if txType != domain.TransactionTypeBet &&
		txType != domain.TransactionTypeWin &&
		txType != domain.TransactionTypeLoss &&
		txType != domain.TransactionTypeRefund &&
		txType != domain.TransactionTypeRollback {
		return nil, domain.ErrInvalidTransactionType
	}

	if (txType == domain.TransactionTypeRefund || txType == domain.TransactionTypeRollback) &&
		input.ReferenceExternalTransactionID == "" {
		return nil, domain.ErrInvalidReferenceExternalTransactionId
	}

	money, err := domain.ParseMoney(input.Amount, input.Currency)
	if err != nil {
		return nil, err
	}

	if txType == domain.TransactionTypeLoss {
		if money.Cents() != 0 {
			return nil, domain.ErrInvalidAmountFormat
		}
	} else {
		if money.Cents() <= 0 {
			return nil, domain.ErrNegativeAmount
		}
	}

	// 2. Hash canônico determinístico para detecção de conflitos
	payloadHash, err := ComputePayloadHash(input)
	if err != nil {
		return nil, err
	}

	var output *ProcessWagerOutput

	// 3. Execução da fronteira transacional
	err = uc.txManager.Execute(ctx, func(tx pgx.Tx) error {
		// 3.1. Deduplicação via Inbox se a operação veio do SQS
		if input.ConsumerName != "" && input.MessageID != "" {
			recorded, err := uc.inboxRepo.TryRecord(ctx, tx, input.ConsumerName, input.MessageID, payloadHash)
			if err != nil {
				return fmt.Errorf("erro ao registrar mensagem na inbox: %w", err)
			}
			if !recorded {
				// Mensagem já processada anteriormente por este consumidor SQS.
				// Tenta buscar o resultado original para devolver replay seguro.
				existing, err := uc.wagerRepo.GetByIdempotencyKey(ctx, tx, input.ProviderID, input.IdempotencyKey)
				if err == nil && existing != nil {
					bal, _ := uc.getObservedBalance(ctx, tx, existing)
					output = &ProcessWagerOutput{
						TransactionID:    existing.ID(),
						Status:           string(existing.Status()),
						Balance:          bal,
						IdempotentReplay: true,
						FailureCode:      existing.FailureCode(),
					}
					return nil
				}
				// Se ainda não estava commitado pela outra goroutine, retorna sem reprocessar
				return nil
			}
		}

		// 3.2. Idempotência por chave do provedor (provider_id + idempotency_key)
		existingTx, err := uc.wagerRepo.GetByIdempotencyKey(ctx, tx, input.ProviderID, input.IdempotencyKey)
		if err == nil && existingTx != nil {
			// Chave reutilizada com payload diferente -> 409 Conflict
			if existingTx.PayloadHash() != payloadHash {
				return ErrIdempotencyConflict
			}

			// Chave reutilizada com conteúdo idêntico -> Retorna replay com saldo observado original
			bal, err := uc.getObservedBalance(ctx, tx, existingTx)
			if err != nil {
				return err
			}

			output = &ProcessWagerOutput{
				TransactionID:    existingTx.ID(),
				Status:           string(existingTx.Status()),
				Balance:          bal,
				IdempotentReplay: true,
				FailureCode:      existingTx.FailureCode(),
			}
			return nil
		}

		// 3.3. Verificação de conflito no externalTransactionID
		// "Uma operação financeira identificada por (providerId, externalTransactionId) não pode ser reaplicada usando outra chave."
		byExt, err := uc.wagerRepo.GetByExternalID(ctx, tx, input.ProviderID, input.ExternalTransactionID)
		if err == nil && byExt != nil {
			if byExt.IdempotencyKey() != input.IdempotencyKey {
				return ErrExternalTransactionConflict
			}
		}

		// 3.4. Lock pessimista por carteira (SELECT ... FOR UPDATE)
		wallet, err := uc.walletRepo.GetByID(ctx, tx, input.WalletID, true)
		if err != nil {
			return ErrWalletNotFound
		}
		if wallet.PlayerID() != input.PlayerID {
			return ErrPlayerWalletMismatch
		}
		if wallet.Currency() != input.Currency {
			return ErrCurrencyMismatch
		}

		balanceBefore := wallet.Balance()
		txID := uuid.NewString()

		wagerTx, err := domain.NewWagerTransaction(
			txID,
			input.ExternalTransactionID,
			input.ProviderID,
			input.IdempotencyKey,
			payloadHash,
			wallet.ID(),
			wallet.PlayerID(),
			input.GameID,
			input.RoundID,
			txType,
			money,
			input.ReferenceExternalTransactionID,
		)
		if err != nil {
			return err
		}

		// 3.5. Roteamento por tipo de operação
		switch txType {
		case domain.TransactionTypeBet:
			if wallet.Balance().Cents() < money.Cents() {
				// Rejeição por saldo insuficiente
				if err := wagerTx.MarkRejected("INSUFFICIENT_FUNDS"); err != nil {
					return err
				}
				if err := uc.wagerRepo.Create(ctx, tx, wagerTx); err != nil {
					return err
				}

				event, err := NewWagerTransactionRejectedOutboxMessage(wallet.ID(), "", WagerTransactionRejectedData{
					TransactionID:         txID,
					WalletID:              wallet.ID(),
					PlayerID:              wallet.PlayerID(),
					TransactionType:       input.Kind,
					Amount:                EventMoney{Amount: money.String(), Currency: money.Currency()},
					Status:                domain.TransactionStatusRejected,
					FailureCode:           "INSUFFICIENT_FUNDS",
					ProviderID:            input.ProviderID,
					ExternalTransactionID: input.ExternalTransactionID,
				})
				if err != nil {
					return err
				}
				if err := uc.outboxRepo.Create(ctx, tx, event); err != nil {
					return err
				}

				output = &ProcessWagerOutput{
					TransactionID:    txID,
					Status:           domain.TransactionStatusRejected,
					Balance:          WalletBalance{Amount: wallet.Balance().String(), Currency: wallet.Currency()},
					IdempotentReplay: false,
					FailureCode:      "INSUFFICIENT_FUNDS",
				}
				return nil
			}

			// Saldo suficiente: debita
			if err := wallet.Debit(money); err != nil {
				return err
			}
			if err := uc.walletRepo.Update(ctx, tx, wallet); err != nil {
				return err
			}
			if err := wagerTx.MarkProcessed(); err != nil {
				return err
			}
			if err := uc.wagerRepo.Create(ctx, tx, wagerTx); err != nil {
				return err
			}

			ledgerEntry, err := domain.NewWalletLedgerEntry(
				uuid.NewString(),
				wallet.ID(),
				txID,
				domain.LedgerDirectionDebit,
				money,
				balanceBefore,
				wallet.Balance(),
			)
			if err != nil {
				return err
			}
			if err := uc.ledgerRepo.Create(ctx, tx, ledgerEntry); err != nil {
				return err
			}

			if err := uc.publishProcessedEvents(ctx, tx, wallet, txID, input, money, domain.LedgerDirectionDebit, balanceBefore); err != nil {
				return err
			}

			output = &ProcessWagerOutput{
				TransactionID:    txID,
				Status:           domain.TransactionStatusProcessed,
				Balance:          WalletBalance{Amount: wallet.Balance().String(), Currency: wallet.Currency()},
				IdempotentReplay: false,
			}

		case domain.TransactionTypeWin:
			if err := wallet.Credit(money); err != nil {
				return err
			}
			if err := uc.walletRepo.Update(ctx, tx, wallet); err != nil {
				return err
			}
			if err := wagerTx.MarkProcessed(); err != nil {
				return err
			}
			if err := uc.wagerRepo.Create(ctx, tx, wagerTx); err != nil {
				return err
			}

			ledgerEntry, err := domain.NewWalletLedgerEntry(
				uuid.NewString(),
				wallet.ID(),
				txID,
				domain.LedgerDirectionCredit,
				money,
				balanceBefore,
				wallet.Balance(),
			)
			if err != nil {
				return err
			}
			if err := uc.ledgerRepo.Create(ctx, tx, ledgerEntry); err != nil {
				return err
			}

			if err := uc.publishProcessedEvents(ctx, tx, wallet, txID, input, money, domain.LedgerDirectionCredit, balanceBefore); err != nil {
				return err
			}

			output = &ProcessWagerOutput{
				TransactionID:    txID,
				Status:           domain.TransactionStatusProcessed,
				Balance:          WalletBalance{Amount: wallet.Balance().String(), Currency: wallet.Currency()},
				IdempotentReplay: false,
			}

		case domain.TransactionTypeLoss:
			// LOSS não altera saldo nem versão da carteira e não gera lançamento no ledger
			if err := wagerTx.MarkProcessed(); err != nil {
				return err
			}
			if err := uc.wagerRepo.Create(ctx, tx, wagerTx); err != nil {
				return err
			}

			// Produz apenas WagerTransactionProcessed, sem WalletBalanceChanged
			processedEvent, err := NewWagerTransactionProcessedOutboxMessage(wallet.ID(), "", WagerTransactionProcessedData{
				TransactionID:                  txID,
				WalletID:                       wallet.ID(),
				PlayerID:                       wallet.PlayerID(),
				TransactionType:                input.Kind,
				Amount:                         EventMoney{Amount: money.String(), Currency: money.Currency()},
				Status:                         domain.TransactionStatusProcessed,
				ProviderID:                     input.ProviderID,
				ExternalTransactionID:          input.ExternalTransactionID,
				ReferenceExternalTransactionID: input.ReferenceExternalTransactionID,
				GameID:                         input.GameID,
				RoundID:                        input.RoundID,
			})
			if err != nil {
				return err
			}
			if err := uc.outboxRepo.Create(ctx, tx, processedEvent); err != nil {
				return err
			}

			output = &ProcessWagerOutput{
				TransactionID:    txID,
				Status:           domain.TransactionStatusProcessed,
				Balance:          WalletBalance{Amount: wallet.Balance().String(), Currency: wallet.Currency()},
				IdempotentReplay: false,
			}

		case domain.TransactionTypeRefund:
			refTx, err := uc.wagerRepo.GetByExternalID(ctx, tx, input.ProviderID, input.ReferenceExternalTransactionID)
			if err != nil {
				// Referência ainda não disponível -> Salva como PENDING_REFERENCE
				return uc.handlePendingReference(ctx, tx, wallet, wagerTx, txID, input, money, &output)
			}

			// Validações da referência
			if refTx.TransactionType() != domain.TransactionTypeBet ||
				refTx.Status() != domain.TransactionStatusProcessed ||
				refTx.PlayerID() != input.PlayerID ||
				refTx.WalletID() != input.WalletID ||
				refTx.RoundID() != input.RoundID ||
				refTx.Amount().Cents() != money.Cents() {
				return ErrReferenceMismatch
			}

			// REFUND credita o valor integral da aposta
			if err := wallet.Credit(money); err != nil {
				return err
			}
			if err := uc.walletRepo.Update(ctx, tx, wallet); err != nil {
				return err
			}
			if err := wagerTx.MarkProcessed(); err != nil {
				return err
			}
			if err := uc.wagerRepo.Create(ctx, tx, wagerTx); err != nil {
				return err
			}

			ledgerEntry, err := domain.NewWalletLedgerEntry(
				uuid.NewString(),
				wallet.ID(),
				txID,
				domain.LedgerDirectionCredit,
				money,
				balanceBefore,
				wallet.Balance(),
			)
			if err != nil {
				return err
			}
			if err := uc.ledgerRepo.Create(ctx, tx, ledgerEntry); err != nil {
				return err
			}

			if err := uc.publishProcessedEvents(ctx, tx, wallet, txID, input, money, domain.LedgerDirectionCredit, balanceBefore); err != nil {
				return err
			}

			output = &ProcessWagerOutput{
				TransactionID:    txID,
				Status:           domain.TransactionStatusProcessed,
				Balance:          WalletBalance{Amount: wallet.Balance().String(), Currency: wallet.Currency()},
				IdempotentReplay: false,
			}

		case domain.TransactionTypeRollback:
			refTx, err := uc.wagerRepo.GetByExternalID(ctx, tx, input.ProviderID, input.ReferenceExternalTransactionID)
			if err != nil {
				// Referência ainda não disponível -> Salva como PENDING_REFERENCE
				return uc.handlePendingReference(ctx, tx, wallet, wagerTx, txID, input, money, &output)
			}

			// Validações da referência
			if refTx.Status() != domain.TransactionStatusProcessed ||
				refTx.PlayerID() != input.PlayerID ||
				refTx.WalletID() != input.WalletID ||
				refTx.RoundID() != input.RoundID ||
				refTx.Amount().Cents() != money.Cents() {
				return ErrReferenceMismatch
			}

			// Movimento contrário ao original:
			// Se reverteu BET (que debitou) -> ROLLBACK credita
			// Se reverteu WIN ou REFUND (que creditou) -> ROLLBACK debita
			if refTx.TransactionType() == domain.TransactionTypeBet {
				if err := wallet.Credit(money); err != nil {
					return err
				}
				if err := uc.walletRepo.Update(ctx, tx, wallet); err != nil {
					return err
				}
				if err := wagerTx.MarkProcessed(); err != nil {
					return err
				}
				if err := uc.wagerRepo.Create(ctx, tx, wagerTx); err != nil {
					return err
				}

				ledgerEntry, err := domain.NewWalletLedgerEntry(
					uuid.NewString(),
					wallet.ID(),
					txID,
					domain.LedgerDirectionCredit,
					money,
					balanceBefore,
					wallet.Balance(),
				)
				if err != nil {
					return err
				}
				if err := uc.ledgerRepo.Create(ctx, tx, ledgerEntry); err != nil {
					return err
				}

				if err := uc.publishProcessedEvents(ctx, tx, wallet, txID, input, money, domain.LedgerDirectionCredit, balanceBefore); err != nil {
					return err
				}

				output = &ProcessWagerOutput{
					TransactionID:    txID,
					Status:           domain.TransactionStatusProcessed,
					Balance:          WalletBalance{Amount: wallet.Balance().String(), Currency: wallet.Currency()},
					IdempotentReplay: false,
				}
			} else if refTx.TransactionType() == domain.TransactionTypeWin || refTx.TransactionType() == domain.TransactionTypeRefund {
				// Se precisaria debitar mais do que o saldo disponível na carteira -> Rejeição auditável
				if wallet.Balance().Cents() < money.Cents() {
					if err := wagerTx.MarkRejected("INSUFFICIENT_FUNDS_FOR_ROLLBACK"); err != nil {
						return err
					}
					if err := uc.wagerRepo.Create(ctx, tx, wagerTx); err != nil {
						return err
					}

					event, err := NewWagerTransactionRejectedOutboxMessage(wallet.ID(), "", WagerTransactionRejectedData{
						TransactionID:         txID,
						WalletID:              wallet.ID(),
						PlayerID:              wallet.PlayerID(),
						TransactionType:       input.Kind,
						Amount:                EventMoney{Amount: money.String(), Currency: money.Currency()},
						Status:                domain.TransactionStatusRejected,
						FailureCode:           "INSUFFICIENT_FUNDS_FOR_ROLLBACK",
						ProviderID:            input.ProviderID,
						ExternalTransactionID: input.ExternalTransactionID,
					})
					if err != nil {
						return err
					}
					if err := uc.outboxRepo.Create(ctx, tx, event); err != nil {
						return err
					}

					output = &ProcessWagerOutput{
						TransactionID:    txID,
						Status:           domain.TransactionStatusRejected,
						Balance:          WalletBalance{Amount: wallet.Balance().String(), Currency: wallet.Currency()},
						IdempotentReplay: false,
						FailureCode:      "INSUFFICIENT_FUNDS_FOR_ROLLBACK",
					}
					return nil
				}

				// Saldo suficiente para desfazer o ganho/reembolso: debita
				if err := wallet.Debit(money); err != nil {
					return err
				}
				if err := uc.walletRepo.Update(ctx, tx, wallet); err != nil {
					return err
				}
				if err := wagerTx.MarkProcessed(); err != nil {
					return err
				}
				if err := uc.wagerRepo.Create(ctx, tx, wagerTx); err != nil {
					return err
				}

				ledgerEntry, err := domain.NewWalletLedgerEntry(
					uuid.NewString(),
					wallet.ID(),
					txID,
					domain.LedgerDirectionDebit,
					money,
					balanceBefore,
					wallet.Balance(),
				)
				if err != nil {
					return err
				}
				if err := uc.ledgerRepo.Create(ctx, tx, ledgerEntry); err != nil {
					return err
				}

				if err := uc.publishProcessedEvents(ctx, tx, wallet, txID, input, money, domain.LedgerDirectionDebit, balanceBefore); err != nil {
					return err
				}

				output = &ProcessWagerOutput{
					TransactionID:    txID,
					Status:           domain.TransactionStatusProcessed,
					Balance:          WalletBalance{Amount: wallet.Balance().String(), Currency: wallet.Currency()},
					IdempotentReplay: false,
				}
			} else {
				return ErrInvalidReferenceType
			}
		}

		// 3.6. Conclui mensagem de entrada na inbox caso tenha vindo do SQS
		if input.ConsumerName != "" && input.MessageID != "" {
			_ = uc.inboxRepo.MarkProcessed(ctx, tx, input.ConsumerName, input.MessageID)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return output, nil
}

// getObservedBalance recupera o saldo exato observado no processamento original para replay idempotente.
func (uc *ProcessWagerUseCase) getObservedBalance(ctx context.Context, tx pgx.Tx, existingTx *domain.WagerTransaction) (WalletBalance, error) {
	ledger, err := uc.ledgerRepo.GetByTransactionID(ctx, tx, existingTx.ID())
	if err == nil && ledger != nil {
		return WalletBalance{
			Amount:   ledger.BalanceAfter().String(),
			Currency: ledger.BalanceAfter().Currency(),
		}, nil
	}

	wallet, err := uc.walletRepo.GetByID(ctx, tx, existingTx.WalletID(), false)
	if err != nil {
		return WalletBalance{}, err
	}

	return WalletBalance{
		Amount:   wallet.Balance().String(),
		Currency: wallet.Currency(),
	}, nil
}

// handlePendingReference grava transação que chegou fora de ordem aguardando a referência.
func (uc *ProcessWagerUseCase) handlePendingReference(
	ctx context.Context,
	tx pgx.Tx,
	wallet *domain.Wallet,
	wagerTx *domain.WagerTransaction,
	txID string,
	input ProcessWagerInput,
	money domain.Money,
	output **ProcessWagerOutput,
) error {
	if err := wagerTx.MarkPendingReference(); err != nil {
		return err
	}
	if err := uc.wagerRepo.Create(ctx, tx, wagerTx); err != nil {
		return err
	}

	event, err := NewWagerTransactionPendingReferenceOutboxMessage(wallet.ID(), "", WagerTransactionPendingReferenceData{
		TransactionID:                  txID,
		WalletID:                       wallet.ID(),
		PlayerID:                       wallet.PlayerID(),
		TransactionType:                input.Kind,
		Amount:                         EventMoney{Amount: money.String(), Currency: money.Currency()},
		Status:                         domain.TransactionStatusPendingReference,
		ProviderID:                     input.ProviderID,
		ExternalTransactionID:          input.ExternalTransactionID,
		ReferenceExternalTransactionID: input.ReferenceExternalTransactionID,
	})
	if err != nil {
		return err
	}
	if err := uc.outboxRepo.Create(ctx, tx, event); err != nil {
		return err
	}

	*output = &ProcessWagerOutput{
		TransactionID:    txID,
		Status:           domain.TransactionStatusPendingReference,
		Balance:          WalletBalance{Amount: wallet.Balance().String(), Currency: wallet.Currency()},
		IdempotentReplay: false,
	}

	if input.ConsumerName != "" && input.MessageID != "" {
		_ = uc.inboxRepo.MarkProcessed(ctx, tx, input.ConsumerName, input.MessageID)
	}

	return nil
}

// publishProcessedEvents emite WagerTransactionProcessed e WalletBalanceChanged na mesma transação.
func (uc *ProcessWagerUseCase) publishProcessedEvents(
	ctx context.Context,
	tx pgx.Tx,
	wallet *domain.Wallet,
	txID string,
	input ProcessWagerInput,
	money domain.Money,
	direction domain.LedgerDirection,
	balanceBefore domain.Money,
) error {
	processedEvent, err := NewWagerTransactionProcessedOutboxMessage(wallet.ID(), "", WagerTransactionProcessedData{
		TransactionID:                  txID,
		WalletID:                       wallet.ID(),
		PlayerID:                       wallet.PlayerID(),
		TransactionType:                input.Kind,
		Amount:                         EventMoney{Amount: money.String(), Currency: money.Currency()},
		Status:                         domain.TransactionStatusProcessed,
		ProviderID:                     input.ProviderID,
		ExternalTransactionID:          input.ExternalTransactionID,
		ReferenceExternalTransactionID: input.ReferenceExternalTransactionID,
		GameID:                         input.GameID,
		RoundID:                        input.RoundID,
	})
	if err != nil {
		return err
	}
	if err := uc.outboxRepo.Create(ctx, tx, processedEvent); err != nil {
		return err
	}

	balanceEvent, err := NewWalletBalanceChangedOutboxMessage(wallet.ID(), "", WalletBalanceChangedData{
		WalletID:      wallet.ID(),
		TransactionID: txID,
		Direction:     string(direction),
		Money:         EventMoney{Amount: money.String(), Currency: money.Currency()},
		BalanceBefore: EventMoney{Amount: balanceBefore.String(), Currency: balanceBefore.Currency()},
		BalanceAfter:  EventMoney{Amount: wallet.Balance().String(), Currency: wallet.Currency()},
		WalletVersion: wallet.Version(),
	})
	if err != nil {
		return err
	}

	return uc.outboxRepo.Create(ctx, tx, balanceEvent)
}
