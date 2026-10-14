package application_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"jungle/internal/domain"
	"jungle/internal/infrastructure/database"

	"github.com/jackc/pgx/v5"
)

// --- FAKE TRANSACTION MANAGER ---

type fakeTxManager struct {
	executeCalled bool
	failErr       error
}

func (f *fakeTxManager) Execute(ctx context.Context, fn func(tx pgx.Tx) error) error {
	f.executeCalled = true
	if f.failErr != nil {
		return f.failErr
	}
	return fn(nil)
}

// --- FAKE WALLET REPOSITORY ---

type fakeWalletRepository struct {
	mu            sync.Mutex
	wallets       map[string]*domain.Wallet
	createdWallet *domain.Wallet
	createErr     error
	getErr        error
	updateErr     error
}

func newFakeWalletRepository() *fakeWalletRepository {
	return &fakeWalletRepository{
		wallets: make(map[string]*domain.Wallet),
	}
}

func (f *fakeWalletRepository) Create(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.createErr != nil {
		return f.createErr
	}
	if f.wallets == nil {
		f.wallets = make(map[string]*domain.Wallet)
	}
	f.wallets[w.ID()] = w
	f.createdWallet = w
	return nil
}

func (f *fakeWalletRepository) GetByID(ctx context.Context, tx pgx.Tx, id string, forUpdate bool) (*domain.Wallet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.getErr != nil {
		return nil, f.getErr
	}
	w, exists := f.wallets[id]
	if !exists {
		return nil, errors.New("wallet not found")
	}
	return w, nil
}

func (f *fakeWalletRepository) Update(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.updateErr != nil {
		return f.updateErr
	}
	if f.wallets == nil {
		f.wallets = make(map[string]*domain.Wallet)
	}
	f.wallets[w.ID()] = w
	return nil
}

// --- FAKE LEDGER REPOSITORY ---

type fakeLedgerRepository struct {
	mu             sync.Mutex
	createdEntries []*domain.WalletLedgerEntry
	byTxID         map[string]*domain.WalletLedgerEntry
	createErr      error
}

func newFakeLedgerRepository() *fakeLedgerRepository {
	return &fakeLedgerRepository{
		byTxID: make(map[string]*domain.WalletLedgerEntry),
	}
}

func (f *fakeLedgerRepository) Create(ctx context.Context, tx pgx.Tx, l *domain.WalletLedgerEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.createErr != nil {
		return f.createErr
	}
	if f.byTxID == nil {
		f.byTxID = make(map[string]*domain.WalletLedgerEntry)
	}
	f.createdEntries = append(f.createdEntries, l)
	f.byTxID[l.TransactionID()] = l
	return nil
}

func (f *fakeLedgerRepository) GetByTransactionID(ctx context.Context, tx pgx.Tx, id string) (*domain.WalletLedgerEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	l, exists := f.byTxID[id]
	if !exists {
		return nil, errors.New("ledger entry not found")
	}
	return l, nil
}

func (f *fakeLedgerRepository) ListByWalletID(ctx context.Context, tx pgx.Tx, walletID string) ([]*domain.WalletLedgerEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var result []*domain.WalletLedgerEntry
	for _, entry := range f.createdEntries {
		if entry.WalletID() == walletID {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (f *fakeLedgerRepository) ListPaginated(ctx context.Context, tx pgx.Tx, walletID string, limit int, cursorCreatedAt *time.Time, cursorID string) ([]*domain.WalletLedgerEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var result []*domain.WalletLedgerEntry
	for _, entry := range f.createdEntries {
		if entry.WalletID() != walletID {
			continue
		}
		if cursorCreatedAt != nil && cursorID != "" {
			if !entry.CreatedAt().After(*cursorCreatedAt) && (entry.CreatedAt().Before(*cursorCreatedAt) || entry.ID() <= cursorID) {
				continue
			}
		}
		result = append(result, entry)
		if limit > 0 && len(result) == limit {
			break
		}
	}
	return result, nil
}

// --- FAKE WAGER REPOSITORY ---

type fakeWagerRepository struct {
	mu        sync.Mutex
	byID      map[string]*domain.WagerTransaction
	byKey     map[string]*domain.WagerTransaction // key: providerID:idempotencyKey
	byExtID   map[string]*domain.WagerTransaction // key: providerID:externalID
	createdTx *domain.WagerTransaction
	createErr error
	updateErr error
}

func newFakeWagerRepository() *fakeWagerRepository {
	return &fakeWagerRepository{
		byID:    make(map[string]*domain.WagerTransaction),
		byKey:   make(map[string]*domain.WagerTransaction),
		byExtID: make(map[string]*domain.WagerTransaction),
	}
}

func (f *fakeWagerRepository) Create(ctx context.Context, tx pgx.Tx, t *domain.WagerTransaction) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.createErr != nil {
		return f.createErr
	}
	if f.byID == nil {
		f.byID = make(map[string]*domain.WagerTransaction)
		f.byKey = make(map[string]*domain.WagerTransaction)
		f.byExtID = make(map[string]*domain.WagerTransaction)
	}

	f.byID[t.ID()] = t
	if t.ProviderID() != "" && t.IdempotencyKey() != "" {
		f.byKey[fmt.Sprintf("%s:%s", t.ProviderID(), t.IdempotencyKey())] = t
	}
	if t.ProviderID() != "" && t.ExternalTransactionID() != "" {
		f.byExtID[fmt.Sprintf("%s:%s", t.ProviderID(), t.ExternalTransactionID())] = t
	}
	f.createdTx = t
	return nil
}

func (f *fakeWagerRepository) Update(ctx context.Context, tx pgx.Tx, t *domain.WagerTransaction) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.updateErr != nil {
		return f.updateErr
	}
	f.byID[t.ID()] = t
	return nil
}

func (f *fakeWagerRepository) GetByID(ctx context.Context, tx pgx.Tx, id string) (*domain.WagerTransaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	t, exists := f.byID[id]
	if !exists {
		return nil, errors.New("transaction not found")
	}
	return t, nil
}

func (f *fakeWagerRepository) GetByIdempotencyKey(ctx context.Context, tx pgx.Tx, providerID string, idempotencyKey string) (*domain.WagerTransaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	k := fmt.Sprintf("%s:%s", providerID, idempotencyKey)
	t, exists := f.byKey[k]
	if !exists {
		return nil, errors.New("transaction not found")
	}
	return t, nil
}

func (f *fakeWagerRepository) GetByExternalID(ctx context.Context, tx pgx.Tx, providerID string, externalID string) (*domain.WagerTransaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	k := fmt.Sprintf("%s:%s", providerID, externalID)
	t, exists := f.byExtID[k]
	if !exists {
		return nil, errors.New("transaction not found")
	}
	return t, nil
}

func (f *fakeWagerRepository) FetchPendingReferences(ctx context.Context, tx pgx.Tx, limit int) ([]*domain.WagerTransaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var result []*domain.WagerTransaction
	for _, t := range f.byID {
		if t.Status() == domain.TransactionStatusPendingReference {
			result = append(result, t)
			if limit > 0 && len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

// --- FAKE OUTBOX REPOSITORY ---

type fakeOutboxRepository struct {
	mu              sync.Mutex
	createdMessages []*database.OutboxMessage
	createErr       error
}

func newFakeOutboxRepository() *fakeOutboxRepository {
	return &fakeOutboxRepository{}
}

func (f *fakeOutboxRepository) Create(ctx context.Context, tx pgx.Tx, msg *database.OutboxMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.createErr != nil {
		return f.createErr
	}
	f.createdMessages = append(f.createdMessages, msg)
	return nil
}

func (f *fakeOutboxRepository) FetchPending(ctx context.Context, tx pgx.Tx, limit int) ([]*database.OutboxMessage, error) {
	return nil, nil
}

func (f *fakeOutboxRepository) MarkPublished(ctx context.Context, tx pgx.Tx, id string) error {
	return nil
}

func (f *fakeOutboxRepository) ScheduleRetry(ctx context.Context, tx pgx.Tx, id string, nextAttemptAt time.Time) error {
	return nil
}

func (f *fakeOutboxRepository) MarkFailed(ctx context.Context, tx pgx.Tx, id string) error {
	return nil
}

// --- FAKE INBOX REPOSITORY ---

type fakeInboxRepository struct {
	mu             sync.Mutex
	messages       map[string]*database.InboxMessage // key: consumerName:messageID
	recordedHashes map[string]string
	processed      map[string]bool
	tryRecordErr   error
}

func newFakeInboxRepository() *fakeInboxRepository {
	return &fakeInboxRepository{
		messages:       make(map[string]*database.InboxMessage),
		recordedHashes: make(map[string]string),
		processed:      make(map[string]bool),
	}
}

func (f *fakeInboxRepository) TryRecord(ctx context.Context, tx pgx.Tx, consumerName string, messageID string, payloadHash string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.tryRecordErr != nil {
		return false, f.tryRecordErr
	}

	key := fmt.Sprintf("%s:%s", consumerName, messageID)
	if _, exists := f.recordedHashes[key]; exists {
		return false, nil // Já existia (duplicada)
	}

	f.recordedHashes[key] = payloadHash
	return true, nil
}

func (f *fakeInboxRepository) Get(ctx context.Context, tx pgx.Tx, consumerName string, messageID string) (*database.InboxMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := fmt.Sprintf("%s:%s", consumerName, messageID)
	msg, exists := f.messages[key]
	if !exists {
		return nil, errors.New("inbox message not found")
	}
	return msg, nil
}

func (f *fakeInboxRepository) MarkProcessed(ctx context.Context, tx pgx.Tx, consumerName string, messageID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := fmt.Sprintf("%s:%s", consumerName, messageID)
	f.processed[key] = true
	return nil
}
