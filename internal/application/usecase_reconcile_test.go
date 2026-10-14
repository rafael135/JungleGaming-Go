package application_test

import (
	"context"
	"errors"
	"testing"

	"jungle/internal/application"
	"jungle/internal/domain"
)

type mockReconciliationMetrics struct {
	called         bool
	lastWalletID   string
	lastConsistent bool
	lastDiffCents  int64
}

func (m *mockReconciliationMetrics) RecordReconciliation(walletID string, consistent bool, differenceCents int64) {
	m.called = true
	m.lastWalletID = walletID
	m.lastConsistent = consistent
	m.lastDiffCents = differenceCents
}

func TestReconcileWalletUseCase(t *testing.T) {
	ctx := context.Background()

	t.Run("sucesso: carteira consistente com multiplos lancamentos de credito e debito", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := newFakeWalletRepository()
		ledgerRepo := newFakeLedgerRepository()
		metrics := &mockReconciliationMetrics{}

		// Saldo atual esperado: 1000.00 - 25.00 + 50.00 - 50.00 = 975.00 BRL
		finalMoney, _ := domain.NewMoney(97500, "BRL")
		wallet, _ := domain.NewWallet("wallet-rec-1", "player-1", finalMoney)
		_ = walletRepo.Create(ctx, nil, wallet)

		// 1. OPENING: CREDIT 1000.00
		m1, _ := domain.NewMoney(100000, "BRL")
		b0, _ := domain.NewMoney(0, "BRL")
		e1, _ := domain.NewWalletLedgerEntry("l-1", wallet.ID(), "tx-open", domain.LedgerDirectionCredit, m1, b0, m1)
		_ = ledgerRepo.Create(ctx, nil, e1)

		// 2. BET: DEBIT 25.00
		m2, _ := domain.NewMoney(2500, "BRL")
		b1, _ := domain.NewMoney(100000, "BRL")
		b2, _ := domain.NewMoney(97500, "BRL")
		e2, _ := domain.NewWalletLedgerEntry("l-2", wallet.ID(), "tx-bet", domain.LedgerDirectionDebit, m2, b1, b2)
		_ = ledgerRepo.Create(ctx, nil, e2)

		// 3. WIN: CREDIT 50.00
		m3, _ := domain.NewMoney(5000, "BRL")
		b3, _ := domain.NewMoney(102500, "BRL")
		e3, _ := domain.NewWalletLedgerEntry("l-3", wallet.ID(), "tx-win", domain.LedgerDirectionCredit, m3, b2, b3)
		_ = ledgerRepo.Create(ctx, nil, e3)

		// 4. BET: DEBIT 50.00
		m4, _ := domain.NewMoney(5000, "BRL")
		b4, _ := domain.NewMoney(97500, "BRL")
		e4, _ := domain.NewWalletLedgerEntry("l-4", wallet.ID(), "tx-bet2", domain.LedgerDirectionDebit, m4, b3, b4)
		_ = ledgerRepo.Create(ctx, nil, e4)

		useCase := application.NewReconcileWalletUseCase(txMgr, walletRepo, ledgerRepo, metrics)

		output, err := useCase.Execute(ctx, application.ReconcileWalletInput{
			WalletID: wallet.ID(),
		})
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu erro: %v", err)
		}

		if output.WalletID != wallet.ID() {
			t.Errorf("walletID esperado %s, recebeu %s", wallet.ID(), output.WalletID)
		}
		if output.StoredBalance.Amount != "975.00" || output.StoredBalance.Currency != "BRL" {
			t.Errorf("storedBalance esperado 975.00 BRL, recebeu %s %s", output.StoredBalance.Amount, output.StoredBalance.Currency)
		}
		if output.CalculatedBalance.Amount != "975.00" || output.CalculatedBalance.Currency != "BRL" {
			t.Errorf("calculatedBalance esperado 975.00 BRL, recebeu %s %s", output.CalculatedBalance.Amount, output.CalculatedBalance.Currency)
		}
		if output.Difference.Amount != "0.00" || output.Difference.Currency != "BRL" {
			t.Errorf("difference esperada 0.00 BRL, recebeu %s %s", output.Difference.Amount, output.Difference.Currency)
		}
		if !output.Consistent {
			t.Errorf("esperava consistent=true")
		}
		if output.CheckedEntries != 4 {
			t.Errorf("checkedEntries esperado 4, recebeu %d", output.CheckedEntries)
		}

		if !metrics.called || !metrics.lastConsistent || metrics.lastDiffCents != 0 {
			t.Errorf("metrica gravada com dados incorretos")
		}
	})

	t.Run("divergencia detectada: saldo armazenado diverge da soma do ledger", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := newFakeWalletRepository()
		ledgerRepo := newFakeLedgerRepository()
		metrics := &mockReconciliationMetrics{}

		// Saldo na carteira foi alterado para 1000.00, mas no ledger só soma 950.00!
		storedMoney, _ := domain.NewMoney(100000, "BRL")
		wallet, _ := domain.NewWallet("wallet-rec-divergent", "player-1", storedMoney)
		_ = walletRepo.Create(ctx, nil, wallet)

		// Ledger soma 950.00
		m1, _ := domain.NewMoney(95000, "BRL")
		b0, _ := domain.NewMoney(0, "BRL")
		e1, _ := domain.NewWalletLedgerEntry("l-div-1", wallet.ID(), "tx-1", domain.LedgerDirectionCredit, m1, b0, m1)
		_ = ledgerRepo.Create(ctx, nil, e1)

		useCase := application.NewReconcileWalletUseCase(txMgr, walletRepo, ledgerRepo, metrics)

		output, err := useCase.Execute(ctx, application.ReconcileWalletInput{
			WalletID: wallet.ID(),
		})
		if err != nil {
			t.Fatalf("reconciliacao deve concluir com diferenca reportada, recebeu erro: %v", err)
		}

		if output.Consistent {
			t.Errorf("esperava consistent=false para carteira divergente")
		}
		if output.StoredBalance.Amount != "1000.00" {
			t.Errorf("storedBalance esperado 1000.00, recebeu %s", output.StoredBalance.Amount)
		}
		if output.CalculatedBalance.Amount != "950.00" {
			t.Errorf("calculatedBalance esperado 950.00, recebeu %s", output.CalculatedBalance.Amount)
		}
		if output.Difference.Amount != "50.00" {
			t.Errorf("difference esperada 50.00 (1000.00 - 950.00), recebeu %s", output.Difference.Amount)
		}
		if output.CheckedEntries != 1 {
			t.Errorf("checkedEntries esperado 1, recebeu %d", output.CheckedEntries)
		}

		if !metrics.called || metrics.lastConsistent || metrics.lastDiffCents != 5000 {
			t.Errorf("metrica esperada inconsistent com diff 5000 cents, recebeu: %v, diff: %d", metrics.lastConsistent, metrics.lastDiffCents)
		}
	})

	t.Run("caso de borda: carteira aberta com saldo zero e sem lancamentos no ledger", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := newFakeWalletRepository()
		ledgerRepo := newFakeLedgerRepository()

		zeroMoney, _ := domain.ZeroMoney("BRL")
		wallet, _ := domain.NewWallet("wallet-rec-zero", "player-1", zeroMoney)
		_ = walletRepo.Create(ctx, nil, wallet)

		useCase := application.NewReconcileWalletUseCase(txMgr, walletRepo, ledgerRepo, nil)

		output, err := useCase.Execute(ctx, application.ReconcileWalletInput{
			WalletID: wallet.ID(),
		})
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu %v", err)
		}

		if !output.Consistent {
			t.Errorf("esperava consistent=true para carteira zero")
		}
		if output.StoredBalance.Amount != "0.00" || output.CalculatedBalance.Amount != "0.00" || output.Difference.Amount != "0.00" {
			t.Errorf("todos os saldos devem ser 0.00")
		}
		if output.CheckedEntries != 0 {
			t.Errorf("checkedEntries esperado 0, recebeu %d", output.CheckedEntries)
		}
	})

	t.Run("caso de borda: WalletID vazia retorna ErrInvalidWalletId", func(t *testing.T) {
		useCase := application.NewReconcileWalletUseCase(&fakeTxManager{}, newFakeWalletRepository(), newFakeLedgerRepository(), nil)

		_, err := useCase.Execute(ctx, application.ReconcileWalletInput{WalletID: ""})
		if !errors.Is(err, domain.ErrInvalidWalletId) {
			t.Errorf("esperava ErrInvalidWalletId, recebeu %v", err)
		}
	})

	t.Run("caso de borda: carteira nao encontrada retorna ErrWalletNotFound", func(t *testing.T) {
		useCase := application.NewReconcileWalletUseCase(&fakeTxManager{}, newFakeWalletRepository(), newFakeLedgerRepository(), nil)

		_, err := useCase.Execute(ctx, application.ReconcileWalletInput{WalletID: "wallet-fantasma"})
		if !errors.Is(err, application.ErrWalletNotFound) {
			t.Errorf("esperava ErrWalletNotFound, recebeu %v", err)
		}
	})
}
