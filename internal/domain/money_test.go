package domain

import (
	"errors"
	"math"
	"testing"
)

func TestParseMoney(t *testing.T) {
	tests := []struct {
		name          string
		amountStr     string
		currency      string
		expectedCents int64
		expectedErr   error
	}{
		{
			name:          "deve rejeitar se a moeda for inválida",
			amountStr:     "25.00",
			currency:      "X",
			expectedCents: 0,
			expectedErr:   ErrInvalidCurrency,
		},
		{
			name:          "deve rejeitar se o valor for inválido",
			amountStr:     "",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   ErrInvalidAmountFormat,
		},
		{
			name:          "deve rejeitar se o valor for negativo",
			amountStr:     "-25.00",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   ErrNegativeAmount,
		},
		{
			name:          "deve rejeitar se a escala for diferente de 2 casas",
			amountStr:     "25.000",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   ErrInvalidScale,
		},
		{
			name:          "deve retornar valor correto",
			amountStr:     "25.00",
			currency:      "BRL",
			expectedCents: 2500,
			expectedErr:   nil,
		},
		{
			name:          "deve rejeitar letras no valor",
			amountStr:     "aaaa",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   ErrInvalidAmountFormat,
		},
		{
			name:          "deve rejeitar 1 casa decimal",
			amountStr:     "5.0",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   ErrInvalidScale,
		},
		{
			name:          "deve rejeitar sem casa decimal",
			amountStr:     "8",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   ErrInvalidAmountFormat,
		},
		{
			name:          "deve rejeitar multiplos pontos",
			amountStr:     "50.00.00",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   ErrInvalidAmountFormat,
		},
		{
			name:          "deve aceitar espacos no valor",
			amountStr:     "    50.00 ",
			currency:      "BRL",
			expectedCents: 5000,
			expectedErr:   nil,
		},
		{
			name:          "deve aceitar valor zero",
			amountStr:     "0.00",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   nil,
		},
		{
			name:          "deve rejeitar zero com sinal negativo",
			amountStr:     "-0.00",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   ErrNegativeAmount,
		},
		{
			name:          "deve rejeitar notação cientifica",
			amountStr:     "1e5",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   ErrInvalidAmountFormat,
		},
		{
			name:          "deve rejeitar NaN",
			amountStr:     "NaN",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   ErrInvalidAmountFormat,
		},
		{
			name:          "deve rejeitar Infinity",
			amountStr:     "Infinity",
			currency:      "BRL",
			expectedCents: 0,
			expectedErr:   ErrInvalidAmountFormat,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseMoney(tc.amountStr, tc.currency)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result.Cents() != tc.expectedCents {
					t.Errorf("esperava %d centavos, mas recebeu %d", tc.expectedCents, result.Cents())
				}

				if result.Currency() != tc.currency {
					t.Errorf("esperava moeda %s, mas recebeu %s", tc.currency, result.Currency())
				}
			}
		})
	}
}

func TestMoney_Add(t *testing.T) {
	tests := []struct {
		name          string
		money1        Money
		money2        Money
		expectedCents int64
		expectedErr   error
	}{
		{
			name:          "deve rejeitar se as moedas forem diferentes",
			money1:        Money{amountCents: 2500, currency: "BRL"},
			money2:        Money{amountCents: 2500, currency: "USD"},
			expectedCents: 0,
			expectedErr:   ErrCurrencyMismatch,
		},
		{
			name:          "deve retornar valor correto",
			money1:        Money{amountCents: 2500, currency: "BRL"},
			money2:        Money{amountCents: 2500, currency: "BRL"},
			expectedCents: 5000,
			expectedErr:   nil,
		},
		{
			name: "deve rejeitar se ocorrer overflow",
			money1: Money{
				amountCents: math.MaxInt64,
				currency:    "BRL",
			},
			money2:        Money{amountCents: 2500, currency: "BRL"},
			expectedCents: 0,
			expectedErr:   ErrAmountOverflow,
		},
		{
			name: "deve rejeitar se ocorrer underflow",
			money1: Money{
				amountCents: math.MinInt64,
				currency:    "BRL",
			},
			money2: Money{
				amountCents: -2500,
				currency:    "BRL",
			},
			expectedCents: 0,
			expectedErr:   ErrAmountOverflow,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.money1.Add(tc.money2)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result.Cents() != tc.expectedCents {
					t.Errorf("esperava %d centavos, mas recebeu %d", tc.expectedCents, result.Cents())
				}
			}
		})
	}
}

func TestMoney_Subtract(t *testing.T) {
	tests := []struct {
		name          string
		money1        Money
		money2        Money
		expectedCents int64
		expectedErr   error
	}{
		{
			name:          "deve rejeitar se as moedas forem diferentes",
			money1:        Money{amountCents: 2500, currency: "BRL"},
			money2:        Money{amountCents: 2500, currency: "USD"},
			expectedCents: 0,
			expectedErr:   ErrCurrencyMismatch,
		},
		{
			name:          "deve retornar valor correto",
			money1:        Money{amountCents: 2500, currency: "BRL"},
			money2:        Money{amountCents: 2500, currency: "BRL"},
			expectedCents: 0,
			expectedErr:   nil,
		},
		{
			name: "deve rejeitar se ocorrer overflow",
			money1: Money{
				amountCents: math.MinInt64,
				currency:    "BRL",
			},
			money2:        Money{amountCents: 2500, currency: "BRL"},
			expectedCents: 0,
			expectedErr:   ErrAmountOverflow,
		},
		{
			name: "deve retornar valor correto",
			money1: Money{
				amountCents: 1000,
				currency:    "BRL",
			},
			money2: Money{
				amountCents: 2500,
				currency:    "BRL",
			},
			expectedCents: -1500,
			expectedErr:   nil,
		},
		{
			name: "deve rejeitar se ocorrer underflow",
			money1: Money{
				amountCents: math.MaxInt64,
				currency:    "BRL",
			},
			money2: Money{
				amountCents: -2500,
				currency:    "BRL",
			},
			expectedCents: 0,
			expectedErr:   ErrAmountOverflow,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.money1.Subtract(tc.money2)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result.Cents() != tc.expectedCents {
					t.Errorf("esperava %d centavos, mas recebeu %d", tc.expectedCents, result.Cents())
				}
			}
		})
	}
}

func TestMoney_Compare(t *testing.T) {
	tests := []struct {
		name        string
		money1      Money
		money2      Money
		expected    int
		expectedErr error
	}{
		{
			name:        "deve retornar 0 se as moedas forem iguais",
			money1:      Money{amountCents: 2500, currency: "BRL"},
			money2:      Money{amountCents: 2500, currency: "BRL"},
			expected:    0,
			expectedErr: nil,
		},
		{
			name:        "deve retornar 1 se a moeda 1 for maior",
			money1:      Money{amountCents: 2500, currency: "BRL"},
			money2:      Money{amountCents: 1000, currency: "BRL"},
			expected:    1,
			expectedErr: nil,
		},
		{
			name:        "deve retornar -1 se a moeda 1 for menor",
			money1:      Money{amountCents: 1000, currency: "BRL"},
			money2:      Money{amountCents: 2500, currency: "BRL"},
			expected:    -1,
			expectedErr: nil,
		},
		{
			name:        "deve rejeitar se as moedas forem diferentes",
			money1:      Money{amountCents: 2500, currency: "BRL"},
			money2:      Money{amountCents: 2500, currency: "USD"},
			expected:    0,
			expectedErr: ErrCurrencyMismatch,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.money1.Compare(tc.money2)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result != tc.expected {
					t.Errorf("esperava %d, mas recebeu %d", tc.expected, result)
				}
			}
		})
	}
}

func TestMoney_String(t *testing.T) {
	tests := []struct {
		name     string
		money    Money
		expected string
	}{
		{
			name:     "deve retornar string formatada",
			money:    Money{amountCents: 2500, currency: "BRL"},
			expected: "25.00",
		},
		{
			name:     "deve retornar string formatada com negativo",
			money:    Money{amountCents: -2500, currency: "BRL"},
			expected: "-25.00",
		},
		{
			name:     "deve retornar string formatada com centavos",
			money:    Money{amountCents: 2550, currency: "BRL"},
			expected: "25.50",
		},
		{
			name:     "deve retornar string formatada com centavos e negativo",
			money:    Money{amountCents: -2550, currency: "BRL"},
			expected: "-25.50",
		},
		{
			name: "deve retornar string formatado com apenas centavos",
			money: Money{
				amountCents: 50,
				currency:    "BRL",
			},
			expected: "0.50",
		},
		{
			name: "deve retornar string formatada com apenas centavos negativo",
			money: Money{
				amountCents: -50,
				currency:    "BRL",
			},
			expected: "-0.50",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.money.String() != tc.expected {
				t.Errorf("esperava %s, mas recebeu %s", tc.expected, tc.money.String())
			}
		})
	}

}

func TestMoney_Negate(t *testing.T) {
	tests := []struct {
		name        string
		money       Money
		expected    Money
		expectedErr error
	}{
		{
			name: "deve negativar o valor positivo",
			money: Money{
				amountCents: 2500,
				currency:    "BRL",
			},
			expected: Money{
				amountCents: -2500,
				currency:    "BRL",
			},
			expectedErr: nil,
		},
		{
			name: "deve negativar valor negativo",
			money: Money{
				amountCents: -2500,
				currency:    "BRL",
			},
			expected: Money{
				amountCents: 2500,
				currency:    "BRL",
			},
			expectedErr: nil,
		},
		{
			name: "deve manter 0",
			money: Money{
				amountCents: 0,
				currency:    "BRL",
			},
			expected: Money{
				amountCents: 0,
				currency:    "BRL",
			},
			expectedErr: nil,
		},
		{
			name: "deve rejeitar se ocorrer overflow",
			money: Money{
				amountCents: math.MinInt64,
				currency:    "BRL",
			},
			expected:    Money{},
			expectedErr: ErrAmountOverflow,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.money.Negate()

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result != tc.expected {
					t.Errorf("esperava %v, mas recebeu %v", tc.expected, result)
				}
			}
		})
	}
}

func TestZeroMoney(t *testing.T) {
	tests := []struct {
		name        string
		currency    string
		expected    Money
		expectedErr error
	}{
		{
			name:     "deve criar um zero money",
			currency: "BRL",
			expected: Money{
				amountCents: 0,
				currency:    "BRL",
			},
			expectedErr: nil,
		},
		{
			name:        "deve rejeitar se moeda for inválida",
			currency:    "BR",
			expected:    Money{},
			expectedErr: ErrInvalidCurrency,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ZeroMoney(tc.currency)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result != tc.expected {
					t.Errorf("esperava %v, mas recebeu %v", tc.expected, result)
				}
			}
		})
	}
}

func TestNewMoney(t *testing.T) {
	tests := []struct {
		name        string
		amountCents int64
		currency    string
		expected    Money
		expectedErr error
	}{
		{
			name:        "deve criar um money",
			amountCents: 2500,
			currency:    "BRL",
			expected: Money{
				amountCents: 2500,
				currency:    "BRL",
			},
			expectedErr: nil,
		},
		{
			name:        "deve rejeitar se moeda for inválida",
			amountCents: 2500,
			currency:    "BR",
			expected:    Money{},
			expectedErr: ErrInvalidCurrency,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewMoney(tc.amountCents, tc.currency)

			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("esperava erro %v, mas recebeu %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if result != tc.expected {
					t.Errorf("esperava %v, mas recebeu %v", tc.expected, result)
				}
			}
		})
	}
}
