package domain

import "time"

type Wallet struct {
	id        string
	playerID  string
	currency  string
	balance   Money
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

func NewWallet(id string, playerID string, initialBalance Money) (*Wallet, error) {
	if id == "" {
		return nil, ErrInvalidWalletId
	}

	if playerID == "" {
		return nil, ErrInvalidPlayerId
	}

	if len(initialBalance.Currency()) != 3 {
		return nil, ErrInvalidCurrency
	}

	if initialBalance.Cents() < 0 {
		return nil, ErrNegativeAmount
	}

	now := time.Now().UTC()

	return &Wallet{
		id:        id,
		playerID:  playerID,
		currency:  initialBalance.Currency(),
		balance:   initialBalance,
		version:   1,
		createdAt: now,
		updatedAt: now,
	}, nil
}

func RehydrateWallet(
	id string,
	playerID string,
	currency string,
	balance Money,
	version int64,
	createdAt time.Time,
	updatedAt time.Time,
) (*Wallet, error) {
	if id == "" {
		return nil, ErrInvalidWalletId
	}

	if playerID == "" {
		return nil, ErrInvalidPlayerId
	}

	if len(currency) != 3 {
		return nil, ErrInvalidCurrency
	}

	if balance.Currency() != currency {
		return nil, ErrWalletCurrencyMismatch
	}

	if balance.Cents() < 0 {
		return nil, ErrNegativeAmount
	}

	return &Wallet{
		id:        id,
		playerID:  playerID,
		currency:  currency,
		balance:   balance,
		version:   version,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}, nil
}

func (w Wallet) ID() string {
	return w.id
}

func (w Wallet) PlayerID() string {
	return w.playerID
}

func (w Wallet) Currency() string {
	return w.currency
}

func (w Wallet) Balance() Money {
	return w.balance
}

func (w Wallet) Version() int64 {
	return w.version
}

func (w Wallet) CreatedAt() time.Time {
	return w.createdAt
}

func (w Wallet) UpdatedAt() time.Time {
	return w.updatedAt
}

func (w *Wallet) Debit(amount Money) error {
	if amount.Currency() != w.currency {
		return ErrWalletCurrencyMismatch
	}

	if amount.Cents() <= 0 {
		return ErrNegativeAmount
	}

	if w.balance.Cents() < amount.Cents() {
		return ErrInsufficientBalance
	}

	newBalance, err := w.balance.Subtract(amount)

	if err != nil {
		return err
	}

	w.balance = newBalance
	w.version += 1
	w.updatedAt = time.Now().UTC()

	return nil
}

func (w *Wallet) Credit(amount Money) error {
	if amount.Currency() != w.currency {
		return ErrWalletCurrencyMismatch
	}

	if amount.Cents() <= 0 {
		return ErrNegativeAmount
	}

	balance, err := w.balance.Add(amount)

	if err != nil {
		return err
	}

	w.balance = balance
	w.version += 1
	w.updatedAt = time.Now().UTC()

	return nil
}
