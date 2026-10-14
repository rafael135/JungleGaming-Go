package application_test

import (
	"context"
	"errors"
	"testing"

	"jungle/internal/application"
	"jungle/internal/domain"
)

// --- SUÍTE DE TESTES UNITÁRIOS COM CASOS DE BORDA ---

func TestCreateWalletUseCase(t *testing.T) {
	ctx := context.Background()

	t.Run("sucesso: abertura com saldo positivo gera OPENING, ledger e 2 eventos na outbox", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := &fakeWalletRepository{}
		ledgerRepo := &fakeLedgerRepository{}
		wagerRepo := &fakeWagerRepository{}
		outboxRepo := &fakeOutboxRepository{}

		useCase := application.NewCreateWalletUseCase(txMgr, walletRepo, ledgerRepo, wagerRepo, outboxRepo)

		input := application.CreateWalletInput{
			PlayerID: "player-123",
			InitialBalance: application.WalletBalance{
				Amount:   "1000.00",
				Currency: "BRL",
			},
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu erro: %v", err)
		}

		// Valida DTO de saída
		if output.ID == "" {
			t.Errorf("ID da carteira nao deveria ser vazio")
		}
		if output.PlayerID != "player-123" {
			t.Errorf("esperava player-123, recebeu %s", output.PlayerID)
		}
		if output.Version != 1 {
			t.Errorf("versao inicial deve ser 1, recebeu %d", output.Version)
		}
		if output.Balance.Amount != "1000.00" || output.Balance.Currency != "BRL" {
			t.Errorf("saldo esperado 1000.00 BRL, recebeu %s %s", output.Balance.Amount, output.Balance.Currency)
		}

		// Valida que a carteira foi persistida
		if walletRepo.createdWallet == nil {
			t.Fatalf("carteira deveria ter sido persistida")
		}
		if walletRepo.createdWallet.Balance().Cents() != 100000 {
			t.Errorf("esperava 100000 cents na carteira, recebeu %d", walletRepo.createdWallet.Balance().Cents())
		}

		// Valida transação OPENING criada e em estado PROCESSED
		if wagerRepo.createdTx == nil {
			t.Fatalf("deveria ter criado transacao OPENING")
		}
		if wagerRepo.createdTx.TransactionType() != domain.TransactionTypeOpening {
			t.Errorf("esperava tipo OPENING, recebeu %s", wagerRepo.createdTx.TransactionType())
		}
		if wagerRepo.createdTx.Status() != domain.TransactionStatusProcessed {
			t.Errorf("status da OPENING deve ser PROCESSED, recebeu %s", wagerRepo.createdTx.Status())
		}

		// Valida lançamento de crédito no Ledger (balanceBefore = 0, balanceAfter = 1000)
		if len(ledgerRepo.createdEntries) != 1 {
			t.Fatalf("esperava 1 entrada no ledger, recebeu %d", len(ledgerRepo.createdEntries))
		}
		entry := ledgerRepo.createdEntries[0]
		if entry.Direction() != domain.LedgerDirectionCredit {
			t.Errorf("direcao do ledger deve ser CREDIT, recebeu %s", entry.Direction())
		}
		if entry.BalanceBefore().Cents() != 0 {
			t.Errorf("balanceBefore deve ser 0, recebeu %d", entry.BalanceBefore().Cents())
		}
		if entry.BalanceAfter().Cents() != 100000 {
			t.Errorf("balanceAfter deve ser 100000, recebeu %d", entry.BalanceAfter().Cents())
		}

		// Valida que foram gerados exatamente 2 eventos na outbox
		if len(outboxRepo.createdMessages) != 2 {
			t.Fatalf("esperava 2 mensagens na outbox, recebeu %d", len(outboxRepo.createdMessages))
		}
		eventTypes := []string{outboxRepo.createdMessages[0].EventType, outboxRepo.createdMessages[1].EventType}
		expectedTypes := map[string]bool{
			application.EventTypeWagerTransactionProcessed: true,
			application.EventTypeWalletBalanceChanged:      true,
		}
		for _, et := range eventTypes {
			if !expectedTypes[et] {
				t.Errorf("tipo de evento inesperado na outbox: %s", et)
			}
		}
	})

	t.Run("caso de borda: saldo zero cria carteira mas NAO cria OPENING, ledger nem outbox", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := &fakeWalletRepository{}
		ledgerRepo := &fakeLedgerRepository{}
		wagerRepo := &fakeWagerRepository{}
		outboxRepo := &fakeOutboxRepository{}

		useCase := application.NewCreateWalletUseCase(txMgr, walletRepo, ledgerRepo, wagerRepo, outboxRepo)

		input := application.CreateWalletInput{
			PlayerID: "player-zero",
			InitialBalance: application.WalletBalance{
				Amount:   "0.00",
				Currency: "BRL",
			},
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu erro: %v", err)
		}

		if output.Balance.Amount != "0.00" {
			t.Errorf("esperava saldo 0.00, recebeu %s", output.Balance.Amount)
		}
		if output.Version != 1 {
			t.Errorf("versao inicial deve ser 1 mesmo com saldo zero, recebeu %d", output.Version)
		}

		// Deve persistir a carteira
		if walletRepo.createdWallet == nil {
			t.Fatalf("carteira com saldo zero deveria ter sido persistida")
		}

		// ⚠️ NÃO deve criar OPENING, nem ledger, nem outbox (conforme seção 9 das instruções)
		if wagerRepo.createdTx != nil {
			t.Errorf("saldo zero NAO deve criar transacao OPENING")
		}
		if len(ledgerRepo.createdEntries) != 0 {
			t.Errorf("saldo zero NAO deve criar lancamento no ledger")
		}
		if len(outboxRepo.createdMessages) != 0 {
			t.Errorf("saldo zero NAO deve gerar eventos na outbox")
		}
	})

	t.Run("caso de borda: rejeita saldo inicial negativo", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := &fakeWalletRepository{}
		ledgerRepo := &fakeLedgerRepository{}
		wagerRepo := &fakeWagerRepository{}
		outboxRepo := &fakeOutboxRepository{}

		useCase := application.NewCreateWalletUseCase(txMgr, walletRepo, ledgerRepo, wagerRepo, outboxRepo)

		input := application.CreateWalletInput{
			PlayerID: "player-neg",
			InitialBalance: application.WalletBalance{
				Amount:   "-50.00",
				Currency: "BRL",
			},
		}

		_, err := useCase.Execute(ctx, input)
		if err == nil {
			t.Fatalf("deveria rejeitar saldo inicial negativo")
		}
		if !errors.Is(err, domain.ErrNegativeAmount) {
			t.Errorf("esperava ErrNegativeAmount, recebeu: %v", err)
		}

		// Nenhuma transação de banco deve ser aberta para entrada inválida
		if txMgr.executeCalled {
			t.Errorf("nao deveria ter aberto transacao para entrada invalida")
		}
	})

	t.Run("caso de borda: rejeita formato de valor invalido (letras, multiplos pontos, escala invalida)", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := &fakeWalletRepository{}
		ledgerRepo := &fakeLedgerRepository{}
		wagerRepo := &fakeWagerRepository{}
		outboxRepo := &fakeOutboxRepository{}

		useCase := application.NewCreateWalletUseCase(txMgr, walletRepo, ledgerRepo, wagerRepo, outboxRepo)

		invalidAmounts := []string{"abc", "100.000", "100,00", "", "100.5", "NaN"}
		for _, amt := range invalidAmounts {
			input := application.CreateWalletInput{
				PlayerID: "player-inv",
				InitialBalance: application.WalletBalance{
					Amount:   amt,
					Currency: "BRL",
				},
			}

			_, err := useCase.Execute(ctx, input)
			if err == nil {
				t.Errorf("deveria rejeitar valor com formato invalido: %q", amt)
			}
		}
	})

	t.Run("caso de borda: rejeita moeda com tamanho diferente de 3 letras ISO 4217", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := &fakeWalletRepository{}
		ledgerRepo := &fakeLedgerRepository{}
		wagerRepo := &fakeWagerRepository{}
		outboxRepo := &fakeOutboxRepository{}

		useCase := application.NewCreateWalletUseCase(txMgr, walletRepo, ledgerRepo, wagerRepo, outboxRepo)

		input := application.CreateWalletInput{
			PlayerID: "player-curr",
			InitialBalance: application.WalletBalance{
				Amount:   "10.00",
				Currency: "BR", // 2 letras em vez de 3
			},
		}

		_, err := useCase.Execute(ctx, input)
		if err == nil {
			t.Fatalf("deveria rejeitar moeda com 2 letras")
		}
		if !errors.Is(err, domain.ErrInvalidCurrency) {
			t.Errorf("esperava ErrInvalidCurrency, recebeu: %v", err)
		}
	})

	t.Run("caso de borda: rejeita PlayerID vazio", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := &fakeWalletRepository{}
		ledgerRepo := &fakeLedgerRepository{}
		wagerRepo := &fakeWagerRepository{}
		outboxRepo := &fakeOutboxRepository{}

		useCase := application.NewCreateWalletUseCase(txMgr, walletRepo, ledgerRepo, wagerRepo, outboxRepo)

		input := application.CreateWalletInput{
			PlayerID: "",
			InitialBalance: application.WalletBalance{
				Amount:   "100.00",
				Currency: "BRL",
			},
		}

		_, err := useCase.Execute(ctx, input)
		if err == nil {
			t.Fatalf("deveria rejeitar PlayerID vazio")
		}
		if !errors.Is(err, domain.ErrInvalidPlayerId) {
			t.Errorf("esperava ErrInvalidPlayerId, recebeu: %v", err)
		}
	})

	t.Run("caso de borda: falha ao persistir carteira repassa erro e aborta", func(t *testing.T) {
		txMgr := &fakeTxManager{}
		walletRepo := &fakeWalletRepository{createErr: errors.New("db unique violation")}
		ledgerRepo := &fakeLedgerRepository{}
		wagerRepo := &fakeWagerRepository{}
		outboxRepo := &fakeOutboxRepository{}

		useCase := application.NewCreateWalletUseCase(txMgr, walletRepo, ledgerRepo, wagerRepo, outboxRepo)

		input := application.CreateWalletInput{
			PlayerID: "player-dup",
			InitialBalance: application.WalletBalance{
				Amount:   "100.00",
				Currency: "BRL",
			},
		}

		_, err := useCase.Execute(ctx, input)
		if err == nil {
			t.Fatalf("esperava erro de banco de dados")
		}
		if err.Error() != "db unique violation" {
			t.Errorf("esperava 'db unique violation', recebeu: %v", err)
		}

		// Não deve prosseguir para ledger ou outbox se a criação da carteira falhou
		if wagerRepo.createdTx != nil {
			t.Errorf("nao deveria ter criado wager tx se carteira falhou")
		}
		if len(ledgerRepo.createdEntries) != 0 {
			t.Errorf("nao deveria ter criado ledger se carteira falhou")
		}
		if len(outboxRepo.createdMessages) != 0 {
			t.Errorf("nao deveria ter criado outbox se carteira falhou")
		}
	})
}

