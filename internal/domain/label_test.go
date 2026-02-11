package domain

import (
	"errors"
	"testing"
)

func TestNormalizeLabelName(t *testing.T) {
	t.Parallel()

	name, err := NormalizeLabelName("  groceries  ")
	if err != nil {
		t.Fatalf("normalize name: %v", err)
	}
	if name != "groceries" {
		t.Fatalf("expected groceries, got %q", name)
	}
}

func TestNormalizeLabelNameRejectsEmpty(t *testing.T) {
	t.Parallel()

	_, err := NormalizeLabelName("   ")
	if !errors.Is(err, ErrInvalidLabelName) {
		t.Fatalf("expected ErrInvalidLabelName, got %v", err)
	}
}

func TestValidateLabelID(t *testing.T) {
	t.Parallel()

	if err := ValidateLabelID(1); err != nil {
		t.Fatalf("expected valid id 1, got %v", err)
	}

	if err := ValidateLabelID(0); !errors.Is(err, ErrInvalidLabelID) {
		t.Fatalf("expected ErrInvalidLabelID for 0, got %v", err)
	}
}
