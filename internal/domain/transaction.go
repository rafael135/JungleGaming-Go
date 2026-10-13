package domain

import "time"

type TransactionType string

const (
	TransactionTypeOpening  = "OPENING"
	TransactionTypeBet      = "BET"
	TransactionTypeWin      = "WIN"
	TransactionTypeLoss     = "LOSS"
	TransactionTypeRefund   = "REFUND"
	TransactionTypeRollback = "ROLLBACK"
)

type TransactionStatus string

const (
	TransactionStatusPending          = "PENDING"
	TransactionStatusPendingReference = "PENDING_REFERENCE"
	TransactionStatusProcessed        = "PROCESSED"
	TransactionStatusRejected         = "REJECTED"
	TransactionStatusFailed           = "FAILED"
)

type WagerTransaction struct {
	id                             string
	externalTransactionID          string
	providerID                     string
	idempotencyKey                 string
	payloadHash                    string
	walletID                       string
	playerID                       string
	gameID                         string
	roundID                        string
	transactionType                TransactionType
	amount                         Money
	referenceExternalTransactionID string
	status                         TransactionStatus
	failureCode                    string
	createdAt                      time.Time
	updatedAt                      time.Time
}

func NewWagerTransaction(
	id string,
	externalTransactionID string,
	providerID string,
	idempotencyKey string,
	payloadHash string,
	walletID string,
	playerID string,
	gameID string,
	roundID string,
	transactionType TransactionType,
	amount Money,
	referenceExternalTransactionID string,
) (*WagerTransaction, error) {

	if id == "" {
		return nil, ErrInvalidTransactionId
	}

	if externalTransactionID == "" {
		return nil, ErrInvalidExternalTransactionId
	}

	if providerID == "" {
		return nil, ErrInvalidProviderId
	}

	if idempotencyKey == "" {
		return nil, ErrInvalidIdempotencyKey
	}

	if payloadHash == "" {
		return nil, ErrInvalidPayloadHash
	}

	if walletID == "" {
		return nil, ErrInvalidWalletId
	}

	if playerID == "" {
		return nil, ErrInvalidPlayerId
	}

	if gameID == "" {
		return nil, ErrInvalidGameId
	}

	if roundID == "" {
		return nil, ErrInvalidRoundId
	}

	if transactionType != TransactionTypeOpening &&
		transactionType != TransactionTypeBet &&
		transactionType != TransactionTypeWin &&
		transactionType != TransactionTypeLoss &&
		transactionType != TransactionTypeRefund &&
		transactionType != TransactionTypeRollback {
		return nil, ErrInvalidTransactionType
	}

	if transactionType == TransactionTypeLoss {
		if amount.Cents() != 0 {
			return nil, ErrInvalidAmountFormat
		}
	} else if transactionType != TransactionTypeOpening {
		if amount.Cents() <= 0 {
			return nil, ErrNegativeAmount
		}
	}

	if transactionType == TransactionTypeRefund || transactionType == TransactionTypeRollback {
		if referenceExternalTransactionID == "" {
			return nil, ErrInvalidReferenceExternalTransactionId
		}
	}

	now := time.Now().UTC()

	return &WagerTransaction{
		id:                             id,
		externalTransactionID:          externalTransactionID,
		providerID:                     providerID,
		idempotencyKey:                 idempotencyKey,
		payloadHash:                    payloadHash,
		walletID:                       walletID,
		playerID:                       playerID,
		gameID:                         gameID,
		roundID:                        roundID,
		transactionType:                transactionType,
		amount:                         amount,
		referenceExternalTransactionID: referenceExternalTransactionID,
		status:                         TransactionStatusPending,
		failureCode:                    "",
		createdAt:                      now,
		updatedAt:                      now,
	}, nil
}

func NewOpeningTransaction(id string, walletID string, playerID string, amount Money) (*WagerTransaction, error) {
	if id == "" {
		return nil, ErrInvalidTransactionId
	}
	if walletID == "" {
		return nil, ErrInvalidWalletId
	}
	if playerID == "" {
		return nil, ErrInvalidPlayerId
	}

	if amount.Cents() < 0 {
		return nil, ErrNegativeAmount
	}

	if len(amount.Currency()) != 3 {
		return nil, ErrInvalidCurrency
	}

	now := time.Now().UTC()
	return &WagerTransaction{
		id:              id,
		walletID:        walletID,
		playerID:        playerID,
		transactionType: TransactionTypeOpening,
		amount:          amount,
		status:          TransactionStatusProcessed, // Abertura já nasce confirmada
		createdAt:       now,
		updatedAt:       now,
	}, nil
}

func RehydrateWagerTransaction(
	id string,
	externalTransactionID string,
	providerID string,
	idempotencyKey string,
	payloadHash string,
	walletID string,
	playerID string,
	gameID string,
	roundID string,
	transactionType TransactionType,
	amount Money,
	referenceExternalTransactionID string,
	status TransactionStatus,
	failureCode string,
	createdAt time.Time,
	updatedAt time.Time,
) (*WagerTransaction, error) {
	if id == "" {
		return nil, ErrInvalidTransactionId
	}

	if externalTransactionID == "" {
		return nil, ErrInvalidExternalTransactionId
	}

	if providerID == "" {
		return nil, ErrInvalidProviderId
	}

	if idempotencyKey == "" {
		return nil, ErrInvalidIdempotencyKey
	}

	if payloadHash == "" {
		return nil, ErrInvalidPayloadHash
	}

	if walletID == "" {
		return nil, ErrInvalidWalletId
	}

	if playerID == "" {
		return nil, ErrInvalidPlayerId
	}

	if gameID == "" {
		return nil, ErrInvalidGameId
	}

	if roundID == "" {
		return nil, ErrInvalidRoundId
	}

	if transactionType != TransactionTypeOpening &&
		transactionType != TransactionTypeBet &&
		transactionType != TransactionTypeWin &&
		transactionType != TransactionTypeLoss &&
		transactionType != TransactionTypeRefund &&
		transactionType != TransactionTypeRollback {
		return nil, ErrInvalidTransactionType
	}

	if transactionType == TransactionTypeLoss {
		if amount.Cents() != 0 {
			return nil, ErrInvalidAmountFormat
		}
	} else if transactionType != TransactionTypeOpening {
		if amount.Cents() <= 0 {
			return nil, ErrNegativeAmount
		}
	}

	if transactionType == TransactionTypeRefund || transactionType == TransactionTypeRollback {
		if referenceExternalTransactionID == "" {
			return nil, ErrInvalidReferenceExternalTransactionId
		}
	}

	return &WagerTransaction{
		id:                             id,
		externalTransactionID:          externalTransactionID,
		providerID:                     providerID,
		idempotencyKey:                 idempotencyKey,
		payloadHash:                    payloadHash,
		walletID:                       walletID,
		playerID:                       playerID,
		gameID:                         gameID,
		roundID:                        roundID,
		transactionType:                transactionType,
		amount:                         amount,
		referenceExternalTransactionID: referenceExternalTransactionID,
		status:                         status,
		failureCode:                    failureCode,
		createdAt:                      createdAt,
		updatedAt:                      updatedAt,
	}, nil
}

func (t *WagerTransaction) IsTerminal() bool {
	return t.status == TransactionStatusProcessed ||
		t.status == TransactionStatusRejected ||
		t.status == TransactionStatusFailed
}

func (t *WagerTransaction) MarkProcessed() error {
	if t.status != TransactionStatusPendingReference && t.status != TransactionStatusPending {
		return ErrInvalidStateTransition
	}

	t.status = TransactionStatusProcessed
	t.updatedAt = time.Now().UTC()

	return nil
}

func (t *WagerTransaction) MarkPendingReference() error {
	if t.status != TransactionStatusPending {
		return ErrInvalidStateTransition
	}

	t.status = TransactionStatusPendingReference
	t.updatedAt = time.Now().UTC()

	return nil
}

func (t *WagerTransaction) MarkRejected(failureCode string) error {
	if t.IsTerminal() {
		return ErrInvalidStateTransition
	}

	t.failureCode = failureCode
	t.status = TransactionStatusRejected
	t.updatedAt = time.Now().UTC()

	return nil
}

func (t *WagerTransaction) MarkFailed(failureCode string) error {
	if t.IsTerminal() {
		return ErrInvalidStateTransition
	}

	t.failureCode = failureCode
	t.status = TransactionStatusFailed
	t.updatedAt = time.Now().UTC()

	return nil
}

func (t WagerTransaction) ID() string {
	return t.id
}

func (t WagerTransaction) ExternalTransactionID() string {
	return t.externalTransactionID
}

func (t WagerTransaction) ProviderID() string {
	return t.providerID
}

func (t WagerTransaction) IdempotencyKey() string {
	return t.idempotencyKey
}

func (t WagerTransaction) PayloadHash() string {
	return t.payloadHash
}

func (t WagerTransaction) WalletID() string {
	return t.walletID
}

func (t WagerTransaction) PlayerID() string {
	return t.playerID
}

func (t WagerTransaction) GameID() string {
	return t.gameID
}

func (t WagerTransaction) RoundID() string {
	return t.roundID
}

func (t WagerTransaction) TransactionType() TransactionType {
	return t.transactionType
}

func (t WagerTransaction) Amount() Money {
	return t.amount
}

func (t WagerTransaction) ReferenceExternalTransactionID() string {
	return t.referenceExternalTransactionID
}

func (t WagerTransaction) Status() TransactionStatus {
	return t.status
}

func (t WagerTransaction) FailureCode() string {
	return t.failureCode
}

func (t WagerTransaction) CreatedAt() time.Time {
	return t.createdAt
}

func (t WagerTransaction) UpdatedAt() time.Time {
	return t.updatedAt
}
