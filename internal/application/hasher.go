package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// CanonicalWagerPayload define os campos de negócio serializados para cálculo
// determinístico do hash de idempotência (excluindo headers e transporte).
type CanonicalWagerPayload struct {
	Amount                         string `json:"amount"`
	Currency                       string `json:"currency"`
	ExternalTransactionID          string `json:"externalTransactionId"`
	GameID                         string `json:"gameId"`
	Kind                           string `json:"kind"`
	PlayerID                       string `json:"playerId"`
	ProviderID                     string `json:"providerId"`
	ReferenceExternalTransactionID string `json:"referenceExternalTransactionId,omitempty"`
	RoundID                        string `json:"roundId"`
	WalletID                       string `json:"walletId"`
}

// ComputePayloadHash gera o hash SHA-256 do payload canônico de negócio.
func ComputePayloadHash(input ProcessWagerInput) (string, error) {
	canonical := CanonicalWagerPayload{
		Amount:                         input.Amount,
		Currency:                       input.Currency,
		ExternalTransactionID:          input.ExternalTransactionID,
		GameID:                         input.GameID,
		Kind:                           input.Kind,
		PlayerID:                       input.PlayerID,
		ProviderID:                     input.ProviderID,
		ReferenceExternalTransactionID: input.ReferenceExternalTransactionID,
		RoundID:                        input.RoundID,
		WalletID:                       input.WalletID,
	}

	bytes, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("erro ao gerar JSON canonico para hash de idempotencia: %w", err)
	}

	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:]), nil
}

