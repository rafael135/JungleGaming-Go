package application_test

import (
	"context"
	"testing"
	"time"

	"jungle/internal/application"
	"jungle/internal/domain"
)

func TestResolvePendingReferencesUseCase(t *testing.T) {
	ctx := context.Background()

	t.Run("sucesso: resolve REFUND pendente quando aposta referenciada chega e eh processada", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := newFakeWalletRepository()
		ledgerRepo := newFakeLedgerRepository()
		wagerRepo := newFakeWagerRepository()
		outboxRepo := newFakeOutboxRepository()

		walletMoney, _ := domain.NewMoney(7500, "BRL") // 75.00 BRL
		wallet, _ := domain.NewWallet("wallet-res-1", "player-res-1", walletMoney)
		_ = walletRepo.Create(ctx, nil, wallet)

		// 1. Transação REFUND gravada no passado com status PENDING_REFERENCE
		refundMoney, _ := domain.NewMoney(2500, "BRL")
		pendingRefund, _ := domain.NewWagerTransaction(
			"tx-refund-id",
			"ext-refund-id",
			"provider-res",
			"idem-refund-1",
			"hash-ref",
			wallet.ID(),
			wallet.PlayerID(),
			"fortune-tiger",
			"round-res-1",
			domain.TransactionTypeRefund,
			refundMoney,
			"ext-bet-arrived",
		)
		_ = pendingRefund.MarkPendingReference()
		_ = wagerRepo.Create(ctx, nil, pendingRefund)

		// 2. A aposta referenciada chegou depois e foi concluída com PROCESSED
		betMoney, _ := domain.NewMoney(2500, "BRL")
		betTx, _ := domain.NewWagerTransaction(
			"tx-bet-arrived",
			"ext-bet-arrived",
			"provider-res",
			"idem-bet-arrived",
			"hash-bet",
			wallet.ID(),
			wallet.PlayerID(),
			"fortune-tiger",
			"round-res-1",
			domain.TransactionTypeBet,
			betMoney,
			"",
		)
		_ = betTx.MarkProcessed()
		_ = wagerRepo.Create(ctx, nil, betTx)

		useCase := application.NewResolvePendingReferencesUseCase(
			txMgr,
			walletRepo,
			ledgerRepo,
			wagerRepo,
			outboxRepo,
		)

		output, err := useCase.Execute(ctx, application.ResolvePendingReferencesInput{
			Limit: 10,
			TTL:   24 * time.Hour,
		})
		if err != nil {
			t.Fatalf("esperava sucesso na resolucao, recebeu: %v", err)
		}

		if output.ResolvedCount != 1 {
			t.Errorf("ResolvedCount esperado 1, recebeu %d", output.ResolvedCount)
		}
		if output.RejectedCount != 0 {
			t.Errorf("RejectedCount esperado 0, recebeu %d", output.RejectedCount)
		}

		// Valida que o REFUND pendente agora está PROCESSED
		updatedRefund, _ := wagerRepo.GetByID(ctx, nil, pendingRefund.ID())
		if updatedRefund.Status() != domain.TransactionStatusProcessed {
			t.Errorf("status esperado PROCESSED, recebeu %s", updatedRefund.Status())
		}

		// Carteira foi creditada (75.00 + 25.00 = 100.00)
		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Balance().String() != "100.00" {
			t.Errorf("saldo da carteira esperado 100.00, recebeu %s", updatedWallet.Balance().String())
		}

		// Ledger CREDIT gerado
		if len(ledgerRepo.createdEntries) != 1 {
			t.Fatalf("esperava 1 entrada no ledger, recebeu %d", len(ledgerRepo.createdEntries))
		}
		if ledgerRepo.createdEntries[0].Direction() != domain.LedgerDirectionCredit {
			t.Errorf("direcao esperada CREDIT, recebeu %s", ledgerRepo.createdEntries[0].Direction())
		}

		// 2 eventos outbox gerados
		if len(outboxRepo.createdMessages) != 2 {
			t.Fatalf("esperava 2 eventos outbox, recebeu %d", len(outboxRepo.createdMessages))
		}
	})

	t.Run("caso de borda: rejeicao por expiracao de TTL quando referencia nunca chega", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := newFakeWalletRepository()
		ledgerRepo := newFakeLedgerRepository()
		wagerRepo := newFakeWagerRepository()
		outboxRepo := newFakeOutboxRepository()

		walletMoney, _ := domain.NewMoney(10000, "BRL")
		wallet, _ := domain.NewWallet("wallet-res-2", "player-res-2", walletMoney)
		_ = walletRepo.Create(ctx, nil, wallet)

		// Transação criada com timestamp de 48 horas atrás
		expiredTime := time.Now().UTC().Add(-48 * time.Hour)
		refundMoney, _ := domain.NewMoney(2500, "BRL")
		expiredRefund, _ := domain.RehydrateWagerTransaction(
			"tx-expired-id",
			"ext-expired-id",
			"provider-res",
			"idem-expired",
			"hash-exp",
			wallet.ID(),
			wallet.PlayerID(),
			"fortune-tiger",
			"round-exp",
			domain.TransactionTypeRefund,
			refundMoney,
			"ext-bet-never-came",
			domain.TransactionStatusPendingReference,
			"",
			expiredTime,
			expiredTime,
		)
		_ = wagerRepo.Create(ctx, nil, expiredRefund)

		useCase := application.NewResolvePendingReferencesUseCase(
			txMgr,
			walletRepo,
			ledgerRepo,
			wagerRepo,
			outboxRepo,
		)

		output, err := useCase.Execute(ctx, application.ResolvePendingReferencesInput{
			Limit: 10,
			TTL:   24 * time.Hour, // TTL de 24h
		})
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu erro: %v", err)
		}

		if output.RejectedCount != 1 {
			t.Errorf("RejectedCount esperado 1, recebeu %d", output.RejectedCount)
		}
		if output.ResolvedCount != 0 {
			t.Errorf("ResolvedCount esperado 0, recebeu %d", output.ResolvedCount)
		}

		// Transação no banco marcada como REJECTED com REFERENCE_NOT_FOUND_EXPIRED
		updatedTx, _ := wagerRepo.GetByID(ctx, nil, expiredRefund.ID())
		if updatedTx.Status() != domain.TransactionStatusRejected {
			t.Errorf("status esperado REJECTED, recebeu %s", updatedTx.Status())
		}
		if updatedTx.FailureCode() != "REFERENCE_NOT_FOUND_EXPIRED" {
			t.Errorf("failureCode esperado REFERENCE_NOT_FOUND_EXPIRED, recebeu %s", updatedTx.FailureCode())
		}

		// Evento outbox WagerTransactionRejected emitido
		if len(outboxRepo.createdMessages) != 1 {
			t.Fatalf("esperava 1 mensagem outbox, recebeu %d", len(outboxRepo.createdMessages))
		}
		if outboxRepo.createdMessages[0].EventType != application.EventTypeWagerTransactionRejected {
			t.Errorf("evento esperado WagerTransactionRejected, recebeu %s", outboxRepo.createdMessages[0].EventType)
		}
	})

	t.Run("caso de borda: referencia encontrada terminou sem sucesso (REJECTED) rejeita pendente", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := newFakeWalletRepository()
		ledgerRepo := newFakeLedgerRepository()
		wagerRepo := newFakeWagerRepository()
		outboxRepo := newFakeOutboxRepository()

		walletMoney, _ := domain.NewMoney(10000, "BRL")
		wallet, _ := domain.NewWallet("wallet-res-3", "player-res-3", walletMoney)
		_ = walletRepo.Create(ctx, nil, wallet)

		// Aposta original foi REJEITADA por saldo insuficiente
		betMoney, _ := domain.NewMoney(2500, "BRL")
		betRejected, _ := domain.NewWagerTransaction(
			"tx-bet-rej",
			"ext-bet-rej",
			"provider-res",
			"idem-bet-rej",
			"hash-bet-rej",
			wallet.ID(),
			wallet.PlayerID(),
			"fortune-tiger",
			"round-rej",
			domain.TransactionTypeBet,
			betMoney,
			"",
		)
		_ = betRejected.MarkRejected("INSUFFICIENT_FUNDS")
		_ = wagerRepo.Create(ctx, nil, betRejected)

		// Estorno estava aguardando
		pendingRefund, _ := domain.NewWagerTransaction(
			"tx-refund-waiting",
			"ext-refund-waiting",
			"provider-res",
			"idem-ref-waiting",
			"hash-ref-waiting",
			wallet.ID(),
			wallet.PlayerID(),
			"fortune-tiger",
			"round-rej",
			domain.TransactionTypeRefund,
			betMoney,
			"ext-bet-rej",
		)
		_ = pendingRefund.MarkPendingReference()
		_ = wagerRepo.Create(ctx, nil, pendingRefund)

		useCase := application.NewResolvePendingReferencesUseCase(
			txMgr,
			walletRepo,
			ledgerRepo,
			wagerRepo,
			outboxRepo,
		)

		output, err := useCase.Execute(ctx, application.ResolvePendingReferencesInput{
			Limit: 10,
			TTL:   24 * time.Hour,
		})
		if err != nil {
			t.Fatalf("esperava sucesso na execucao, recebeu erro: %v", err)
		}

		if output.RejectedCount != 1 {
			t.Errorf("RejectedCount esperado 1, recebeu %d", output.RejectedCount)
		}

		updatedTx, _ := wagerRepo.GetByID(ctx, nil, pendingRefund.ID())
		if updatedTx.Status() != domain.TransactionStatusRejected {
			t.Errorf("status esperado REJECTED, recebeu %s", updatedTx.Status())
		}
		if updatedTx.FailureCode() != "REFERENCE_TERMINATED_UNSUCCESSFULLY" {
			t.Errorf("failureCode esperado REFERENCE_TERMINATED_UNSUCCESSFULLY, recebeu %s", updatedTx.FailureCode())
		}
	})

	t.Run("caso de borda: referencia ainda ausente dentro do TTL permanece em PENDING_REFERENCE", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := newFakeWalletRepository()
		ledgerRepo := newFakeLedgerRepository()
		wagerRepo := newFakeWagerRepository()
		outboxRepo := newFakeOutboxRepository()

		walletMoney, _ := domain.NewMoney(10000, "BRL")
		wallet, _ := domain.NewWallet("wallet-res-4", "player-res-4", walletMoney)
		_ = walletRepo.Create(ctx, nil, wallet)

		// Transação recente (menos de 5 minutos)
		money, _ := domain.NewMoney(1000, "BRL")
		pendingTx, _ := domain.NewWagerTransaction(
			"tx-pending-recent",
			"ext-pending-recent",
			"provider-res",
			"idem-recent",
			"hash-recent",
			wallet.ID(),
			wallet.PlayerID(),
			"fortune-tiger",
			"round-recent",
			domain.TransactionTypeRefund,
			money,
			"ext-missing-still",
		)
		_ = pendingTx.MarkPendingReference()
		_ = wagerRepo.Create(ctx, nil, pendingTx)

		useCase := application.NewResolvePendingReferencesUseCase(txMgr, walletRepo, ledgerRepo, wagerRepo, outboxRepo)

		output, err := useCase.Execute(ctx, application.ResolvePendingReferencesInput{
			Limit: 10,
			TTL:   24 * time.Hour,
		})
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu %v", err)
		}

		if output.RemainingCount != 1 {
			t.Errorf("RemainingCount esperado 1, recebeu %d", output.RemainingCount)
		}
		if output.ResolvedCount != 0 || output.RejectedCount != 0 {
			t.Errorf("ResolvedCount e RejectedCount devem ser 0")
		}

		updatedTx, _ := wagerRepo.GetByID(ctx, nil, pendingTx.ID())
		if updatedTx.Status() != domain.TransactionStatusPendingReference {
			t.Errorf("transacao recente deve permanecer PENDING_REFERENCE, status atual: %s", updatedTx.Status())
		}
	})
}

