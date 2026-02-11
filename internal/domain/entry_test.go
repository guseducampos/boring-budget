package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestNormalizeEntryType(t *testing.T) {
	t.Parallel()

	entryType, err := NormalizeEntryType("  ExPenSe  ")
	if err != nil {
		t.Fatalf("normalize entry type: %v", err)
	}
	if entryType != EntryTypeExpense {
		t.Fatalf("expected expense, got %q", entryType)
	}
}

func TestNormalizeEntryTypeRejectsInvalid(t *testing.T) {
	t.Parallel()

	_, err := NormalizeEntryType("transfer")
	if !errors.Is(err, ErrInvalidEntryType) {
		t.Fatalf("expected ErrInvalidEntryType, got %v", err)
	}
}

func TestValidateAmountMinor(t *testing.T) {
	t.Parallel()

	if err := ValidateAmountMinor(1); err != nil {
		t.Fatalf("expected amount 1 to be valid, got %v", err)
	}
	if err := ValidateAmountMinor(0); !errors.Is(err, ErrInvalidAmountMinor) {
		t.Fatalf("expected ErrInvalidAmountMinor for 0, got %v", err)
	}
}

func TestNormalizeCurrencyCode(t *testing.T) {
	t.Parallel()

	currency, err := NormalizeCurrencyCode(" usd ")
	if err != nil {
		t.Fatalf("normalize currency: %v", err)
	}
	if currency != "USD" {
		t.Fatalf("expected USD, got %q", currency)
	}
}

func TestNormalizeCurrencyCodeRejectsInvalid(t *testing.T) {
	t.Parallel()

	_, err := NormalizeCurrencyCode("US1")
	if !errors.Is(err, ErrInvalidCurrencyCode) {
		t.Fatalf("expected ErrInvalidCurrencyCode, got %v", err)
	}
}

func TestNormalizeTransactionDateUTC(t *testing.T) {
	t.Parallel()

	normalized, err := NormalizeTransactionDateUTC("2026-02-11")
	if err != nil {
		t.Fatalf("normalize date: %v", err)
	}
	if normalized != "2026-02-11T00:00:00Z" {
		t.Fatalf("unexpected normalized date: %q", normalized)
	}
}

func TestNormalizeTransactionDateUTCRejectsInvalid(t *testing.T) {
	t.Parallel()

	_, err := NormalizeTransactionDateUTC("02/11/2026")
	if !errors.Is(err, ErrInvalidTransactionDate) {
		t.Fatalf("expected ErrInvalidTransactionDate, got %v", err)
	}
}

func TestValidateEntryID(t *testing.T) {
	t.Parallel()

	if err := ValidateEntryID(10); err != nil {
		t.Fatalf("expected valid id, got %v", err)
	}
	if err := ValidateEntryID(0); !errors.Is(err, ErrInvalidEntryID) {
		t.Fatalf("expected ErrInvalidEntryID, got %v", err)
	}
}

func TestValidateOptionalCategoryID(t *testing.T) {
	t.Parallel()

	if err := ValidateOptionalCategoryID(nil); err != nil {
		t.Fatalf("nil category should be valid, got %v", err)
	}

	categoryID := int64(3)
	if err := ValidateOptionalCategoryID(&categoryID); err != nil {
		t.Fatalf("expected category id 3 valid, got %v", err)
	}

	invalidID := int64(0)
	if err := ValidateOptionalCategoryID(&invalidID); !errors.Is(err, ErrInvalidCategoryID) {
		t.Fatalf("expected ErrInvalidCategoryID, got %v", err)
	}
}

func TestNormalizeLabelIDs(t *testing.T) {
	t.Parallel()

	normalized, err := NormalizeLabelIDs([]int64{3, 2, 3, 1})
	if err != nil {
		t.Fatalf("normalize label ids: %v", err)
	}

	expected := []int64{1, 2, 3}
	if !reflect.DeepEqual(expected, normalized) {
		t.Fatalf("expected %v, got %v", expected, normalized)
	}
}

func TestNormalizeLabelIDsRejectsInvalid(t *testing.T) {
	t.Parallel()

	_, err := NormalizeLabelIDs([]int64{1, 0})
	if !errors.Is(err, ErrInvalidLabelID) {
		t.Fatalf("expected ErrInvalidLabelID, got %v", err)
	}
}

func TestNormalizeLabelMode(t *testing.T) {
	t.Parallel()

	mode, err := NormalizeLabelMode(" all ")
	if err != nil {
		t.Fatalf("normalize label mode: %v", err)
	}
	if mode != LabelFilterModeAll {
		t.Fatalf("expected ALL, got %q", mode)
	}

	defaultMode, err := NormalizeLabelMode("")
	if err != nil {
		t.Fatalf("normalize empty mode: %v", err)
	}
	if defaultMode != LabelFilterModeAny {
		t.Fatalf("expected default ANY, got %q", defaultMode)
	}
}

func TestNormalizeLabelModeRejectsInvalid(t *testing.T) {
	t.Parallel()

	_, err := NormalizeLabelMode("partial")
	if !errors.Is(err, ErrInvalidLabelMode) {
		t.Fatalf("expected ErrInvalidLabelMode, got %v", err)
	}
}

func TestNormalizeOptionalTransactionDateUTC(t *testing.T) {
	t.Parallel()

	empty, err := NormalizeOptionalTransactionDateUTC("   ")
	if err != nil {
		t.Fatalf("normalize optional empty date: %v", err)
	}
	if empty != "" {
		t.Fatalf("expected empty normalized optional date, got %q", empty)
	}
}

func TestValidateDateRange(t *testing.T) {
	t.Parallel()

	if err := ValidateDateRange("2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z"); err != nil {
		t.Fatalf("expected valid range, got %v", err)
	}

	if err := ValidateDateRange("2026-02-01T00:00:00Z", "2026-01-31T23:59:59Z"); !errors.Is(err, ErrInvalidDateRange) {
		t.Fatalf("expected ErrInvalidDateRange, got %v", err)
	}
}
