package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"jungle/internal/application"
	"jungle/internal/domain"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func writeError(w http.ResponseWriter, status int, errCode string, message string) {
	writeJSON(w, status, ErrorResponse{
		Error:   errCode,
		Message: message,
	})
}

func mapAppErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, application.ErrWalletNotFound):
		return http.StatusNotFound, "wallet_not_found", "carteira nao encontrada"

	case errors.Is(err, application.ErrTransactionNotFound):
		return http.StatusNotFound, "transaction_not_found", "transacao nao encontrada"

	case errors.Is(err, application.ErrIdempotencyConflict):
		return http.StatusConflict, "idempotency_conflict", err.Error()

	case errors.Is(err, application.ErrExternalTransactionConflict):
		return http.StatusConflict, "external_transaction_conflict", err.Error()

	case errors.Is(err, application.ErrOpeningTransactionForbidden):
		return http.StatusBadRequest, "invalid_operation", err.Error()

	case errors.Is(err, application.ErrInvalidCursor):
		return http.StatusBadRequest, "invalid_cursor", err.Error()

	case errors.Is(err, application.ErrReferenceMismatch):
		return http.StatusBadRequest, "reference_mismatch", err.Error()

	case errors.Is(err, domain.ErrInvalidAmountFormat),
		errors.Is(err, domain.ErrNegativeAmount),
		errors.Is(err, domain.ErrInvalidScale),
		errors.Is(err, domain.ErrInvalidCurrency),
		errors.Is(err, domain.ErrInvalidPlayerId),
		errors.Is(err, domain.ErrInvalidWalletId),
		errors.Is(err, domain.ErrInvalidTransactionType),
		errors.Is(err, domain.ErrInvalidIdempotencyKey),
		errors.Is(err, domain.ErrInvalidExternalTransactionId),
		errors.Is(err, domain.ErrInvalidReferenceExternalTransactionId):
		return http.StatusBadRequest, "bad_request", err.Error()

	default:
		return http.StatusInternalServerError, "internal_error", "erro interno no processamento"
	}
}
