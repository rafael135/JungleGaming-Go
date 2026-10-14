package http

import (
	"net/http"

	"jungle/internal/application"
)

type ReconcileHandler struct {
	reconcileUseCase *application.ReconcileWalletUseCase
}

func NewReconcileHandler(reconcileUseCase *application.ReconcileWalletUseCase) *ReconcileHandler {
	return &ReconcileHandler{
		reconcileUseCase: reconcileUseCase,
	}
}

func (h *ReconcileHandler) ReconcileWallet(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("walletId")

	output, err := h.reconcileUseCase.Execute(r.Context(), application.ReconcileWalletInput{
		WalletID: walletID,
	})
	if err != nil {
		status, code, msg := mapAppErrorToHTTP(err)
		writeError(w, status, code, msg)
		return
	}

	writeJSON(w, http.StatusOK, output)
}
