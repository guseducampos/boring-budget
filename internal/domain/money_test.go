package domain

import (
	"errors"
	"testing"
)

func TestCurrencyMinorUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		currency string
		want     int
	}{
		{name: "default_two_decimals", currency: "USD", want: 2},
		{name: "zero_decimal_currency", currency: "JPY", want: 0},
		{name: "three_decimal_currency", currency: "BHD", want: 3},
		{name: "four_decimal_currency", currency: "CLF", want: 4},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := CurrencyMinorUnit(tc.currency)
			if err != nil {
				t.Fatalf("CurrencyMinorUnit(%q): %v", tc.currency, err)
			}
			if got != tc.want {
				t.Fatalf("expected minor unit %d, got %d", tc.want, got)
			}
		})
	}
}

func TestParseMajorAmountToMinor(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		amount   string
		currency string
		want     int64
	}{
		{name: "usd_with_decimals", amount: "74.25", currency: "USD", want: 7425},
		{name: "usd_without_decimals", amount: "74", currency: "USD", want: 7400},
		{name: "jpy_no_minor", amount: "500", currency: "JPY", want: 500},
		{name: "jpy_with_zero_fraction", amount: "500.0", currency: "JPY", want: 500},
		{name: "three_decimals", amount: "1.234", currency: "BHD", want: 1234},
		{name: "four_decimals", amount: "1.2345", currency: "CLF", want: 12345},
		{name: "leading_dot", amount: ".5", currency: "USD", want: 50},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMajorAmountToMinor(tc.amount, tc.currency)
			if err != nil {
				t.Fatalf("ParseMajorAmountToMinor(%q, %q): %v", tc.amount, tc.currency, err)
			}
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestParseMajorAmountToMinorRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		amount   string
		currency string
		err      error
	}{
		{name: "invalid_format", amount: "abc", currency: "USD", err: ErrInvalidAmount},
		{name: "missing_digits", amount: ".", currency: "USD", err: ErrInvalidAmount},
		{name: "trailing_decimal_point", amount: "1.", currency: "USD", err: ErrInvalidAmount},
		{name: "negative_amount", amount: "-1.00", currency: "USD", err: ErrInvalidAmount},
		{name: "too_many_decimals", amount: "1.001", currency: "USD", err: ErrInvalidAmountPrecision},
		{name: "invalid_zero_decimal_precision", amount: "1.5", currency: "JPY", err: ErrInvalidAmountPrecision},
		{name: "overflow", amount: "92233720368547759", currency: "USD", err: ErrAmountOverflow},
		{name: "invalid_currency", amount: "1.00", currency: "US", err: ErrInvalidCurrencyCode},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseMajorAmountToMinor(tc.amount, tc.currency)
			if !errors.Is(err, tc.err) {
				t.Fatalf("expected %v, got %v", tc.err, err)
			}
		})
	}
}
