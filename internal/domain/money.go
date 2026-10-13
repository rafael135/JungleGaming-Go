package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Money struct {
	amountCents int64
	currency    string
}

func NewMoney(amount int64, currency string) (Money, error) {
	if len(currency) != 3 {
		return Money{}, ErrInvalidCurrency
	}

	return Money{
		amountCents: amount,
		currency:    currency,
	}, nil
}

func ParseMoney(amountStr string, currency string) (Money, error) {
	if len(currency) != 3 {
		return Money{}, ErrInvalidCurrency
	}

	trimmedAmountStr := strings.TrimSpace(amountStr)

	if len(trimmedAmountStr) == 0 {
		return Money{}, ErrInvalidAmountFormat
	}

	if trimmedAmountStr[0] == '-' {
		return Money{}, ErrNegativeAmount
	}

	parts := strings.Split(trimmedAmountStr, ".")
	if len(parts) != 2 {
		return Money{}, ErrInvalidAmountFormat
	}

	if len(parts[1]) != 2 {
		return Money{}, ErrInvalidScale
	}

	integers, err := strconv.ParseInt(parts[0], 10, 64)

	if integers > math.MaxInt64/100 {
		return Money{}, ErrAmountOverflow
	}

	if err != nil {
		return Money{}, ErrInvalidAmountFormat
	}

	cents, err := strconv.ParseInt(parts[1], 10, 64)

	if err != nil {
		return Money{}, ErrInvalidAmountFormat
	}

	amount := (integers * 100) + (cents)

	return NewMoney(amount, currency)
}

func ZeroMoney(currency string) (Money, error) {
	if len(currency) != 3 {
		return Money{}, ErrInvalidCurrency
	}

	return NewMoney(0, currency)
}

func (m Money) Cents() int64 {
	return m.amountCents
}

func (m Money) Currency() string {
	return m.currency
}

func (m Money) Compare(other Money) (int, error) {
	if m.currency != other.currency {
		return 0, ErrCurrencyMismatch
	}

	if m.amountCents < other.amountCents {
		return -1, nil
	}

	if m.amountCents > other.amountCents {
		return 1, nil
	}

	return 0, nil
}

func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}

	if other.amountCents > 0 && m.amountCents > math.MaxInt64-other.amountCents {
		return Money{}, ErrAmountOverflow
	}

	if other.amountCents < 0 && m.amountCents < math.MinInt64-other.amountCents {
		return Money{}, ErrAmountOverflow
	}

	result := m.amountCents + other.amountCents

	return NewMoney(result, m.currency)
}

func (m Money) Subtract(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}

	if other.amountCents > 0 && m.amountCents < math.MinInt64+other.amountCents {
		return Money{}, ErrAmountOverflow
	}

	if other.amountCents < 0 && m.amountCents > math.MaxInt64+other.amountCents {
		return Money{}, ErrAmountOverflow
	}

	result := m.amountCents - other.amountCents

	return NewMoney(result, m.currency)
}

func (m Money) Negate() (Money, error) {
	if m.amountCents == math.MinInt64 {
		return Money{}, ErrAmountOverflow
	}

	return Money{
		amountCents: -m.amountCents,
		currency:    m.currency,
	}, nil
}

func (m Money) String() string {
	integer := m.amountCents / 100
	cents := m.amountCents % 100
	sign := ""

	if m.amountCents < 0 {
		sign = "-"
	}

	if integer < 0 {
		sign = "-"
		integer = -integer
	}

	if cents < 0 {
		sign = "-"
		cents = -cents
	}

	return fmt.Sprintf("%s%d.%02d", sign, integer, cents)
}
