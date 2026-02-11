package domain

import (
	"errors"
	"testing"
)

func TestNormalizeMonthKey(t *testing.T) {
	t.Parallel()

	monthKey, err := NormalizeMonthKey(" 2026-02 ")
	if err != nil {
		t.Fatalf("normalize month key: %v", err)
	}
	if monthKey != "2026-02" {
		t.Fatalf("expected 2026-02, got %q", monthKey)
	}
}

func TestNormalizeMonthKeyRejectsInvalid(t *testing.T) {
	t.Parallel()

	_, err := NormalizeMonthKey("2026-13")
	if !errors.Is(err, ErrInvalidMonthKey) {
		t.Fatalf("expected ErrInvalidMonthKey, got %v", err)
	}
}

func TestNormalizeCapSetInput(t *testing.T) {
	t.Parallel()

	normalized, err := NormalizeCapSetInput(CapSetInput{
		MonthKey:     "2026-02",
		AmountMinor:  50000,
		CurrencyCode: " usd ",
	})
	if err != nil {
		t.Fatalf("normalize cap set input: %v", err)
	}

	if normalized.MonthKey != "2026-02" {
		t.Fatalf("expected month key 2026-02, got %q", normalized.MonthKey)
	}
	if normalized.CurrencyCode != "USD" {
		t.Fatalf("expected USD, got %q", normalized.CurrencyCode)
	}
}

func TestNormalizeCapSetInputRejectsAmount(t *testing.T) {
	t.Parallel()

	_, err := NormalizeCapSetInput(CapSetInput{
		MonthKey:     "2026-02",
		AmountMinor:  0,
		CurrencyCode: "USD",
	})
	if !errors.Is(err, ErrInvalidCapAmount) {
		t.Fatalf("expected ErrInvalidCapAmount, got %v", err)
	}
}

func TestMonthKeyFromDateTimeUTC(t *testing.T) {
	t.Parallel()

	monthKey, err := MonthKeyFromDateTimeUTC("2026-02-11T10:00:00-03:00")
	if err != nil {
		t.Fatalf("month key from datetime: %v", err)
	}

	if monthKey != "2026-02" {
		t.Fatalf("expected 2026-02, got %q", monthKey)
	}
}

func TestMonthRangeUTC(t *testing.T) {
	t.Parallel()

	startUTC, endUTC, err := MonthRangeUTC("2026-02")
	if err != nil {
		t.Fatalf("month range: %v", err)
	}

	if startUTC != "2026-02-01T00:00:00Z" {
		t.Fatalf("unexpected start UTC: %q", startUTC)
	}
	if endUTC != "2026-03-01T00:00:00Z" {
		t.Fatalf("unexpected end UTC: %q", endUTC)
	}
}
