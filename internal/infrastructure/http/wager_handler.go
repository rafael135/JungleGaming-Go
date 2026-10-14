package http

import (
	"encoding/json"
	"net/http"

	"jungle/internal/application"
	"jungle/internal/infrastructure/auth"
)

type ProcessWagerRequest struct {
	ProviderID                     string                    `json:"providerId"`
	ExternalTransactionID          string                    `json:"externalTransactionId"`
	PlayerID                       string                    `json:"playerId"`
	WalletID                       string                    `json:"walletId"`
	RoundID                        string                    `json:"roundId"`
	GameID                         string                    `json:"gameId"`
	Kind                           string                    `json:"kind"`
	Money                          application.WalletBalance `json:"money"`
	ReferenceExternalTransactionID string                    `json:"referenceExternalTransactionId,omitempty"`
}

type WagerHandler struct {
	processWagerUseCase        *application.ProcessWagerUseCase
	getWagerTransactionUseCase *application.GetWagerTransactionUseCase
}

func NewWagerHandler(
	processWagerUseCase *application.ProcessWagerUseCase,
	getWagerTransactionUseCase *application.GetWagerTransactionUseCase,
) *WagerHandler {
	return &WagerHandler{
		processWagerUseCase:        processWagerUseCase,
		getWagerTransactionUseCase: getWagerTransactionUseCase,
	}
}

func (h *WagerHandler) ProcessTransaction(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "missing_header", "header Idempotency-Key e obrigatorio")
		return
	}

	var req ProcessWagerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "corpo JSON invalido")
		return
	}

	// Isolamento de Tenant / Provedor
	if claims, ok := auth.ClaimsFromContext(r.Context()); ok && claims != nil {
		if !claims.HasRole("internal") && !claims.HasRole("admin") {
			if claims.ProviderID != "" && req.ProviderID != "" && req.ProviderID != claims.ProviderID {
				writeError(w, http.StatusForbidden, "forbidden", "provedor autenticado nao pode enviar transacoes para outro provedor")
				return
			}
		}
	}

	input := application.ProcessWagerInput{
		IdempotencyKey:                 idempotencyKey,
		ProviderID:                     req.ProviderID,
		ExternalTransactionID:          req.ExternalTransactionID,
		PlayerID:                       req.PlayerID,
		WalletID:                       req.WalletID,
		GameID:                         req.GameID,
		RoundID:                        req.RoundID,
		Kind:                           req.Kind,
		Amount:                         req.Money.Amount,
		Currency:                       req.Money.Currency,
		ReferenceExternalTransactionID: req.ReferenceExternalTransactionID,
	}

	output, err := h.processWagerUseCase.Execute(r.Context(), input)
	if err != nil {
		status, code, msg := mapAppErrorToHTTP(err)
		writeError(w, status, code, msg)
		return
	}

	writeJSON(w, http.StatusOK, output)
}

func (h *WagerHandler) GetTransactionByID(w http.ResponseWriter, r *http.Request) {
	txID := r.PathValue("transactionId")

	output, err := h.getWagerTransactionUseCase.Execute(r.Context(), application.GetWagerTransactionInput{
		TransactionID: txID,
	})
	if err != nil {
		status, code, msg := mapAppErrorToHTTP(err)
		writeError(w, status, code, msg)
		return
	}

	// Isolamento de Tenant na leitura por ID interno
	if claims, ok := auth.ClaimsFromContext(r.Context()); ok && claims != nil {
		if !claims.HasRole("internal") && !claims.HasRole("admin") {
			if claims.ProviderID != "" && output.ProviderID != claims.ProviderID {
				writeError(w, http.StatusForbidden, "forbidden", "provedor nao autorizado a visualizar transacoes de outro provedor")
				return
			}
		}
	}

	writeJSON(w, http.StatusOK, output)
}

func (h *WagerHandler) GetTransactionByExternalID(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("providerId")
	extID := r.PathValue("externalTransactionId")

	// Isolamento de Tenant
	if claims, ok := auth.ClaimsFromContext(r.Context()); ok && claims != nil {
		if !claims.HasRole("internal") && !claims.HasRole("admin") {
			if claims.ProviderID != "" && providerID != claims.ProviderID {
				writeError(w, http.StatusForbidden, "forbidden", "provedor nao autorizado a consultar outro provedor")
				return
			}
		}
	}

	output, err := h.getWagerTransactionUseCase.Execute(r.Context(), application.GetWagerTransactionInput{
		ProviderID:            providerID,
		ExternalTransactionID: extID,
	})
	if err != nil {
		status, code, msg := mapAppErrorToHTTP(err)
		writeError(w, status, code, msg)
		return
	}

	writeJSON(w, http.StatusOK, output)
}
