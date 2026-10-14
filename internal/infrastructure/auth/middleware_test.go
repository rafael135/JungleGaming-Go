package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"jungle/internal/infrastructure/auth"
)

type mockValidator struct {
	claims  *auth.AuthClaims
	failErr error
}

func (m *mockValidator) ValidateToken(ctx context.Context, tokenStr string) (*auth.AuthClaims, error) {
	if m.failErr != nil {
		return nil, m.failErr
	}
	return m.claims, nil
}

func TestAuthMiddlewares(t *testing.T) {
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if ok && claims != nil {
			w.Header().Set("X-User-Subject", claims.Subject)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	t.Run("Authenticate: header ausente retorna 401", func(t *testing.T) {
		val := &mockValidator{}
		mw := auth.Authenticate(val)
		handler := mw(dummyHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status esperado 401, recebeu %d", rec.Code)
		}
	})

	t.Run("Authenticate: formato sem Bearer retorna 401", func(t *testing.T) {
		val := &mockValidator{}
		mw := auth.Authenticate(val)
		handler := mw(dummyHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Basic 123456")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status esperado 401, recebeu %d", rec.Code)
		}
	})

	t.Run("Authenticate: token invalido retorna 401", func(t *testing.T) {
		val := &mockValidator{failErr: errors.New("jwt expired")}
		mw := auth.Authenticate(val)
		handler := mw(dummyHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer token-invalido")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status esperado 401, recebeu %d", rec.Code)
		}
	})

	t.Run("Authenticate: token valido propaga claims no context", func(t *testing.T) {
		val := &mockValidator{
			claims: &auth.AuthClaims{
				Subject:    "user-123",
				ClientID:   "provider-a",
				ProviderID: "provider-a",
				Roles:      []string{"provider"},
			},
		}
		mw := auth.Authenticate(val)
		handler := mw(dummyHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer token-valido")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status esperado 200, recebeu %d", rec.Code)
		}
		if rec.Header().Get("X-User-Subject") != "user-123" {
			t.Errorf("claims nao foram propagados corretamente no context")
		}
	})

	t.Run("RequireRole: role correta permite acesso", func(t *testing.T) {
		claims := &auth.AuthClaims{Roles: []string{"internal"}}
		ctx := auth.ContextWithClaims(context.Background(), claims)

		handler := auth.RequireRole("internal")(dummyHandler)

		req := httptest.NewRequest(http.MethodGet, "/wallets", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status esperado 200, recebeu %d", rec.Code)
		}
	})

	t.Run("RequireRole: role divergente retorna 403 Forbidden", func(t *testing.T) {
		claims := &auth.AuthClaims{Roles: []string{"provider"}}
		ctx := auth.ContextWithClaims(context.Background(), claims)

		handler := auth.RequireRole("internal")(dummyHandler)

		req := httptest.NewRequest(http.MethodGet, "/wallets", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("status esperado 403, recebeu %d", rec.Code)
		}
	})

	t.Run("EnforceProviderIsolation: provedor acessando próprio ID é permitido", func(t *testing.T) {
		claims := &auth.AuthClaims{
			ProviderID: "provider-a",
			Roles:      []string{"provider"},
		}
		ctx := auth.ContextWithClaims(context.Background(), claims)

		mux := http.NewServeMux()
		mux.Handle("GET /providers/{providerId}/transactions", auth.EnforceProviderIsolation("providerId")(dummyHandler))

		req := httptest.NewRequest(http.MethodGet, "/providers/provider-a/transactions", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status esperado 200, recebeu %d", rec.Code)
		}
	})

	t.Run("EnforceProviderIsolation: provedor tentando acessar outro provedor retorna 403", func(t *testing.T) {
		claims := &auth.AuthClaims{
			ProviderID: "provider-a",
			Roles:      []string{"provider"},
		}
		ctx := auth.ContextWithClaims(context.Background(), claims)

		mux := http.NewServeMux()
		mux.Handle("GET /providers/{providerId}/transactions", auth.EnforceProviderIsolation("providerId")(dummyHandler))

		req := httptest.NewRequest(http.MethodGet, "/providers/provider-b/transactions", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("status esperado 403 Forbidden para acesso cruzado, recebeu %d", rec.Code)
		}
	})

	t.Run("EnforceProviderIsolation: servico interno com role internal tem acesso irrestrito", func(t *testing.T) {
		claims := &auth.AuthClaims{
			Roles: []string{"internal"},
		}
		ctx := auth.ContextWithClaims(context.Background(), claims)

		mux := http.NewServeMux()
		mux.Handle("GET /providers/{providerId}/transactions", auth.EnforceProviderIsolation("providerId")(dummyHandler))

		req := httptest.NewRequest(http.MethodGet, "/providers/provider-b/transactions", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status esperado 200 para role internal, recebeu %d", rec.Code)
		}
	})
}
