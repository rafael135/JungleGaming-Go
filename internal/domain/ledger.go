package domain

import "time"

// "DEBIT" ou "CREDIT"
type LedgerDirection string

const (
	LedgerDirectionDebit  LedgerDirection = "DEBIT"
	LedgerDirectionCredit LedgerDirection = "CREDIT"
)

type WalletLedgerEntry struct {
	id            string
	walletID      string
	transactionID string
	direction     LedgerDirection
	amount        Money
	balanceBefore Money
	balanceAfter  Money
	createdAt     time.Time
}

func NewWalletLedgerEntry(id string, walletID string, transactionID string, direction LedgerDirection, amount Money, balanceBefore Money, balanceAfter Money) (*WalletLedgerEntry, error) {
	if id == "" {
		return nil, ErrInvalidLedgerId
	}

	if walletID == "" {
		return nil, ErrInvalidWalletId
	}

	if transactionID == "" {
		return nil, ErrInvalidTransactionId
	}

	if direction != LedgerDirectionDebit && direction != LedgerDirectionCredit {
		return nil, ErrInvalidLedgerDirection
	}

	if amount.Cents() <= 0 {
		return nil, ErrNegativeAmount
	}

	currencys := []string{amount.Currency(), balanceBefore.Currency(), balanceAfter.Currency()}
	allEqual := currencys[0] == currencys[1] && currencys[1] == currencys[2]

	if !allEqual {
		return nil, ErrCurrencyMismatch
	}

	isLedgerValid := false

	if direction == LedgerDirectionDebit {
		isLedgerValid = (balanceBefore.Cents() - amount.Cents()) == balanceAfter.Cents()
	} else {
		isLedgerValid = (balanceBefore.Cents() + amount.Cents()) == balanceAfter.Cents()
	}

	if !isLedgerValid {
		return nil, ErrInvalidLedgerMath
	}

	return &WalletLedgerEntry{
		id:            id,
		walletID:      walletID,
		transactionID: transactionID,
		direction:     direction,
		amount:        amount,
		balanceBefore: balanceBefore,
		balanceAfter:  balanceAfter,
		createdAt:     time.Now().UTC(),
	}, nil
}

func RehydrateWalletLedgerEntry(
	id string,
	walletID string,
	transactionID string,
	direction LedgerDirection,
	amount Money,
	balanceBefore Money,
	balanceAfter Money,
	createdAt time.Time,
) (*WalletLedgerEntry, error) {
	if id == "" {
		return nil, ErrInvalidLedgerId
	}

	if walletID == "" {
		return nil, ErrInvalidWalletId
	}

	if transactionID == "" {
		return nil, ErrInvalidTransactionId
	}

	if direction != LedgerDirectionDebit && direction != LedgerDirectionCredit {
		return nil, ErrInvalidLedgerDirection
	}

	if amount.Cents() <= 0 {
		return nil, ErrNegativeAmount
	}

	currencys := []string{amount.Currency(), balanceBefore.Currency(), balanceAfter.Currency()}
	allEqual := currencys[0] == currencys[1] && currencys[1] == currencys[2]

	if !allEqual {
		return nil, ErrCurrencyMismatch
	}

	isLedgerValid := false

	if direction == LedgerDirectionDebit {
		isLedgerValid = (balanceBefore.Cents() - amount.Cents()) == balanceAfter.Cents()
	} else {
		isLedgerValid = (balanceBefore.Cents() + amount.Cents()) == balanceAfter.Cents()
	}

	if !isLedgerValid {
		return nil, ErrInvalidLedgerMath
	}

	return &WalletLedgerEntry{
		id:            id,
		walletID:      walletID,
		transactionID: transactionID,
		direction:     direction,
		amount:        amount,
		balanceBefore: balanceBefore,
		balanceAfter:  balanceAfter,
		createdAt:     createdAt,
	}, nil
}

func (l WalletLedgerEntry) ID() string {
	return l.id
}

func (l WalletLedgerEntry) WalletID() string {
	return l.walletID
}

func (l WalletLedgerEntry) TransactionID() string {
	return l.transactionID
}

func (l WalletLedgerEntry) Direction() LedgerDirection {
	return l.direction
}

func (l WalletLedgerEntry) Amount() Money {
	return l.amount
}

func (l WalletLedgerEntry) BalanceBefore() Money {
	return l.balanceBefore
}

func (l WalletLedgerEntry) BalanceAfter() Money {
	return l.balanceAfter
}

func (l WalletLedgerEntry) CreatedAt() time.Time {
	return l.createdAt
}
