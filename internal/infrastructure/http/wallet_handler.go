package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"jungle/internal/application"
)

type CreateWalletRequest struct {
	PlayerID       string                    `json:"playerId"`
	InitialBalance application.WalletBalance `json:"initialBalance"`
}

type WalletHandler struct {
	createWalletUseCase    *application.CreateWalletUseCase
	getWalletUseCase       *application.GetWalletUseCase
	getWalletLedgerUseCase *application.GetWalletLedgerUseCase
}

func NewWalletHandler(
	createWalletUseCase *application.CreateWalletUseCase,
	getWalletUseCase *application.GetWalletUseCase,
	getWalletLedgerUseCase *application.GetWalletLedgerUseCase,
) *WalletHandler {
	return &WalletHandler{
		createWalletUseCase:    createWalletUseCase,
		getWalletUseCase:       getWalletUseCase,
		getWalletLedgerUseCase: getWalletLedgerUseCase,
	}
}

func (h *WalletHandler) CreateWallet(w http.ResponseWriter, r *http.Request) {
	var req CreateWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "corpo JSON invalido")
		return
	}

	output, err := h.createWalletUseCase.Execute(r.Context(), application.CreateWalletInput{
		PlayerID:       req.PlayerID,
		InitialBalance: req.InitialBalance,
	})
	if err != nil {
		status, code, msg := mapAppErrorToHTTP(err)
		writeError(w, status, code, msg)
		return
	}

	writeJSON(w, http.StatusCreated, output)
}

func (h *WalletHandler) GetWallet(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("walletId")

	output, err := h.getWalletUseCase.Execute(r.Context(), application.GetWalletInput{
		WalletID: walletID,
	})
	if err != nil {
		status, code, msg := mapAppErrorToHTTP(err)
		writeError(w, status, code, msg)
		return
	}

	writeJSON(w, http.StatusOK, output)
}

func (h *WalletHandler) GetLedger(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("walletId")
	cursor := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	output, err := h.getWalletLedgerUseCase.Execute(r.Context(), application.GetWalletLedgerInput{
		WalletID: walletID,
		Limit:    limit,
		Cursor:   cursor,
	})
	if err != nil {
		status, code, msg := mapAppErrorToHTTP(err)
		writeError(w, status, code, msg)
		return
	}

	writeJSON(w, http.StatusOK, output)
}
