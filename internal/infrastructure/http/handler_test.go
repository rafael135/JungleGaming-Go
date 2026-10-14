package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"jungle/internal/application"
	"jungle/internal/domain"
	"jungle/internal/infrastructure/auth"
	infradb "jungle/internal/infrastructure/database"
	infrahttp "jungle/internal/infrastructure/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// --- MOCK TOKEN VALIDATOR ---

type mockTokenValidator struct {
	claims  *auth.AuthClaims
	failErr error
}

func (m *mockTokenValidator) ValidateToken(ctx context.Context, tokenStr string) (*auth.AuthClaims, error) {
	if m.failErr != nil {
		return nil, m.failErr
	}
	return m.claims, nil
}

// --- FAKE REPOSITORIES FOR HTTP TESTS ---

type fakeTxManager struct{}

func (f *fakeTxManager) Execute(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return fn(nil)
}

type fakeWalletRepo struct {
	mu      sync.Mutex
	wallets map[string]*domain.Wallet
}

func newFakeWalletRepo() *fakeWalletRepo {
	return &fakeWalletRepo{wallets: make(map[string]*domain.Wallet)}
}

func (f *fakeWalletRepo) GetByID(ctx context.Context, tx pgx.Tx, id string, forUpdate bool) (*domain.Wallet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.wallets[id]
	if !ok {
		return nil, errors.New("wallet not found")
	}
	return w, nil
}

func (f *fakeWalletRepo) Create(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wallets[w.ID()] = w
	return nil
}

func (f *fakeWalletRepo) Update(ctx context.Context, tx pgx.Tx, w *domain.Wallet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wallets[w.ID()] = w
	return nil
}

type fakeLedgerRepo struct {
	mu      sync.Mutex
	entries []*domain.WalletLedgerEntry
}

func newFakeLedgerRepo() *fakeLedgerRepo {
	return &fakeLedgerRepo{entries: make([]*domain.WalletLedgerEntry, 0)}
}

func (f *fakeLedgerRepo) GetByTransactionID(ctx context.Context, tx pgx.Tx, id string) (*domain.WalletLedgerEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.entries {
		if e.TransactionID() == id {
			return e, nil
		}
	}
	return nil, errors.New("ledger entry not found")
}

func (f *fakeLedgerRepo) ListByWalletID(ctx context.Context, tx pgx.Tx, walletID string) ([]*domain.WalletLedgerEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []*domain.WalletLedgerEntry
	for _, e := range f.entries {
		if e.WalletID() == walletID {
			res = append(res, e)
		}
	}
	return res, nil
}

func (f *fakeLedgerRepo) ListPaginated(ctx context.Context, tx pgx.Tx, walletID string, limit int, cursorCreatedAt *time.Time, cursorID string) ([]*domain.WalletLedgerEntry, error) {
	return f.ListByWalletID(ctx, tx, walletID)
}

func (f *fakeLedgerRepo) Create(ctx context.Context, tx pgx.Tx, l *domain.WalletLedgerEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, l)
	return nil
}

type fakeWagerRepo struct {
	mu            sync.Mutex
	byID          map[string]*domain.WagerTransaction
	byProviderExt map[string]*domain.WagerTransaction
	byIdemp       map[string]*domain.WagerTransaction
}

func newFakeWagerRepo() *fakeWagerRepo {
	return &fakeWagerRepo{
		byID:          make(map[string]*domain.WagerTransaction),
		byProviderExt: make(map[string]*domain.WagerTransaction),
		byIdemp:       make(map[string]*domain.WagerTransaction),
	}
}

func (f *fakeWagerRepo) GetByID(ctx context.Context, tx pgx.Tx, id string) (*domain.WagerTransaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return nil, errors.New("wager transaction not found")
	}
	return t, nil
}

func (f *fakeWagerRepo) GetByIdempotencyKey(ctx context.Context, tx pgx.Tx, providerID string, idempotencyKey string) (*domain.WagerTransaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := fmt.Sprintf("%s:%s", providerID, idempotencyKey)
	t, ok := f.byIdemp[k]
	if !ok {
		return nil, errors.New("wager not found by idempotency key")
	}
	return t, nil
}

func (f *fakeWagerRepo) GetByExternalID(ctx context.Context, tx pgx.Tx, providerID string, externalID string) (*domain.WagerTransaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := fmt.Sprintf("%s:%s", providerID, externalID)
	t, ok := f.byProviderExt[k]
	if !ok {
		return nil, errors.New("wager not found by external ID")
	}
	return t, nil
}

func (f *fakeWagerRepo) Create(ctx context.Context, tx pgx.Tx, t *domain.WagerTransaction) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[t.ID()] = t
	f.byProviderExt[fmt.Sprintf("%s:%s", t.ProviderID(), t.ExternalTransactionID())] = t
	f.byIdemp[fmt.Sprintf("%s:%s", t.ProviderID(), t.IdempotencyKey())] = t
	return nil
}

func (f *fakeWagerRepo) Update(ctx context.Context, tx pgx.Tx, t *domain.WagerTransaction) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[t.ID()] = t
	f.byProviderExt[fmt.Sprintf("%s:%s", t.ProviderID(), t.ExternalTransactionID())] = t
	f.byIdemp[fmt.Sprintf("%s:%s", t.ProviderID(), t.IdempotencyKey())] = t
	return nil
}

func (f *fakeWagerRepo) FetchPendingReferences(ctx context.Context, tx pgx.Tx, limit int) ([]*domain.WagerTransaction, error) {
	return nil, nil
}

type fakeOutboxRepo struct{}

func (f *fakeOutboxRepo) Create(ctx context.Context, tx pgx.Tx, msg *infradb.OutboxMessage) error {
	return nil
}
func (f *fakeOutboxRepo) FetchPending(ctx context.Context, tx pgx.Tx, limit int) ([]*infradb.OutboxMessage, error) {
	return nil, nil
}
func (f *fakeOutboxRepo) MarkPublished(ctx context.Context, tx pgx.Tx, id string) error {
	return nil
}
func (f *fakeOutboxRepo) ScheduleRetry(ctx context.Context, tx pgx.Tx, id string, nextAttemptAt time.Time) error {
	return nil
}
func (f *fakeOutboxRepo) MarkFailed(ctx context.Context, tx pgx.Tx, id string) error {
	return nil
}

type fakeInboxRepo struct{}

func (f *fakeInboxRepo) TryRecord(ctx context.Context, tx pgx.Tx, consumerName string, messageID string, payloadHash string) (bool, error) {
	return true, nil
}
func (f *fakeInboxRepo) Get(ctx context.Context, tx pgx.Tx, consumerName string, messageID string) (*infradb.InboxMessage, error) {
	return nil, errors.New("not found")
}
func (f *fakeInboxRepo) MarkProcessed(ctx context.Context, tx pgx.Tx, consumerName string, messageID string) error {
	return nil
}

// --- TEST SUITE SETUP ---

type testEnvironment struct {
	router     http.Handler
	val        *mockTokenValidator
	walletRepo *fakeWalletRepo
	ledgerRepo *fakeLedgerRepo
	wagerRepo  *fakeWagerRepo
}

func setupTestEnvironment() *testEnvironment {
	txMgr := &fakeTxManager{}
	walletRepo := newFakeWalletRepo()
	ledgerRepo := newFakeLedgerRepo()
	wagerRepo := newFakeWagerRepo()
	outboxRepo := &fakeOutboxRepo{}
	inboxRepo := &fakeInboxRepo{}

	createWalletUC := application.NewCreateWalletUseCase(txMgr, walletRepo, ledgerRepo, wagerRepo, outboxRepo)
	getWalletUC := application.NewGetWalletUseCase(txMgr, walletRepo)
	getLedgerUC := application.NewGetWalletLedgerUseCase(txMgr, walletRepo, ledgerRepo)
	processWagerUC := application.NewProcessWagerUseCase(txMgr, walletRepo, ledgerRepo, wagerRepo, outboxRepo, inboxRepo)
	getWagerUC := application.NewGetWagerTransactionUseCase(txMgr, wagerRepo)
	reconcileUC := application.NewReconcileWalletUseCase(txMgr, walletRepo, ledgerRepo, nil)

	walletHandler := infrahttp.NewWalletHandler(createWalletUC, getWalletUC, getLedgerUC)
	wagerHandler := infrahttp.NewWagerHandler(processWagerUC, getWagerUC)
	reconcileHandler := infrahttp.NewReconcileHandler(reconcileUC)
	healthHandler := infrahttp.NewHealthHandler(nil, nil)

	mockVal := &mockTokenValidator{
		claims: &auth.AuthClaims{
			Subject:    "user-1",
			Roles:      []string{"internal", "provider"},
			ProviderID: "provider-a",
		},
	}

	router := infrahttp.NewRouter(infrahttp.RouterParams{
		WalletHandler:    walletHandler,
		WagerHandler:     wagerHandler,
		ReconcileHandler: reconcileHandler,
		HealthHandler:    healthHandler,
		AuthValidator:    mockVal,
		Logger:           nil,
	})

	return &testEnvironment{
		router:     router,
		val:        mockVal,
		walletRepo: walletRepo,
		ledgerRepo: ledgerRepo,
		wagerRepo:  wagerRepo,
	}
}

// --- TESTS ---

func TestHealthEndpoints(t *testing.T) {
	env := setupTestEnvironment()

	t.Run("GET /health/live retorna 200 UP", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperava 200, recebeu %d", rec.Code)
		}
		var body map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body["status"] != "UP" {
			t.Errorf("esperava status UP, recebeu %s", body["status"])
		}
	})

	t.Run("GET /health/ready com dependencias nil retorna 503 DOWN", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("esperava 503, recebeu %d", rec.Code)
		}
	})
}

func TestWalletEndpoints(t *testing.T) {
	env := setupTestEnvironment()

	t.Run("POST /wallets cria carteira com sucesso", func(t *testing.T) {
		payload := []byte(`{"playerId":"player-10","initialBalance":{"amount":"100.50","currency":"USD"}}`)
		req := httptest.NewRequest(http.MethodPost, "/wallets", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("esperava 201 Created, recebeu %d (body: %s)", rec.Code, rec.Body.String())
		}
		var out application.CreateWalletOutput
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("erro ao deserializar saida: %v", err)
		}
		if out.PlayerID != "player-10" {
			t.Errorf("esperava player-10, recebeu %s", out.PlayerID)
		}
		if out.Balance.Amount != "100.50" {
			t.Errorf("esperava 100.50, recebeu %s", out.Balance.Amount)
		}
	})

	t.Run("POST /wallets com corpo invalido retorna 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/wallets", bytes.NewReader([]byte(`{invalid-json`)))
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("esperava 400 Bad Request, recebeu %d", rec.Code)
		}
	})

	t.Run("GET /wallets/{walletId} retorna carteira existente", func(t *testing.T) {
		wID := uuid.NewString()
		bal, _ := domain.ParseMoney("50.00", "USD")
		w, _ := domain.NewWallet(wID, "player-20", bal)
		_ = env.walletRepo.Create(context.Background(), nil, w)

		req := httptest.NewRequest(http.MethodGet, "/wallets/"+wID, nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperava 200 OK, recebeu %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /wallets/{walletId} retorna 404 para carteira inexistente", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/wallets/"+uuid.NewString(), nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("esperava 404 Not Found, recebeu %d", rec.Code)
		}
	})
}

func TestWagerEndpoints(t *testing.T) {
	env := setupTestEnvironment()

	// Cria carteira com 200.00 USD
	wID := uuid.NewString()
	bal, _ := domain.ParseMoney("200.00", "USD")
	w, _ := domain.NewWallet(wID, "player-100", bal)
	_ = env.walletRepo.Create(context.Background(), nil, w)

	t.Run("POST /wagering/transactions sem Idempotency-Key retorna 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/wagering/transactions", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("esperava 400 Bad Request, recebeu %d", rec.Code)
		}
	})

	t.Run("POST /wagering/transactions com isolamento de provedor violado retorna 403", func(t *testing.T) {
		// Token tem ProviderID "provider-a", mas requisicao vem com "provider-b" e sem role "internal"
		env.val.claims = &auth.AuthClaims{
			Subject:    "client-provider-a",
			Roles:      []string{"provider"},
			ProviderID: "provider-a",
		}

		payload := []byte(fmt.Sprintf(`{
			"providerId": "provider-b",
			"externalTransactionId": "ext-1",
			"playerId": "player-100",
			"walletId": "%s",
			"roundId": "round-1",
			"gameId": "game-1",
			"kind": "BET",
			"money": {"amount": "10.00", "currency": "USD"}
		}`, wID))

		req := httptest.NewRequest(http.MethodPost, "/wagering/transactions", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer valid-token")
		req.Header.Set("Idempotency-Key", "idemp-test-1")
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("esperava 403 Forbidden por violacao de isolamento, recebeu %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /wagering/transactions processa aposta com sucesso", func(t *testing.T) {
		env.val.claims = &auth.AuthClaims{
			Subject:    "client-provider-a",
			Roles:      []string{"provider"},
			ProviderID: "provider-a",
		}

		payload := []byte(fmt.Sprintf(`{
			"providerId": "provider-a",
			"externalTransactionId": "ext-success-1",
			"playerId": "player-100",
			"walletId": "%s",
			"roundId": "round-1",
			"gameId": "game-1",
			"kind": "BET",
			"money": {"amount": "10.00", "currency": "USD"}
		}`, wID))

		req := httptest.NewRequest(http.MethodPost, "/wagering/transactions", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer valid-token")
		req.Header.Set("Idempotency-Key", "idemp-bet-1")
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperava 200 OK, recebeu %d (body: %s)", rec.Code, rec.Body.String())
		}

		var out application.ProcessWagerOutput
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("falha ao deserializar resposta: %v", err)
		}
		if out.Status != "PROCESSED" {
			t.Errorf("esperava status PROCESSED, recebeu %s", out.Status)
		}
		if out.Balance.Amount != "190.00" {
			t.Errorf("esperava saldo 190.00, recebeu %s", out.Balance.Amount)
		}
	})

	t.Run("GET /wagering/transactions/{transactionId} com sucesso e isolamento", func(t *testing.T) {
		// Insere transacao no fake repo
		txID := uuid.NewString()
		wager, _ := domain.NewWagerTransaction(
			txID,
			"ext-query-1",
			"provider-a",
			"idemp-q-1",
			"hash-1",
			wID,
			"player-100",
			"game-1",
			"round-1",
			domain.TransactionTypeBet,
			bal,
			"",
		)
		_ = wager.MarkProcessed()
		_ = env.wagerRepo.Create(context.Background(), nil, wager)

		// 1. Acesso como internal -> 200
		env.val.claims = &auth.AuthClaims{
			Subject: "internal-service",
			Roles:   []string{"internal"},
		}
		req := httptest.NewRequest(http.MethodGet, "/wagering/transactions/"+txID, nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("esperava 200, recebeu %d", rec.Code)
		}

		// 2. Acesso como provider-b (mismatch) -> 403
		env.val.claims = &auth.AuthClaims{
			Subject:    "prov-b",
			Roles:      []string{"provider"},
			ProviderID: "provider-b",
		}
		reqMismatch := httptest.NewRequest(http.MethodGet, "/wagering/transactions/"+txID, nil)
		reqMismatch.Header.Set("Authorization", "Bearer valid-token")
		recMismatch := httptest.NewRecorder()
		env.router.ServeHTTP(recMismatch, reqMismatch)
		if recMismatch.Code != http.StatusForbidden {
			t.Fatalf("esperava 403 Forbidden para provedor diferente, recebeu %d", recMismatch.Code)
		}
	})

	t.Run("GET /providers/{providerId}/wagering/transactions/{externalTransactionId}", func(t *testing.T) {
		// Provedor consulta sua propria transacao
		env.val.claims = &auth.AuthClaims{
			Subject:    "prov-a",
			Roles:      []string{"provider"},
			ProviderID: "provider-a",
		}
		req := httptest.NewRequest(http.MethodGet, "/providers/provider-a/wagering/transactions/ext-query-1", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("esperava 200, recebeu %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestLedgerAndReconciliationEndpoints(t *testing.T) {
	env := setupTestEnvironment()

	wID := uuid.NewString()
	bal, _ := domain.ParseMoney("0.00", "USD")
	w, _ := domain.NewWallet(wID, "player-reconcile", bal)
	_ = env.walletRepo.Create(context.Background(), nil, w)

	t.Run("GET /wallets/{walletId}/ledger lista entradas paginadas", func(t *testing.T) {
		env.val.claims = &auth.AuthClaims{
			Subject: "internal-service",
			Roles:   []string{"internal"},
		}

		req := httptest.NewRequest(http.MethodGet, "/wallets/"+wID+"/ledger?limit=10", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperava 200 OK, recebeu %d (body: %s)", rec.Code, rec.Body.String())
		}
		var out application.GetWalletLedgerOutput
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("erro ao deserializar ledger: %v", err)
		}
	})

	t.Run("POST /wallets/{walletId}/reconciliation executa reconciliacao", func(t *testing.T) {
		env.val.claims = &auth.AuthClaims{
			Subject: "internal-service",
			Roles:   []string{"internal"},
		}

		req := httptest.NewRequest(http.MethodPost, "/wallets/"+wID+"/reconciliation", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperava 200 OK, recebeu %d (body: %s)", rec.Code, rec.Body.String())
		}
		var out application.ReconcileWalletOutput
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("erro ao deserializar reconciliacao: %v", err)
		}
		if out.WalletID != wID {
			t.Errorf("esperava walletID %s, recebeu %s", wID, out.WalletID)
		}
		if !out.Consistent {
			t.Errorf("esperava consistent = true para carteira sem discrepancias")
		}
	})
}

func TestRouterAuthAndCorrelation(t *testing.T) {
	env := setupTestEnvironment()

	t.Run("Requisicao protegida sem Authorization retorna 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/wallets", bytes.NewReader([]byte(`{}`)))
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("esperava 401 Unauthorized, recebeu %d", rec.Code)
		}
	})

	t.Run("Requisicao para endpoint interno com role apenas provider retorna 403", func(t *testing.T) {
		env.val.claims = &auth.AuthClaims{
			Subject:    "prov-client",
			Roles:      []string{"provider"},
			ProviderID: "provider-a",
		}

		req := httptest.NewRequest(http.MethodPost, "/wallets", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("esperava 403 Forbidden para rota interna, recebeu %d", rec.Code)
		}
	})

	t.Run("Propagacao e geracao de X-Correlation-ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
		rec := httptest.NewRecorder()

		env.router.ServeHTTP(rec, req)

		corrID := rec.Header().Get("X-Correlation-ID")
		if corrID == "" {
			t.Errorf("esperava X-Correlation-ID gerado no cabecalho de resposta")
		}

		// Agora passando um correlation id customizado
		customID := "test-corr-12345"
		req2 := httptest.NewRequest(http.MethodGet, "/health/live", nil)
		req2.Header.Set("X-Correlation-ID", customID)
		rec2 := httptest.NewRecorder()

		env.router.ServeHTTP(rec2, req2)
		if rec2.Header().Get("X-Correlation-ID") != customID {
			t.Errorf("esperava %s, recebeu %s", customID, rec2.Header().Get("X-Correlation-ID"))
		}
	})
}
