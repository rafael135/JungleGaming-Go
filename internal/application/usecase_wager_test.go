package application_test

import (
	"context"
	"errors"
	"testing"

	"jungle/internal/application"
	"jungle/internal/domain"
)

func setupWagerTest(t *testing.T, initialCents int64) (
	context.Context,
	*fakeTxManager,
	*fakeWalletRepository,
	*fakeLedgerRepository,
	*fakeWagerRepository,
	*fakeOutboxRepository,
	*fakeInboxRepository,
	*application.ProcessWagerUseCase,
	*domain.Wallet,
) {
	t.Helper()
	ctx := context.Background()
	txMgr := &fakeTxManager{}
	walletRepo := newFakeWalletRepository()
	ledgerRepo := newFakeLedgerRepository()
	wagerRepo := newFakeWagerRepository()
	outboxRepo := newFakeOutboxRepository()
	inboxRepo := newFakeInboxRepository()

	initialMoney, err := domain.NewMoney(initialCents, "BRL")
	if err != nil {
		t.Fatalf("erro ao criar saldo inicial: %v", err)
	}

	wallet, err := domain.NewWallet("wallet-123", "player-456", initialMoney)
	if err != nil {
		t.Fatalf("erro ao criar carteira: %v", err)
	}
	_ = walletRepo.Create(ctx, nil, wallet)

	useCase := application.NewProcessWagerUseCase(
		txMgr,
		walletRepo,
		ledgerRepo,
		wagerRepo,
		outboxRepo,
		inboxRepo,
	)

	return ctx, txMgr, walletRepo, ledgerRepo, wagerRepo, outboxRepo, inboxRepo, useCase, wallet
}

// --- 1. TESTES DE BET ---

func TestProcessWagerUseCase_Bet(t *testing.T) {
	t.Run("sucesso: aposta debita saldo, incrementa versao, cria ledger DEBIT e publica 2 eventos outbox", func(t *testing.T) {
		ctx, _, walletRepo, ledgerRepo, wagerRepo, outboxRepo, _, useCase, wallet := setupWagerTest(t, 10000) // 100.00 BRL

		input := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:bet-1",
			ProviderID:            "provider-a",
			ExternalTransactionID: "bet-1",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-100",
			Kind:                  "BET",
			Amount:                "25.00",
			Currency:              "BRL",
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu erro: %v", err)
		}

		if output.Status != domain.TransactionStatusProcessed {
			t.Errorf("status esperado PROCESSED, recebeu %s", output.Status)
		}
		if output.Balance.Amount != "75.00" || output.Balance.Currency != "BRL" {
			t.Errorf("saldo esperado 75.00 BRL, recebeu %s %s", output.Balance.Amount, output.Balance.Currency)
		}
		if output.IdempotentReplay {
			t.Errorf("idempotentReplay esperado false")
		}

		// Valida atualização da carteira
		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Balance().String() != "75.00" {
			t.Errorf("saldo da carteira persistida esperado 75.00, recebeu %s", updatedWallet.Balance().String())
		}
		if updatedWallet.Version() != 2 {
			t.Errorf("versao da carteira esperado 2, recebeu %d", updatedWallet.Version())
		}

		// Valida lançamento no ledger
		if len(ledgerRepo.createdEntries) != 1 {
			t.Fatalf("esperava 1 entrada no ledger, recebeu %d", len(ledgerRepo.createdEntries))
		}
		ledger := ledgerRepo.createdEntries[0]
		if ledger.Direction() != domain.LedgerDirectionDebit {
			t.Errorf("direcao esperada DEBIT, recebeu %s", ledger.Direction())
		}
		if ledger.Amount().String() != "25.00" {
			t.Errorf("valor do ledger esperado 25.00, recebeu %s", ledger.Amount().String())
		}
		if ledger.BalanceBefore().String() != "100.00" || ledger.BalanceAfter().String() != "75.00" {
			t.Errorf("ledger balances antes/depois invalidos: %s / %s", ledger.BalanceBefore().String(), ledger.BalanceAfter().String())
		}

		// Valida transação persistida
		savedTx, err := wagerRepo.GetByIdempotencyKey(ctx, nil, "provider-a", "prov1:bet-1")
		if err != nil || savedTx == nil {
			t.Fatalf("transacao deveria ter sido persistida")
		}
		if savedTx.Status() != domain.TransactionStatusProcessed {
			t.Errorf("status da transacao persistida esperado PROCESSED, recebeu %s", savedTx.Status())
		}

		// Valida eventos na outbox (WagerTransactionProcessed + WalletBalanceChanged)
		if len(outboxRepo.createdMessages) != 2 {
			t.Fatalf("esperava 2 mensagens na outbox, recebeu %d", len(outboxRepo.createdMessages))
		}
		if outboxRepo.createdMessages[0].EventType != application.EventTypeWagerTransactionProcessed {
			t.Errorf("primeiro evento esperado %s, recebeu %s", application.EventTypeWagerTransactionProcessed, outboxRepo.createdMessages[0].EventType)
		}
		if outboxRepo.createdMessages[1].EventType != application.EventTypeWalletBalanceChanged {
			t.Errorf("segundo evento esperado %s, recebeu %s", application.EventTypeWalletBalanceChanged, outboxRepo.createdMessages[1].EventType)
		}
	})

	t.Run("rejeicao por saldo insuficiente: nao altera carteira nem ledger, grava REJECTED INSUFFICIENT_FUNDS e emite evento", func(t *testing.T) {
		ctx, _, walletRepo, ledgerRepo, wagerRepo, outboxRepo, _, useCase, wallet := setupWagerTest(t, 5000) // 50.00 BRL

		input := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:bet-2",
			ProviderID:            "provider-a",
			ExternalTransactionID: "bet-2",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-101",
			Kind:                  "BET",
			Amount:                "80.00", // Mais do que os 50.00 disponíveis!
			Currency:              "BRL",
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava processamento da rejeicao sem erro de execucao, recebeu: %v", err)
		}

		if output.Status != domain.TransactionStatusRejected {
			t.Errorf("status esperado REJECTED, recebeu %s", output.Status)
		}
		if output.FailureCode != "INSUFFICIENT_FUNDS" {
			t.Errorf("failureCode esperado INSUFFICIENT_FUNDS, recebeu %s", output.FailureCode)
		}
		if output.Balance.Amount != "50.00" {
			t.Errorf("saldo da carteira deve permanecer 50.00, recebeu %s", output.Balance.Amount)
		}

		// Carteira NÃO deve ter sido alterada
		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Balance().String() != "50.00" {
			t.Errorf("saldo da carteira nao deveria ter mudado")
		}
		if updatedWallet.Version() != 1 {
			t.Errorf("versao da carteira nao deveria ter mudado")
		}

		// Ledger NÃO deve ter sido gerado
		if len(ledgerRepo.createdEntries) != 0 {
			t.Errorf("rejeicao nao pode criar lancamento no ledger")
		}

		// Transação persistida como REJECTED
		savedTx, err := wagerRepo.GetByIdempotencyKey(ctx, nil, "provider-a", "prov1:bet-2")
		if err != nil || savedTx == nil {
			t.Fatalf("transacao rejeitada deveria ter sido persistida para auditoria")
		}
		if savedTx.Status() != domain.TransactionStatusRejected {
			t.Errorf("status esperado REJECTED, recebeu %s", savedTx.Status())
		}
		if savedTx.FailureCode() != "INSUFFICIENT_FUNDS" {
			t.Errorf("failureCode esperado INSUFFICIENT_FUNDS, recebeu %s", savedTx.FailureCode())
		}

		// Apenas 1 evento na outbox: WagerTransactionRejected (SEM WalletBalanceChanged)
		if len(outboxRepo.createdMessages) != 1 {
			t.Fatalf("esperava 1 mensagem na outbox, recebeu %d", len(outboxRepo.createdMessages))
		}
		if outboxRepo.createdMessages[0].EventType != application.EventTypeWagerTransactionRejected {
			t.Errorf("evento esperado %s, recebeu %s", application.EventTypeWagerTransactionRejected, outboxRepo.createdMessages[0].EventType)
		}
	})
}

// --- 2. TESTES DE WIN ---

func TestProcessWagerUseCase_Win(t *testing.T) {
	t.Run("sucesso: ganho credita saldo, incrementa versao, cria ledger CREDIT e publica 2 eventos outbox", func(t *testing.T) {
		ctx, _, walletRepo, ledgerRepo, _, outboxRepo, _, useCase, wallet := setupWagerTest(t, 10000) // 100.00 BRL

		input := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:win-1",
			ProviderID:            "provider-a",
			ExternalTransactionID: "win-1",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-100",
			Kind:                  "WIN",
			Amount:                "50.00",
			Currency:              "BRL",
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu erro: %v", err)
		}

		if output.Status != domain.TransactionStatusProcessed {
			t.Errorf("status esperado PROCESSED, recebeu %s", output.Status)
		}
		if output.Balance.Amount != "150.00" {
			t.Errorf("saldo esperado 150.00, recebeu %s", output.Balance.Amount)
		}

		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Balance().String() != "150.00" {
			t.Errorf("saldo da carteira esperado 150.00, recebeu %s", updatedWallet.Balance().String())
		}
		if updatedWallet.Version() != 2 {
			t.Errorf("versao esperada 2, recebeu %d", updatedWallet.Version())
		}

		if len(ledgerRepo.createdEntries) != 1 {
			t.Fatalf("esperava 1 entrada no ledger, recebeu %d", len(ledgerRepo.createdEntries))
		}
		ledger := ledgerRepo.createdEntries[0]
		if ledger.Direction() != domain.LedgerDirectionCredit {
			t.Errorf("direcao esperada CREDIT, recebeu %s", ledger.Direction())
		}
		if ledger.Amount().String() != "50.00" {
			t.Errorf("valor esperado 50.00, recebeu %s", ledger.Amount().String())
		}

		if len(outboxRepo.createdMessages) != 2 {
			t.Fatalf("esperava 2 mensagens na outbox, recebeu %d", len(outboxRepo.createdMessages))
		}
		if outboxRepo.createdMessages[0].EventType != application.EventTypeWagerTransactionProcessed {
			t.Errorf("evento esperado WagerTransactionProcessed, recebeu %s", outboxRepo.createdMessages[0].EventType)
		}
		if outboxRepo.createdMessages[1].EventType != application.EventTypeWalletBalanceChanged {
			t.Errorf("evento esperado WalletBalanceChanged, recebeu %s", outboxRepo.createdMessages[1].EventType)
		}
	})
}

// --- 3. TESTES DE LOSS ---

func TestProcessWagerUseCase_Loss(t *testing.T) {
	t.Run("sucesso: LOSS exige amount 0.00, nao altera saldo nem versao, nao cria ledger e emite apenas WagerTransactionProcessed", func(t *testing.T) {
		ctx, _, walletRepo, ledgerRepo, wagerRepo, outboxRepo, _, useCase, wallet := setupWagerTest(t, 10000) // 100.00 BRL

		input := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:loss-1",
			ProviderID:            "provider-a",
			ExternalTransactionID: "loss-1",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-100",
			Kind:                  "LOSS",
			Amount:                "0.00",
			Currency:              "BRL",
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu erro: %v", err)
		}

		if output.Status != domain.TransactionStatusProcessed {
			t.Errorf("status esperado PROCESSED, recebeu %s", output.Status)
		}
		if output.Balance.Amount != "100.00" {
			t.Errorf("saldo esperado 100.00, recebeu %s", output.Balance.Amount)
		}

		// Carteira inalterada
		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Balance().String() != "100.00" {
			t.Errorf("saldo da carteira nao deve mudar")
		}
		if updatedWallet.Version() != 1 {
			t.Errorf("versao da carteira nao deve mudar")
		}

		// Sem ledger
		if len(ledgerRepo.createdEntries) != 0 {
			t.Errorf("LOSS nao deve criar lancamento no ledger")
		}

		// Transação persistida
		savedTx, _ := wagerRepo.GetByIdempotencyKey(ctx, nil, "provider-a", "prov1:loss-1")
		if savedTx == nil || savedTx.Status() != domain.TransactionStatusProcessed {
			t.Fatalf("transacao deveria estar persistida com status PROCESSED")
		}

		// Apenas 1 evento na outbox: WagerTransactionProcessed (sem WalletBalanceChanged)
		if len(outboxRepo.createdMessages) != 1 {
			t.Fatalf("esperava exatamente 1 mensagem na outbox, recebeu %d", len(outboxRepo.createdMessages))
		}
		if outboxRepo.createdMessages[0].EventType != application.EventTypeWagerTransactionProcessed {
			t.Errorf("evento esperado WagerTransactionProcessed, recebeu %s", outboxRepo.createdMessages[0].EventType)
		}
	})

	t.Run("caso de borda: LOSS com amount diferente de 0.00 eh rejeitado", func(t *testing.T) {
		ctx, _, _, _, _, _, _, useCase, wallet := setupWagerTest(t, 10000)

		input := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:loss-2",
			ProviderID:            "provider-a",
			ExternalTransactionID: "loss-2",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-100",
			Kind:                  "LOSS",
			Amount:                "5.00", // Inválido para LOSS!
			Currency:              "BRL",
		}

		_, err := useCase.Execute(ctx, input)
		if !errors.Is(err, domain.ErrInvalidAmountFormat) {
			t.Errorf("esperava ErrInvalidAmountFormat, recebeu %v", err)
		}
	})
}

// --- 4. TESTES DE REFUND ---

func TestProcessWagerUseCase_Refund(t *testing.T) {
	t.Run("sucesso: REFUND de aposta BET credita saldo integral da aposta, cria ledger CREDIT e publica 2 eventos", func(t *testing.T) {
		ctx, _, walletRepo, ledgerRepo, wagerRepo, outboxRepo, _, useCase, wallet := setupWagerTest(t, 7500) // 75.00 BRL (após aposta prévia de 25.00)

		// Cria BET prévia no repositório de transações
		betMoney, _ := domain.NewMoney(2500, "BRL")
		betTx, err := domain.NewWagerTransaction(
			"tx-bet-original",
			"bet-original",
			"provider-a",
			"prov1:bet-original",
			"hash-original",
			wallet.ID(),
			wallet.PlayerID(),
			"fortune-tiger",
			"round-100",
			domain.TransactionTypeBet,
			betMoney,
			"",
		)
		if err != nil {
			t.Fatalf("erro ao criar betTx: %v", err)
		}
		_ = betTx.MarkProcessed()
		_ = wagerRepo.Create(ctx, nil, betTx)

		// Executa REFUND referenciando a BET
		input := application.ProcessWagerInput{
			IdempotencyKey:                 "prov1:refund-1",
			ProviderID:                     "provider-a",
			ExternalTransactionID:          "refund-1",
			PlayerID:                       wallet.PlayerID(),
			WalletID:                       wallet.ID(),
			GameID:                         "fortune-tiger",
			RoundID:                        "round-100",
			Kind:                           "REFUND",
			Amount:                         "25.00",
			Currency:                       "BRL",
			ReferenceExternalTransactionID: "bet-original",
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava sucesso no REFUND, recebeu erro: %v", err)
		}

		if output.Status != domain.TransactionStatusProcessed {
			t.Errorf("status esperado PROCESSED, recebeu %s", output.Status)
		}
		if output.Balance.Amount != "100.00" {
			t.Errorf("saldo esperado 100.00 (75.00 + 25.00), recebeu %s", output.Balance.Amount)
		}

		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Balance().String() != "100.00" {
			t.Errorf("saldo da carteira esperado 100.00, recebeu %s", updatedWallet.Balance().String())
		}

		if len(ledgerRepo.createdEntries) != 1 {
			t.Fatalf("esperava 1 entrada no ledger, recebeu %d", len(ledgerRepo.createdEntries))
		}
		if ledgerRepo.createdEntries[0].Direction() != domain.LedgerDirectionCredit {
			t.Errorf("ledger direction esperada CREDIT, recebeu %s", ledgerRepo.createdEntries[0].Direction())
		}

		if len(outboxRepo.createdMessages) != 2 {
			t.Fatalf("esperava 2 mensagens na outbox, recebeu %d", len(outboxRepo.createdMessages))
		}
	})

	t.Run("caso de borda: referencia ausente transiciona para PENDING_REFERENCE e grava evento outbox", func(t *testing.T) {
		ctx, _, walletRepo, ledgerRepo, wagerRepo, outboxRepo, _, useCase, wallet := setupWagerTest(t, 10000)

		input := application.ProcessWagerInput{
			IdempotencyKey:                 "prov1:refund-pending",
			ProviderID:                     "provider-a",
			ExternalTransactionID:          "refund-pending",
			PlayerID:                       wallet.PlayerID(),
			WalletID:                       wallet.ID(),
			GameID:                         "fortune-tiger",
			RoundID:                        "round-100",
			Kind:                           "REFUND",
			Amount:                         "25.00",
			Currency:                       "BRL",
			ReferenceExternalTransactionID: "bet-nao-existe-ainda",
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava processamento gracioso de PENDING_REFERENCE, recebeu erro: %v", err)
		}

		if output.Status != domain.TransactionStatusPendingReference {
			t.Errorf("status esperado PENDING_REFERENCE, recebeu %s", output.Status)
		}
		if output.Balance.Amount != "100.00" {
			t.Errorf("saldo da carteira nao deve mudar, esperado 100.00, recebeu %s", output.Balance.Amount)
		}

		// Carteira e ledger inalterados
		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Version() != 1 {
			t.Errorf("versao da carteira nao deve mudar")
		}
		if len(ledgerRepo.createdEntries) != 0 {
			t.Errorf("ledger nao deve ser criado para PENDING_REFERENCE")
		}

		// Transação persistida como PENDING_REFERENCE
		savedTx, _ := wagerRepo.GetByIdempotencyKey(ctx, nil, "provider-a", "prov1:refund-pending")
		if savedTx == nil || savedTx.Status() != domain.TransactionStatusPendingReference {
			t.Fatalf("transacao deveria estar persistida como PENDING_REFERENCE")
		}

		// Outbox contém evento WagerTransactionPendingReference
		if len(outboxRepo.createdMessages) != 1 {
			t.Fatalf("esperava 1 mensagem na outbox, recebeu %d", len(outboxRepo.createdMessages))
		}
		if outboxRepo.createdMessages[0].EventType != application.EventTypeWagerTransactionPendingReference {
			t.Errorf("evento esperado %s, recebeu %s", application.EventTypeWagerTransactionPendingReference, outboxRepo.createdMessages[0].EventType)
		}
	})

	t.Run("caso de borda: divergencia de atributos da referencia (valor diferente) retorna ErrReferenceMismatch", func(t *testing.T) {
		ctx, _, _, _, wagerRepo, _, _, useCase, wallet := setupWagerTest(t, 10000)

		// Cria BET de 25.00
		betMoney, _ := domain.NewMoney(2500, "BRL")
		betTx, _ := domain.NewWagerTransaction(
			"tx-bet-1", "ext-bet-1", "provider-a", "prov1:ext-bet-1", "hash-1",
			wallet.ID(), wallet.PlayerID(), "fortune-tiger", "round-100",
			domain.TransactionTypeBet, betMoney, "",
		)
		_ = betTx.MarkProcessed()
		_ = wagerRepo.Create(ctx, nil, betTx)

		// Tenta REFUND de 50.00 (valor divergente do original!)
		input := application.ProcessWagerInput{
			IdempotencyKey:                 "prov1:refund-mismatch",
			ProviderID:                     "provider-a",
			ExternalTransactionID:          "refund-mismatch",
			PlayerID:                       wallet.PlayerID(),
			WalletID:                       wallet.ID(),
			GameID:                         "fortune-tiger",
			RoundID:                        "round-100",
			Kind:                           "REFUND",
			Amount:                         "50.00", // Diferente de 25.00
			Currency:                       "BRL",
			ReferenceExternalTransactionID: "ext-bet-1",
		}

		_, err := useCase.Execute(ctx, input)
		if !errors.Is(err, application.ErrReferenceMismatch) {
			t.Errorf("esperava ErrReferenceMismatch, recebeu %v", err)
		}
	})

	t.Run("caso de borda: tentativa de REFUND referenciando WIN retorna ErrReferenceMismatch", func(t *testing.T) {
		ctx, _, _, _, wagerRepo, _, _, useCase, wallet := setupWagerTest(t, 10000)

		// Cria WIN de 25.00
		winMoney, _ := domain.NewMoney(2500, "BRL")
		winTx, _ := domain.NewWagerTransaction(
			"tx-win-1", "ext-win-1", "provider-a", "prov1:ext-win-1", "hash-1",
			wallet.ID(), wallet.PlayerID(), "fortune-tiger", "round-100",
			domain.TransactionTypeWin, winMoney, "",
		)
		_ = winTx.MarkProcessed()
		_ = wagerRepo.Create(ctx, nil, winTx)

		input := application.ProcessWagerInput{
			IdempotencyKey:                 "prov1:refund-win-err",
			ProviderID:                     "provider-a",
			ExternalTransactionID:          "refund-win-err",
			PlayerID:                       wallet.PlayerID(),
			WalletID:                       wallet.ID(),
			GameID:                         "fortune-tiger",
			RoundID:                        "round-100",
			Kind:                           "REFUND",
			Amount:                         "25.00",
			Currency:                       "BRL",
			ReferenceExternalTransactionID: "ext-win-1",
		}

		_, err := useCase.Execute(ctx, input)
		if !errors.Is(err, application.ErrReferenceMismatch) {
			t.Errorf("esperava ErrReferenceMismatch para REFUND sobre WIN, recebeu %v", err)
		}
	})
}

// --- 5. TESTES DE ROLLBACK ---

func TestProcessWagerUseCase_Rollback(t *testing.T) {
	t.Run("sucesso: ROLLBACK de BET desfaz aposta e credita a carteira", func(t *testing.T) {
		ctx, _, walletRepo, ledgerRepo, wagerRepo, outboxRepo, _, useCase, wallet := setupWagerTest(t, 7000) // 70.00 BRL

		betMoney, _ := domain.NewMoney(3000, "BRL")
		betTx, _ := domain.NewWagerTransaction(
			"tx-bet-rb", "ext-bet-rb", "provider-a", "prov1:ext-bet-rb", "hash-rb",
			wallet.ID(), wallet.PlayerID(), "fortune-tiger", "round-200",
			domain.TransactionTypeBet, betMoney, "",
		)
		_ = betTx.MarkProcessed()
		_ = wagerRepo.Create(ctx, nil, betTx)

		input := application.ProcessWagerInput{
			IdempotencyKey:                 "prov1:rb-bet",
			ProviderID:                     "provider-a",
			ExternalTransactionID:          "rb-bet",
			PlayerID:                       wallet.PlayerID(),
			WalletID:                       wallet.ID(),
			GameID:                         "fortune-tiger",
			RoundID:                        "round-200",
			Kind:                           "ROLLBACK",
			Amount:                         "30.00",
			Currency:                       "BRL",
			ReferenceExternalTransactionID: "ext-bet-rb",
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava sucesso no rollback de BET, recebeu: %v", err)
		}

		if output.Status != domain.TransactionStatusProcessed {
			t.Errorf("status esperado PROCESSED, recebeu %s", output.Status)
		}
		if output.Balance.Amount != "100.00" {
			t.Errorf("saldo esperado 100.00, recebeu %s", output.Balance.Amount)
		}

		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Balance().String() != "100.00" {
			t.Errorf("saldo esperado 100.00, recebeu %s", updatedWallet.Balance().String())
		}
		if ledgerRepo.createdEntries[0].Direction() != domain.LedgerDirectionCredit {
			t.Errorf("direcao esperada CREDIT, recebeu %s", ledgerRepo.createdEntries[0].Direction())
		}
		if len(outboxRepo.createdMessages) != 2 {
			t.Errorf("esperava 2 mensagens outbox")
		}
	})

	t.Run("sucesso: ROLLBACK de WIN desfaz ganho debitando a carteira com saldo suficiente", func(t *testing.T) {
		ctx, _, walletRepo, ledgerRepo, wagerRepo, outboxRepo, _, useCase, wallet := setupWagerTest(t, 10000) // 100.00 BRL

		winMoney, _ := domain.NewMoney(4000, "BRL")
		winTx, _ := domain.NewWagerTransaction(
			"tx-win-rb", "ext-win-rb", "provider-a", "prov1:ext-win-rb", "hash-win-rb",
			wallet.ID(), wallet.PlayerID(), "fortune-tiger", "round-201",
			domain.TransactionTypeWin, winMoney, "",
		)
		_ = winTx.MarkProcessed()
		_ = wagerRepo.Create(ctx, nil, winTx)

		input := application.ProcessWagerInput{
			IdempotencyKey:                 "prov1:rb-win",
			ProviderID:                     "provider-a",
			ExternalTransactionID:          "rb-win",
			PlayerID:                       wallet.PlayerID(),
			WalletID:                       wallet.ID(),
			GameID:                         "fortune-tiger",
			RoundID:                        "round-201",
			Kind:                           "ROLLBACK",
			Amount:                         "40.00",
			Currency:                       "BRL",
			ReferenceExternalTransactionID: "ext-win-rb",
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava sucesso no rollback de WIN, recebeu: %v", err)
		}

		if output.Status != domain.TransactionStatusProcessed {
			t.Errorf("status esperado PROCESSED, recebeu %s", output.Status)
		}
		if output.Balance.Amount != "60.00" {
			t.Errorf("saldo esperado 60.00, recebeu %s", output.Balance.Amount)
		}

		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Balance().String() != "60.00" {
			t.Errorf("saldo da carteira esperado 60.00, recebeu %s", updatedWallet.Balance().String())
		}
		if ledgerRepo.createdEntries[0].Direction() != domain.LedgerDirectionDebit {
			t.Errorf("direcao esperada DEBIT, recebeu %s", ledgerRepo.createdEntries[0].Direction())
		}
		if len(outboxRepo.createdMessages) != 2 {
			t.Errorf("esperava 2 mensagens outbox")
		}
	})

	t.Run("caso de borda: ROLLBACK de WIN quando carteira nao possui saldo suficiente para estorno grava REJECTED auditavel", func(t *testing.T) {
		ctx, _, walletRepo, ledgerRepo, wagerRepo, outboxRepo, _, useCase, wallet := setupWagerTest(t, 2000) // Saldo atual: 20.00 BRL

		// O jogador ganhou 100.00 BRL no passado, mas já gastou 80.00 e sobrou só 20.00!
		winMoney, _ := domain.NewMoney(10000, "BRL")
		winTx, _ := domain.NewWagerTransaction(
			"tx-win-spent", "ext-win-spent", "provider-a", "prov1:ext-win-spent", "hash-spent",
			wallet.ID(), wallet.PlayerID(), "fortune-tiger", "round-202",
			domain.TransactionTypeWin, winMoney, "",
		)
		_ = winTx.MarkProcessed()
		_ = wagerRepo.Create(ctx, nil, winTx)

		input := application.ProcessWagerInput{
			IdempotencyKey:                 "prov1:rb-win-spent",
			ProviderID:                     "provider-a",
			ExternalTransactionID:          "rb-win-spent",
			PlayerID:                       wallet.PlayerID(),
			WalletID:                       wallet.ID(),
			GameID:                         "fortune-tiger",
			RoundID:                        "round-202",
			Kind:                           "ROLLBACK",
			Amount:                         "100.00", // Precisa debitar 100.00, mas carteira tem apenas 20.00!
			Currency:                       "BRL",
			ReferenceExternalTransactionID: "ext-win-spent",
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava rejeicao controlada sem crash, recebeu erro: %v", err)
		}

		if output.Status != domain.TransactionStatusRejected {
			t.Errorf("status esperado REJECTED, recebeu %s", output.Status)
		}
		if output.FailureCode != "INSUFFICIENT_FUNDS_FOR_ROLLBACK" {
			t.Errorf("failureCode esperado INSUFFICIENT_FUNDS_FOR_ROLLBACK, recebeu %s", output.FailureCode)
		}
		if output.Balance.Amount != "20.00" {
			t.Errorf("saldo da carteira deve permanecer 20.00, recebeu %s", output.Balance.Amount)
		}

		// Carteira inalterada
		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Balance().String() != "20.00" {
			t.Errorf("saldo da carteira nao deveria ter mudado")
		}
		if len(ledgerRepo.createdEntries) != 0 {
			t.Errorf("ledger nao deve ser criado para rejeicao")
		}

		// Transação persistida como REJECTED
		savedTx, _ := wagerRepo.GetByIdempotencyKey(ctx, nil, "provider-a", "prov1:rb-win-spent")
		if savedTx == nil || savedTx.Status() != domain.TransactionStatusRejected {
			t.Fatalf("transacao deveria estar persistida como REJECTED")
		}
		if savedTx.FailureCode() != "INSUFFICIENT_FUNDS_FOR_ROLLBACK" {
			t.Errorf("failureCode no banco esperado INSUFFICIENT_FUNDS_FOR_ROLLBACK, recebeu %s", savedTx.FailureCode())
		}

		// Evento outbox WagerTransactionRejected
		if len(outboxRepo.createdMessages) != 1 {
			t.Fatalf("esperava 1 mensagem outbox, recebeu %d", len(outboxRepo.createdMessages))
		}
		if outboxRepo.createdMessages[0].EventType != application.EventTypeWagerTransactionRejected {
			t.Errorf("evento esperado WagerTransactionRejected, recebeu %s", outboxRepo.createdMessages[0].EventType)
		}
	})
}

// --- 6. TESTES DE IDEMPOTÊNCIA E CONFLITOS ---

func TestProcessWagerUseCase_Idempotency(t *testing.T) {
	t.Run("sucesso: replay com mesma chave e mesmo payload devolve resultado persistido com idempotentReplay true", func(t *testing.T) {
		ctx, _, walletRepo, ledgerRepo, _, _, _, useCase, wallet := setupWagerTest(t, 10000)

		input := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:idem-1",
			ProviderID:            "provider-a",
			ExternalTransactionID: "idem-1",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-300",
			Kind:                  "BET",
			Amount:                "25.00",
			Currency:              "BRL",
		}

		// 1ª execução
		firstOutput, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("erro na primeira execucao: %v", err)
		}
		if firstOutput.IdempotentReplay {
			t.Errorf("primeira execucao nao deve ter idempotentReplay=true")
		}
		if firstOutput.Balance.Amount != "75.00" {
			t.Errorf("saldo esperado 75.00, recebeu %s", firstOutput.Balance.Amount)
		}

		// 2ª execução (Replay idêntico)
		secondOutput, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("erro no replay: %v", err)
		}
		if !secondOutput.IdempotentReplay {
			t.Errorf("replay deve ter idempotentReplay=true")
		}
		if secondOutput.TransactionID != firstOutput.TransactionID {
			t.Errorf("transactionId deve ser identico no replay: %s != %s", secondOutput.TransactionID, firstOutput.TransactionID)
		}
		if secondOutput.Balance.Amount != "75.00" {
			t.Errorf("saldo no replay deve ser 75.00, recebeu %s", secondOutput.Balance.Amount)
		}

		// Carteira continua com saldo 75.00 e apenas 1 lançamento no ledger
		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Balance().String() != "75.00" {
			t.Errorf("carteira nao pode ter sido debitada duas vezes")
		}
		if len(ledgerRepo.createdEntries) != 1 {
			t.Errorf("esperava apenas 1 lancamento no ledger")
		}
	})

	t.Run("garantia de saldo observado: replay devolve saldo original da operacao mesmo se carteira recebeu outras movimentacoes posteriores", func(t *testing.T) {
		ctx, _, walletRepo, _, _, _, _, useCase, wallet := setupWagerTest(t, 10000) // 100.00 BRL

		// 1ª operação: Aposta de 25.00 -> Saldo observado = 75.00
		inputBet := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:bet-historic",
			ProviderID:            "provider-a",
			ExternalTransactionID: "bet-historic",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-hist-1",
			Kind:                  "BET",
			Amount:                "25.00",
			Currency:              "BRL",
		}
		_, err := useCase.Execute(ctx, inputBet)
		if err != nil {
			t.Fatalf("erro na aposta: %v", err)
		}

		// 2ª operação subsequente: Ganho gigante de 500.00 -> Saldo atual da carteira vai para 575.00
		inputWin := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:win-subsequent",
			ProviderID:            "provider-a",
			ExternalTransactionID: "win-subsequent",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-hist-2",
			Kind:                  "WIN",
			Amount:                "500.00",
			Currency:              "BRL",
		}
		_, err = useCase.Execute(ctx, inputWin)
		if err != nil {
			t.Fatalf("erro no ganho subsequente: %v", err)
		}

		currentWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if currentWallet.Balance().String() != "575.00" {
			t.Fatalf("saldo atual esperado 575.00, recebeu %s", currentWallet.Balance().String())
		}

		// Replay da 1ª operação (aposta): DEVE retornar o saldo observado no momento original (75.00), e NÃO o saldo atual (575.00)!
		replayOutput, err := useCase.Execute(ctx, inputBet)
		if err != nil {
			t.Fatalf("erro no replay: %v", err)
		}
		if !replayOutput.IdempotentReplay {
			t.Errorf("esperava idempotentReplay=true")
		}
		if replayOutput.Balance.Amount != "75.00" {
			t.Errorf("requisito formal violado: replay deve devolver o saldo observado no momento original (75.00), mas devolveu %s", replayOutput.Balance.Amount)
		}
	})

	t.Run("conflito 409: mesma chave reutilizada com payload diferente retorna ErrIdempotencyConflict", func(t *testing.T) {
		ctx, _, _, _, _, _, _, useCase, wallet := setupWagerTest(t, 10000)

		input1 := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:same-key",
			ProviderID:            "provider-a",
			ExternalTransactionID: "tx-1",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-1",
			Kind:                  "BET",
			Amount:                "25.00",
			Currency:              "BRL",
		}
		_, err := useCase.Execute(ctx, input1)
		if err != nil {
			t.Fatalf("erro na 1a execucao: %v", err)
		}

		// Mesma chave de idempotência com payload adulterado (valor 50.00)
		input2 := input1
		input2.Amount = "50.00"

		_, err = useCase.Execute(ctx, input2)
		if !errors.Is(err, application.ErrIdempotencyConflict) {
			t.Errorf("esperava ErrIdempotencyConflict, recebeu: %v", err)
		}
	})

	t.Run("conflito 409: mesmo (providerId, externalTransactionId) com outra idempotencyKey retorna ErrExternalTransactionConflict", func(t *testing.T) {
		ctx, _, _, _, _, _, _, useCase, wallet := setupWagerTest(t, 10000)

		input1 := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:key-A",
			ProviderID:            "provider-a",
			ExternalTransactionID: "ext-tx-fixed",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-1",
			Kind:                  "BET",
			Amount:                "25.00",
			Currency:              "BRL",
		}
		_, err := useCase.Execute(ctx, input1)
		if err != nil {
			t.Fatalf("erro na 1a execucao: %v", err)
		}

		// Mesma externalTransactionID porém enviada com chave diferente
		input2 := input1
		input2.IdempotencyKey = "prov1:key-B"

		_, err = useCase.Execute(ctx, input2)
		if !errors.Is(err, application.ErrExternalTransactionConflict) {
			t.Errorf("esperava ErrExternalTransactionConflict, recebeu: %v", err)
		}
	})
}

// --- 7. TESTES DE INBOX SQS ---

func TestProcessWagerUseCase_SQSInbox(t *testing.T) {
	t.Run("sucesso: primeira entrega via SQS conclui mensagem na inbox", func(t *testing.T) {
		ctx, _, _, _, _, _, inboxRepo, useCase, wallet := setupWagerTest(t, 10000)

		input := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:sqs-bet-1",
			ProviderID:            "provider-a",
			ExternalTransactionID: "sqs-bet-1",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-sqs-1",
			Kind:                  "BET",
			Amount:                "20.00",
			Currency:              "BRL",
			ConsumerName:          "wager-sqs-consumer",
			MessageID:             "msg-uuid-1",
		}

		output, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("esperava sucesso, recebeu %v", err)
		}
		if output.Status != domain.TransactionStatusProcessed {
			t.Errorf("status esperado PROCESSED, recebeu %s", output.Status)
		}

		// Valida que a inbox registrou e marcou como processada
		if !inboxRepo.processed["wager-sqs-consumer:msg-uuid-1"] {
			t.Errorf("mensagem na inbox deveria estar marcada como processada")
		}
	})

	t.Run("reentrega SQS: deduplicacao na inbox nao debita carteira novamente", func(t *testing.T) {
		ctx, _, walletRepo, ledgerRepo, _, _, _, useCase, wallet := setupWagerTest(t, 10000)

		input := application.ProcessWagerInput{
			IdempotencyKey:        "prov1:sqs-dup-1",
			ProviderID:            "provider-a",
			ExternalTransactionID: "sqs-dup-1",
			PlayerID:              wallet.PlayerID(),
			WalletID:              wallet.ID(),
			GameID:                "fortune-tiger",
			RoundID:               "round-sqs-2",
			Kind:                  "BET",
			Amount:                "20.00",
			Currency:              "BRL",
			ConsumerName:          "wager-sqs-consumer",
			MessageID:             "msg-dup-123",
		}

		// 1ª entrega
		_, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("erro na 1a entrega: %v", err)
		}

		// 2ª entrega (reentrega da mesma mensagem SQS pelo broker)
		output2, err := useCase.Execute(ctx, input)
		if err != nil {
			t.Fatalf("erro na reentrega: %v", err)
		}

		if output2.Balance.Amount != "80.00" {
			t.Errorf("saldo esperado 80.00, recebeu %s", output2.Balance.Amount)
		}

		// A carteira foi debitada apenas UMA vez
		updatedWallet, _ := walletRepo.GetByID(ctx, nil, wallet.ID(), false)
		if updatedWallet.Balance().String() != "80.00" {
			t.Errorf("saldo da carteira deve ser 80.00, recebeu %s", updatedWallet.Balance().String())
		}
		if len(ledgerRepo.createdEntries) != 1 {
			t.Errorf("deve haver apenas 1 lançamento no ledger, recebeu %d", len(ledgerRepo.createdEntries))
		}
	})
}

// --- 8. CASOS DE BORDA E VALIDAÇÕES PRELIMINARES ---

func TestProcessWagerUseCase_ValidationsAndEdgeCases(t *testing.T) {
	ctx, _, _, _, _, _, _, useCase, wallet := setupWagerTest(t, 10000)

	baseValidInput := application.ProcessWagerInput{
		IdempotencyKey:        "prov1:valid-1",
		ProviderID:            "provider-a",
		ExternalTransactionID: "tx-valid-1",
		PlayerID:              wallet.PlayerID(),
		WalletID:              wallet.ID(),
		GameID:                "fortune-tiger",
		RoundID:               "round-1",
		Kind:                  "BET",
		Amount:                "25.00",
		Currency:              "BRL",
	}

	t.Run("caso de borda: rejeita tipo OPENING via HTTP ou SQS", func(t *testing.T) {
		input := baseValidInput
		input.Kind = domain.TransactionTypeOpening

		_, err := useCase.Execute(ctx, input)
		if !errors.Is(err, application.ErrOpeningTransactionForbidden) {
			t.Errorf("esperava ErrOpeningTransactionForbidden, recebeu %v", err)
		}
	})

	t.Run("caso de borda: validacao de campos obrigatorios", func(t *testing.T) {
		tests := []struct {
			name        string
			modifier    func(in *application.ProcessWagerInput)
			expectedErr error
		}{
			{
				name:        "IdempotencyKey vazia",
				modifier:    func(in *application.ProcessWagerInput) { in.IdempotencyKey = "" },
				expectedErr: domain.ErrInvalidIdempotencyKey,
			},
			{
				name:        "ProviderID vazio",
				modifier:    func(in *application.ProcessWagerInput) { in.ProviderID = "" },
				expectedErr: domain.ErrInvalidProviderId,
			},
			{
				name:        "ExternalTransactionID vazio",
				modifier:    func(in *application.ProcessWagerInput) { in.ExternalTransactionID = "" },
				expectedErr: domain.ErrInvalidExternalTransactionId,
			},
			{
				name:        "PlayerID vazio",
				modifier:    func(in *application.ProcessWagerInput) { in.PlayerID = "" },
				expectedErr: domain.ErrInvalidPlayerId,
			},
			{
				name:        "WalletID vazio",
				modifier:    func(in *application.ProcessWagerInput) { in.WalletID = "" },
				expectedErr: domain.ErrInvalidWalletId,
			},
			{
				name:        "GameID vazio",
				modifier:    func(in *application.ProcessWagerInput) { in.GameID = "" },
				expectedErr: domain.ErrInvalidGameId,
			},
			{
				name:        "RoundID vazio",
				modifier:    func(in *application.ProcessWagerInput) { in.RoundID = "" },
				expectedErr: domain.ErrInvalidRoundId,
			},
			{
				name:        "Kind invalido",
				modifier:    func(in *application.ProcessWagerInput) { in.Kind = "DEPOSIT" },
				expectedErr: domain.ErrInvalidTransactionType,
			},
			{
				name: "REFUND sem ReferenceExternalTransactionID",
				modifier: func(in *application.ProcessWagerInput) {
					in.Kind = "REFUND"
					in.ReferenceExternalTransactionID = ""
				},
				expectedErr: domain.ErrInvalidReferenceExternalTransactionId,
			},
			{
				name: "ROLLBACK sem ReferenceExternalTransactionID",
				modifier: func(in *application.ProcessWagerInput) {
					in.Kind = "ROLLBACK"
					in.ReferenceExternalTransactionID = ""
				},
				expectedErr: domain.ErrInvalidReferenceExternalTransactionId,
			},
			{
				name: "Amount negativo",
				modifier: func(in *application.ProcessWagerInput) {
					in.Amount = "-10.00"
				},
				expectedErr: domain.ErrNegativeAmount,
			},
			{
				name: "Amount zero para BET",
				modifier: func(in *application.ProcessWagerInput) {
					in.Amount = "0.00"
				},
				expectedErr: domain.ErrNegativeAmount,
			},
			{
				name: "Currency invalida",
				modifier: func(in *application.ProcessWagerInput) {
					in.Currency = "REAL"
				},
				expectedErr: domain.ErrInvalidCurrency,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				in := baseValidInput
				tc.modifier(&in)

				_, err := useCase.Execute(ctx, in)
				if !errors.Is(err, tc.expectedErr) {
					t.Errorf("esperava erro %v, recebeu %v", tc.expectedErr, err)
				}
			})
		}
	})

	t.Run("caso de borda: carteira nao encontrada retorna ErrWalletNotFound", func(t *testing.T) {
		input := baseValidInput
		input.WalletID = "wallet-fantasma"

		_, err := useCase.Execute(ctx, input)
		if !errors.Is(err, application.ErrWalletNotFound) {
			t.Errorf("esperava ErrWalletNotFound, recebeu %v", err)
		}
	})

	t.Run("caso de borda: PlayerID incompativel com a carteira retorna ErrPlayerWalletMismatch", func(t *testing.T) {
		input := baseValidInput
		input.PlayerID = "player-outro-ladrao"

		_, err := useCase.Execute(ctx, input)
		if !errors.Is(err, application.ErrPlayerWalletMismatch) {
			t.Errorf("esperava ErrPlayerWalletMismatch, recebeu %v", err)
		}
	})

	t.Run("caso de borda: moeda da operacao incompativel com a carteira retorna ErrCurrencyMismatch", func(t *testing.T) {
		input := baseValidInput
		input.Currency = "USD" // Carteira é BRL

		_, err := useCase.Execute(ctx, input)
		if !errors.Is(err, application.ErrCurrencyMismatch) {
			t.Errorf("esperava ErrCurrencyMismatch, recebeu %v", err)
		}
	})
}
