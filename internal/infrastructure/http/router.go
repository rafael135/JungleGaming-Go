package http

import (
	"log/slog"
	"net/http"
	"time"

	"jungle/internal/infrastructure/auth"

	"github.com/google/uuid"
)

type RouterParams struct {
	WalletHandler    *WalletHandler
	WagerHandler     *WagerHandler
	ReconcileHandler *ReconcileHandler
	HealthHandler    *HealthHandler
	AuthValidator    auth.TokenValidator
	Logger           *slog.Logger
}

func NewRouter(p RouterParams) http.Handler {
	mux := http.NewServeMux()

	// 1. Rotas Públicas (Health Checks)
	mux.HandleFunc("GET /health/live", p.HealthHandler.Live)
	mux.HandleFunc("GET /health/ready", p.HealthHandler.Ready)

	// Middlewares
	authMW := auth.Authenticate(p.AuthValidator)
	requireInternal := auth.RequireRole("internal")
	providerIsolation := auth.EnforceProviderIsolation("providerId")

	// 2. Rotas Internas de Carteira (Requer role "internal")
	mux.Handle("POST /wallets", authMW(requireInternal(http.HandlerFunc(p.WalletHandler.CreateWallet))))
	mux.Handle("GET /wallets/{walletId}", authMW(requireInternal(http.HandlerFunc(p.WalletHandler.GetWallet))))
	mux.Handle("GET /wallets/{walletId}/ledger", authMW(requireInternal(http.HandlerFunc(p.WalletHandler.GetLedger))))
	mux.Handle("POST /wallets/{walletId}/reconciliation", authMW(requireInternal(http.HandlerFunc(p.ReconcileHandler.ReconcileWallet))))

	// 3. Rotas de Operações de Apostas
	mux.Handle("POST /wagering/transactions", authMW(http.HandlerFunc(p.WagerHandler.ProcessTransaction)))
	mux.Handle("GET /wagering/transactions/{transactionId}", authMW(http.HandlerFunc(p.WagerHandler.GetTransactionByID)))
	mux.Handle("GET /providers/{providerId}/wagering/transactions/{externalTransactionId}",
		authMW(providerIsolation(http.HandlerFunc(p.WagerHandler.GetTransactionByExternalID))))

	// Middleware Global de Correlation ID e Logging
	return correlationAndLoggingMiddleware(mux, p.Logger)
}

func correlationAndLoggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.NewString()
		}
		w.Header().Set("X-Correlation-ID", correlationID)

		start := time.Now()
		wrapped := &responseWriterInterceptor{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		logger.InfoContext(r.Context(), "requisicao http concluida",
			slog.String("correlation_id", correlationID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.statusCode),
			slog.Duration("latency", time.Since(start)),
		)
	})
}

type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriterInterceptor) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}
