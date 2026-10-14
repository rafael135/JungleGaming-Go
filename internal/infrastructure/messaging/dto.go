package messaging

type SQSRequestedMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type SQSRequestedData struct {
	ProviderID                     string            `json:"providerId"`
	ExternalTransactionID          string            `json:"externalTransactionId"`
	IdempotencyKey                 string            `json:"idempotencyKey"`
	PlayerID                       string            `json:"playerId"`
	WalletID                       string            `json:"walletId"`
	RoundID                        string            `json:"roundId"`
	GameID                         string            `json:"gameId"`
	Kind                           string            `json:"kind"`
	Money                          SQSRequestedMoney `json:"money"`
	ReferenceExternalTransactionID string            `json:"referenceExternalTransactionId,omitempty"`
}

type SQSRequestedMessageEnvelope struct {
	MessageID  string           `json:"messageId"`
	Type       string           `json:"type"`
	OccurredAt string           `json:"occurredAt"`
	Data       SQSRequestedData `json:"data"`
}
