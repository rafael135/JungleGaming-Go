package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeJSONError(w http.ResponseWriter, status int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:   errCode,
		Message: message,
	})
}

// Authenticate valida o token Bearer JWT e propaga os claims no context.
func Authenticate(validator TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "header Authorization ausente")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "formato de token invalido, esperado 'Bearer <token>'")
				return
			}

			tokenStr := strings.TrimSpace(parts[1])
			claims, err := validator.ValidateToken(r.Context(), tokenStr)
			if err != nil {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "token invalido ou expirado")
				return
			}

			ctx := ContextWithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole exige que o token possua a role informada.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok || claims == nil {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "claims de autenticacao ausentes")
				return
			}

			if !claims.HasRole(role) && !claims.HasRole("admin") {
				writeJSONError(w, http.StatusForbidden, "forbidden", "permissao insuficiente para esta operacao")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// EnforceProviderIsolation garante que um provedor acesse somente recursos próprios.
// Serviços internos com a role "internal" podem acessar qualquer provedor.
func EnforceProviderIsolation(paramProviderID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok || claims == nil {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "claims de autenticacao ausentes")
				return
			}

			// Se for serviço interno, acesso irrestrito
			if claims.HasRole("internal") || claims.HasRole("admin") {
				next.ServeHTTP(w, r)
				return
			}

			// Se for provedor externo, valida correspondência
			targetProviderID := r.PathValue(paramProviderID)
			if targetProviderID != "" && claims.ProviderID != "" && targetProviderID != claims.ProviderID {
				writeJSONError(w, http.StatusForbidden, "forbidden", "provedor nao autorizado a acessar transacoes de outro provedor")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
