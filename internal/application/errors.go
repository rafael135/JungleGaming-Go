package application

import "errors"

var (
	ErrIdempotencyConflict         = errors.New("idempotency key already used with different payload")
	ErrExternalTransactionConflict = errors.New("external transaction id already used with different idempotency key")
	ErrOpeningTransactionForbidden = errors.New("opening transaction type is forbidden in external API/messaging")
	ErrWalletNotFound              = errors.New("wallet not found")
	ErrPlayerWalletMismatch        = errors.New("player id does not match wallet owner")
	ErrCurrencyMismatch            = errors.New("transaction currency does not match wallet currency")
	ErrReferenceMismatch           = errors.New("referenced transaction attributes do not match reversal request")
	ErrDuplicateReversal           = errors.New("reversal of this type was already processed for this reference")
	ErrInvalidReferenceType        = errors.New("referenced transaction cannot be reversed with requested kind")
	ErrInvalidCursor               = errors.New("invalid pagination cursor")
	ErrInvalidTransactionQuery     = errors.New("transaction query requires transactionId or (providerId and externalTransactionId)")
	ErrTransactionNotFound         = errors.New("transaction not found")
)

