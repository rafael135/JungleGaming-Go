package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"jungle/internal/application"
	"jungle/internal/domain"
)

func TestGetWalletUseCase(t *testing.T) {
	ctx := context.Background()
	txMgr := &fakeTxManager{}
	walletRepo := newFakeWalletRepository()

	m, _ := domain.NewMoney(15000, "BRL")
	wallet, _ := domain.NewWallet("wallet-q-1", "player-q-1", m)
	_ = walletRepo.Create(ctx, nil, wallet)

	useCase := application.NewGetWalletUseCase(txMgr, walletRepo)

	t.Run("sucesso: retorna detalhes da carteira", func(t *testing.T) {
		output, err := useCase.Execute(ctx, application.GetWalletInput{
			WalletID: wallet.ID(),
		})
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu erro: %v", err)
		}
		if output.ID != wallet.ID() {
			t.Errorf("ID esperado %s, recebeu %s", wallet.ID(), output.ID)
		}
		if output.PlayerID != wallet.PlayerID() {
			t.Errorf("PlayerID esperado %s, recebeu %s", wallet.PlayerID(), output.PlayerID)
		}
		if output.Balance.Amount != "150.00" || output.Balance.Currency != "BRL" {
			t.Errorf("saldo esperado 150.00 BRL, recebeu %s %s", output.Balance.Amount, output.Balance.Currency)
		}
		if output.Version != 1 {
			t.Errorf("versao esperada 1, recebeu %d", output.Version)
		}
	})

	t.Run("caso de borda: WalletID vazia", func(t *testing.T) {
		_, err := useCase.Execute(ctx, application.GetWalletInput{WalletID: ""})
		if !errors.Is(err, domain.ErrInvalidWalletId) {
			t.Errorf("esperava ErrInvalidWalletId, recebeu %v", err)
		}
	})

	t.Run("caso de borda: carteira nao encontrada", func(t *testing.T) {
		_, err := useCase.Execute(ctx, application.GetWalletInput{WalletID: "wallet-fantasma"})
		if !errors.Is(err, application.ErrWalletNotFound) {
			t.Errorf("esperava ErrWalletNotFound, recebeu %v", err)
		}
	})
}

func TestGetWalletLedgerUseCase(t *testing.T) {
	ctx := context.Background()
	txMgr := &fakeTxManager{}
	walletRepo := newFakeWalletRepository()
	ledgerRepo := newFakeLedgerRepository()

	m, _ := domain.NewMoney(10000, "BRL")
	wallet, _ := domain.NewWallet("wallet-ledger-test", "player-1", m)
	_ = walletRepo.Create(ctx, nil, wallet)

	// Cria 3 entradas com timestamps distintos
	now := time.Now().UTC()
	b0, _ := domain.NewMoney(0, "BRL")
	m1, _ := domain.NewMoney(5000, "BRL")
	m2, _ := domain.NewMoney(2000, "BRL")
	bAfter2, _ := domain.NewMoney(3000, "BRL")
	m3, _ := domain.NewMoney(7000, "BRL")

	e1, err1 := domain.RehydrateWalletLedgerEntry("e-1", wallet.ID(), "tx-1", domain.LedgerDirectionCredit, m1, b0, m1, now.Add(-3*time.Minute))
	if err1 != nil {
		t.Fatalf("erro e1: %v", err1)
	}
	e2, err2 := domain.RehydrateWalletLedgerEntry("e-2", wallet.ID(), "tx-2", domain.LedgerDirectionDebit, m2, m1, bAfter2, now.Add(-2*time.Minute))
	if err2 != nil {
		t.Fatalf("erro e2: %v", err2)
	}
	e3, err3 := domain.RehydrateWalletLedgerEntry("e-3", wallet.ID(), "tx-3", domain.LedgerDirectionCredit, m3, bAfter2, m, now.Add(-1*time.Minute))
	if err3 != nil {
		t.Fatalf("erro e3: %v", err3)
	}

	_ = ledgerRepo.Create(ctx, nil, e1)
	_ = ledgerRepo.Create(ctx, nil, e2)
	_ = ledgerRepo.Create(ctx, nil, e3)

	useCase := application.NewGetWalletLedgerUseCase(txMgr, walletRepo, ledgerRepo)

	t.Run("sucesso: paginacao estavel via cursor opaco", func(t *testing.T) {
		// Página 1 com limit=2 -> deve retornar e1 e e2, e um nextCursor
		page1, err := useCase.Execute(ctx, application.GetWalletLedgerInput{
			WalletID: wallet.ID(),
			Limit:    2,
		})
		if err != nil {
			t.Fatalf("erro na pagina 1: %v", err)
		}
		if len(page1.Items) != 2 {
			t.Fatalf("esperava 2 itens na pagina 1, recebeu %d", len(page1.Items))
		}
		if page1.Items[0].ID != "e-1" || page1.Items[1].ID != "e-2" {
			t.Errorf("itens incorretos na pagina 1")
		}
		if page1.NextCursor == "" {
			t.Fatalf("nextCursor deveria estar presente na pagina 1")
		}

		// Página 2 usando o cursor retornado -> deve retornar e3 e nextCursor vazio
		page2, err := useCase.Execute(ctx, application.GetWalletLedgerInput{
			WalletID: wallet.ID(),
			Limit:    2,
			Cursor:   page1.NextCursor,
		})
		if err != nil {
			t.Fatalf("erro na pagina 2: %v", err)
		}
		if len(page2.Items) != 1 {
			t.Fatalf("esperava 1 item na pagina 2, recebeu %d", len(page2.Items))
		}
		if page2.Items[0].ID != "e-3" {
			t.Errorf("item esperado na pagina 2 era e-3, recebeu %s", page2.Items[0].ID)
		}
		if page2.NextCursor != "" {
			t.Errorf("nextCursor deveria ser vazio na ultima pagina, recebeu %s", page2.NextCursor)
		}
	})

	t.Run("caso de borda: cursor invalido retorna ErrInvalidCursor", func(t *testing.T) {
		_, err := useCase.Execute(ctx, application.GetWalletLedgerInput{
			WalletID: wallet.ID(),
			Limit:    10,
			Cursor:   "cursor-corrompido-totalmente-invalido",
		})
		if !errors.Is(err, application.ErrInvalidCursor) {
			t.Errorf("esperava ErrInvalidCursor, recebeu %v", err)
		}
	})

	t.Run("caso de borda: carteira nao encontrada", func(t *testing.T) {
		_, err := useCase.Execute(ctx, application.GetWalletLedgerInput{
			WalletID: "wallet-fantasma",
			Limit:    10,
		})
		if !errors.Is(err, application.ErrWalletNotFound) {
			t.Errorf("esperava ErrWalletNotFound, recebeu %v", err)
		}
	})
}

func TestGetWagerTransactionUseCase(t *testing.T) {
	ctx := context.Background()
	txMgr := &fakeTxManager{}
	wagerRepo := newFakeWagerRepository()

	money, _ := domain.NewMoney(2500, "BRL")
	txObj, _ := domain.NewWagerTransaction(
		"tx-uuid-123",
		"ext-tx-456",
		"provider-alpha",
		"alpha:idem-789",
		"hash-123",
		"wallet-1",
		"player-1",
		"fortune-tiger",
		"round-10",
		domain.TransactionTypeBet,
		money,
		"",
	)
	_ = txObj.MarkProcessed()
	_ = wagerRepo.Create(ctx, nil, txObj)

	useCase := application.NewGetWagerTransactionUseCase(txMgr, wagerRepo)

	t.Run("sucesso: busca por ID interno da transacao", func(t *testing.T) {
		output, err := useCase.Execute(ctx, application.GetWagerTransactionInput{
			TransactionID: "tx-uuid-123",
		})
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu erro: %v", err)
		}
		if output.ID != "tx-uuid-123" {
			t.Errorf("ID esperado tx-uuid-123, recebeu %s", output.ID)
		}
		if output.Status != domain.TransactionStatusProcessed {
			t.Errorf("status esperado PROCESSED, recebeu %s", output.Status)
		}
		if output.Amount.Amount != "25.00" {
			t.Errorf("valor esperado 25.00, recebeu %s", output.Amount.Amount)
		}
	})

	t.Run("sucesso: busca por providerId e externalTransactionId", func(t *testing.T) {
		output, err := useCase.Execute(ctx, application.GetWagerTransactionInput{
			ProviderID:            "provider-alpha",
			ExternalTransactionID: "ext-tx-456",
		})
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu erro: %v", err)
		}
		if output.ID != "tx-uuid-123" {
			t.Errorf("ID esperado tx-uuid-123, recebeu %s", output.ID)
		}
	})

	t.Run("caso de borda: parametros insuficientes retorna ErrInvalidTransactionQuery", func(t *testing.T) {
		_, err := useCase.Execute(ctx, application.GetWagerTransactionInput{})
		if !errors.Is(err, application.ErrInvalidTransactionQuery) {
			t.Errorf("esperava ErrInvalidTransactionQuery, recebeu %v", err)
		}
	})

	t.Run("caso de borda: transacao nao encontrada retorna ErrTransactionNotFound", func(t *testing.T) {
		_, err := useCase.Execute(ctx, application.GetWagerTransactionInput{
			TransactionID: "tx-inexistente",
		})
		if !errors.Is(err, application.ErrTransactionNotFound) {
			t.Errorf("esperava ErrTransactionNotFound, recebeu %v", err)
		}
	})
}
