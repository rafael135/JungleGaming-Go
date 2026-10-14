package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"jungle/internal/domain"
	"jungle/internal/infrastructure/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://jungle:jungle_secret@localhost:5432/jungle_db?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, connStr)
	if err != nil {
		t.Fatalf("falha ao conectar no postgres de teste: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

func TestWalletRepository_LiveDB(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := database.NewWalletRepository()

	playerID := uuid.NewString()
	walletID := uuid.NewString()
	initialMoney, err := domain.NewMoney(10000, "BRL")
	if err != nil {
		t.Fatalf("erro ao criar money: %v", err)
	}

	wallet, err := domain.NewWallet(walletID, playerID, initialMoney)
	if err != nil {
		t.Fatalf("erro ao criar wallet: %v", err)
	}

	// 1. Inserir carteira
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro ao iniciar tx: %v", err)
	}
	defer tx.Rollback(ctx)

	err = repo.Create(ctx, tx, wallet)
	if err != nil {
		t.Fatalf("esperava sucesso ao criar carteira, recebeu: %v", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatalf("erro no commit: %v", err)
	}

	// 2. Buscar carteira sem lock
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro ao iniciar tx2: %v", err)
	}
	defer tx2.Rollback(ctx)

	found, err := repo.GetByID(ctx, tx2, walletID, false)
	if err != nil {
		t.Fatalf("esperava encontrar carteira, recebeu: %v", err)
	}
	if found.ID() != walletID {
		t.Errorf("esperava id %s, recebeu %s", walletID, found.ID())
	}
	if found.Balance().Cents() != 10000 {
		t.Errorf("esperava saldo 10000, recebeu %d", found.Balance().Cents())
	}

	// 3. Buscar com FOR UPDATE e Atualizar saldo
	tx3, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro ao iniciar tx3: %v", err)
	}
	defer tx3.Rollback(ctx)

	lockedWallet, err := repo.GetByID(ctx, tx3, walletID, true)
	if err != nil {
		t.Fatalf("erro ao buscar com lock: %v", err)
	}

	debitMoney, _ := domain.NewMoney(2000, "BRL")
	err = lockedWallet.Debit(debitMoney)
	if err != nil {
		t.Fatalf("erro ao debitar da carteira: %v", err)
	}

	err = repo.Update(ctx, tx3, lockedWallet)
	if err != nil {
		t.Fatalf("erro ao atualizar carteira: %v", err)
	}

	err = tx3.Commit(ctx)
	if err != nil {
		t.Fatalf("erro no commit tx3: %v", err)
	}

	// 4. Conferir saldo e versão atualizados
	tx4, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro ao iniciar tx4: %v", err)
	}
	defer tx4.Rollback(ctx)

	updatedWallet, err := repo.GetByID(ctx, tx4, walletID, false)
	if err != nil {
		t.Fatalf("erro ao buscar carteira atualizada: %v", err)
	}
	if updatedWallet.Balance().Cents() != 8000 {
		t.Errorf("esperava saldo 8000 cents, recebeu %d", updatedWallet.Balance().Cents())
	}
	if updatedWallet.Version() != 2 {
		t.Errorf("esperava versao 2, recebeu %d", updatedWallet.Version())
	}
}

func TestWalletLedgerEntryRepository_LiveDB(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	walletRepo := database.NewWalletRepository()
	ledgerRepo := database.NewWalletLedgerEntryRepository()

	// Criar carteira base
	walletID := uuid.NewString()
	playerID := uuid.NewString()
	initialMoney, _ := domain.NewMoney(5000, "BRL")
	wallet, _ := domain.NewWallet(walletID, playerID, initialMoney)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := walletRepo.Create(ctx, tx, wallet); err != nil {
		t.Fatalf("erro ao criar carteira: %v", err)
	}

	// Criar entrada de ledger
	txID := uuid.NewString()
	ledgerID := uuid.NewString()
	amount, _ := domain.NewMoney(2000, "BRL")
	balBefore, _ := domain.NewMoney(5000, "BRL")
	balAfter, _ := domain.NewMoney(3000, "BRL")

	entry, err := domain.NewWalletLedgerEntry(ledgerID, walletID, txID, domain.LedgerDirectionDebit, amount, balBefore, balAfter)
	if err != nil {
		t.Fatalf("erro ao criar entrada ledger de dominio: %v", err)
	}

	err = ledgerRepo.Create(ctx, tx, entry)
	if err != nil {
		t.Fatalf("erro ao persistir ledger: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("erro commit: %v", err)
	}

	// 1. Buscar por transaction_id
	txRead, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro tx: %v", err)
	}
	defer txRead.Rollback(ctx)

	foundEntry, err := ledgerRepo.GetByTransactionID(ctx, txRead, txID)
	if err != nil {
		t.Fatalf("erro ao buscar ledger por txID: %v", err)
	}
	if foundEntry.ID() != ledgerID {
		t.Errorf("esperava ledger ID %s, recebeu %s", ledgerID, foundEntry.ID())
	}
	if foundEntry.Amount().Cents() != 2000 {
		t.Errorf("esperava valor 2000, recebeu %d", foundEntry.Amount().Cents())
	}

	// 2. Listar entradas da carteira
	entries, err := ledgerRepo.ListByWalletID(ctx, txRead, walletID)
	if err != nil {
		t.Fatalf("erro ao listar ledger da carteira: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("esperava 1 entrada, recebeu %d", len(entries))
	}

	// 3. Verificação obrigatória: Trigger impede UPDATE e DELETE
	_, err = txRead.Exec(ctx, "UPDATE wallet_ledger_entries SET amount_cents = 9999 WHERE id = $1", ledgerID)
	if err == nil {
		t.Fatalf("FALHA CRÍTICA DE SEGURANÇA: trigger permitiu UPDATE em wallet_ledger_entries!")
	}

	_, err = txRead.Exec(ctx, "DELETE FROM wallet_ledger_entries WHERE id = $1", ledgerID)
	if err == nil {
		t.Fatalf("FALHA CRÍTICA DE SEGURANÇA: trigger permitiu DELETE em wallet_ledger_entries!")
	}
}

func TestWagerTransactionRepository_LiveDB(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	walletRepo := database.NewWalletRepository()
	txRepo := database.NewWagerTransactionRepository()

	walletID := uuid.NewString()
	playerID := uuid.NewString()
	initialMoney, _ := domain.NewMoney(10000, "BRL")
	wallet, _ := domain.NewWallet(walletID, playerID, initialMoney)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := walletRepo.Create(ctx, tx, wallet); err != nil {
		t.Fatalf("erro carteira: %v", err)
	}

	// 1. Criar transacao de aposta BET
	txID := uuid.NewString()
	extID := "ext-" + uuid.NewString()
	providerID := "provider-jungle"
	idempotencyKey := "idem-" + uuid.NewString()
	payloadHash := "hash-123456"
	betMoney, _ := domain.NewMoney(2500, "BRL")

	wager, err := domain.NewWagerTransaction(
		txID,
		extID,
		providerID,
		idempotencyKey,
		payloadHash,
		walletID,
		playerID,
		"game-safari",
		"round-001",
		domain.TransactionTypeBet,
		betMoney,
		"",
	)
	if err != nil {
		t.Fatalf("erro ao criar wager: %v", err)
	}

	err = txRepo.Create(ctx, tx, wager)
	if err != nil {
		t.Fatalf("erro ao inserir wager: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("erro commit: %v", err)
	}

	// 2. Buscar por ID, IdempotencyKey e ExternalID
	txRead, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro tx: %v", err)
	}
	defer txRead.Rollback(ctx)

	byID, err := txRepo.GetByID(ctx, txRead, txID)
	if err != nil {
		t.Fatalf("erro GetByID: %v", err)
	}
	if byID.ID() != txID {
		t.Errorf("esperava id %s, recebeu %s", txID, byID.ID())
	}

	byIdem, err := txRepo.GetByIdempotencyKey(ctx, txRead, providerID, idempotencyKey)
	if err != nil {
		t.Fatalf("erro GetByIdempotencyKey: %v", err)
	}
	if byIdem.ID() != txID {
		t.Errorf("esperava mesmo id na busca por chave de idempotencia")
	}

	byExt, err := txRepo.GetByExternalID(ctx, txRead, providerID, extID)
	if err != nil {
		t.Fatalf("erro GetByExternalID: %v", err)
	}
	if byExt.ID() != txID {
		t.Errorf("esperava mesmo id na busca por external id")
	}

	// 3. Atualizar status para PROCESSED
	err = byID.MarkProcessed()
	if err != nil {
		t.Fatalf("erro ao marcar processed: %v", err)
	}

	err = txRepo.Update(ctx, txRead, byID)
	if err != nil {
		t.Fatalf("erro ao atualizar wager: %v", err)
	}

	if err := txRead.Commit(ctx); err != nil {
		t.Fatalf("erro commit: %v", err)
	}

	// 4. Conferir status atualizado
	txCheck, _ := pool.Begin(ctx)
	defer txCheck.Rollback(ctx)
	reloaded, err := txRepo.GetByID(ctx, txCheck, txID)
	if err != nil {
		t.Fatalf("erro reload: %v", err)
	}
	if reloaded.Status() != domain.TransactionStatusProcessed {
		t.Errorf("esperava PROCESSED, recebeu %s", reloaded.Status())
	}

	// 5. Testar transação do tipo OPENING (campos nulos no banco)
	openingID := uuid.NewString()
	openingWalletID := uuid.NewString()
	openingPlayerID := uuid.NewString()
	openingMoney, _ := domain.NewMoney(10000, "BRL")

	openingTx, err := domain.NewOpeningTransaction(openingID, openingWalletID, openingPlayerID, openingMoney)
	if err != nil {
		t.Fatalf("erro ao criar opening tx: %v", err)
	}

	txOpening, _ := pool.Begin(ctx)
	defer txOpening.Rollback(ctx)

	// Inserir carteira correspondente antes para respeitar a FK
	newWallet, _ := domain.NewWallet(openingWalletID, openingPlayerID, openingMoney)
	_ = walletRepo.Create(ctx, txOpening, newWallet)

	err = txRepo.Create(ctx, txOpening, openingTx)
	if err != nil {
		t.Fatalf("esperava sucesso ao inserir OPENING com campos opcionais nulos, recebeu: %v", err)
	}

	openingReloaded, err := txRepo.GetByID(ctx, txOpening, openingID)
	if err != nil {
		t.Fatalf("erro ao buscar OPENING: %v", err)
	}
	if openingReloaded.TransactionType() != domain.TransactionTypeOpening {
		t.Errorf("esperava tipo OPENING, recebeu %s", openingReloaded.TransactionType())
	}
}

func TestInboxRepository_LiveDB(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	inboxRepo := database.NewInboxRepository()

	consumerName := "sqs-wager-consumer"
	messageID := "msg-" + uuid.NewString()
	payloadHash := "hash-" + uuid.NewString()

	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro tx: %v", err)
	}
	defer tx1.Rollback(ctx)

	// 1. Primeira tentativa de registrar: deve retornar true (inédito)
	inserted, err := inboxRepo.TryRecord(ctx, tx1, consumerName, messageID, payloadHash)
	if err != nil {
		t.Fatalf("erro TryRecord: %v", err)
	}
	if !inserted {
		t.Fatalf("esperava inserted = true na primeira tentativa")
	}

	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("erro commit: %v", err)
	}

	// 2. Segunda tentativa de registrar a mesma mensagem: deve retornar false (duplicada)
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro tx2: %v", err)
	}
	defer tx2.Rollback(ctx)

	duplicate, err := inboxRepo.TryRecord(ctx, tx2, consumerName, messageID, payloadHash)
	if err != nil {
		t.Fatalf("erro TryRecord duplicado: %v", err)
	}
	if duplicate {
		t.Fatalf("esperava inserted = false na segunda tentativa (mensagem duplicada)")
	}

	// 3. Marcar como processada
	err = inboxRepo.MarkProcessed(ctx, tx2, consumerName, messageID)
	if err != nil {
		t.Fatalf("erro MarkProcessed: %v", err)
	}

	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("erro commit: %v", err)
	}

	// 4. Conferir registro com ProcessedAt preenchido
	tx3, _ := pool.Begin(ctx)
	defer tx3.Rollback(ctx)

	msg, err := inboxRepo.Get(ctx, tx3, consumerName, messageID)
	if err != nil {
		t.Fatalf("erro ao buscar mensagem: %v", err)
	}
	if msg.ProcessedAt == nil {
		t.Errorf("esperava ProcessedAt preenchido apos MarkProcessed")
	}
}

func TestOutboxRepository_LiveDB(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	outboxRepo := database.NewOutboxRepository()

	msgID := uuid.NewString()
	aggregateID := uuid.NewString()

	outboxMsg := &database.OutboxMessage{
		ID:            msgID,
		AggregateType: "Wallet",
		AggregateID:   aggregateID,
		EventType:     "WalletBalanceChanged",
		Payload:       []byte(`{"balance":"80.00","currency":"BRL"}`),
		Status:        database.OutboxStatusPending,
		RetryCount:    0,
		NextAttemptAt: time.Now().UTC().Add(-1 * time.Minute), // Pronto para envio imediato
		CreatedAt:     time.Now().UTC(),
	}

	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro tx: %v", err)
	}
	defer tx1.Rollback(ctx)

	err = outboxRepo.Create(ctx, tx1, outboxMsg)
	if err != nil {
		t.Fatalf("erro Create outbox: %v", err)
	}

	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("erro commit: %v", err)
	}

	// 1. FetchPending com FOR UPDATE SKIP LOCKED
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("erro tx2: %v", err)
	}
	defer tx2.Rollback(ctx)

	pending, err := outboxRepo.FetchPending(ctx, tx2, 10)
	if err != nil {
		t.Fatalf("erro FetchPending: %v", err)
	}

	var found *database.OutboxMessage
	for _, p := range pending {
		if p.ID == msgID {
			found = p
			break
		}
	}
	if found == nil {
		t.Fatalf("mensagem %s deveria estar entre as pendentes", msgID)
	}

	// 2. Marcar como publicada
	err = outboxRepo.MarkPublished(ctx, tx2, msgID)
	if err != nil {
		t.Fatalf("erro MarkPublished: %v", err)
	}

	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("erro commit: %v", err)
	}

	// 3. Testar ScheduleRetry com outra mensagem
	retryMsgID := uuid.NewString()
	retryMsg := &database.OutboxMessage{
		ID:            retryMsgID,
		AggregateType: "WagerTransaction",
		AggregateID:   uuid.NewString(),
		EventType:     "WagerTransactionProcessed",
		Payload:       []byte(`{"status":"PROCESSED"}`),
		Status:        database.OutboxStatusPending,
		NextAttemptAt: time.Now().UTC().Add(-10 * time.Second),
		CreatedAt:     time.Now().UTC(),
	}

	tx3, _ := pool.Begin(ctx)
	defer tx3.Rollback(ctx)
	_ = outboxRepo.Create(ctx, tx3, retryMsg)
	_ = tx3.Commit(ctx)

	tx4, _ := pool.Begin(ctx)
	defer tx4.Rollback(ctx)

	nextAttempt := time.Now().UTC().Add(5 * time.Minute)
	err = outboxRepo.ScheduleRetry(ctx, tx4, retryMsgID, nextAttempt)
	if err != nil {
		t.Fatalf("erro ScheduleRetry: %v", err)
	}
	_ = tx4.Commit(ctx)
}
