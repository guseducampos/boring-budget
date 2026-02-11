package domain

import (
	"errors"
	"testing"
)

func TestBuildReportPeriodPresetScopes(t *testing.T) {
	t.Parallel()

	monthly, err := BuildReportPeriod(ReportPeriodInput{
		Scope:    ReportScopeMonthly,
		MonthKey: "2026-02",
	})
	if err != nil {
		t.Fatalf("build monthly period: %v", err)
	}
	if monthly.FromUTC != "2026-02-01T00:00:00Z" {
		t.Fatalf("unexpected monthly from_utc: %q", monthly.FromUTC)
	}
	if monthly.ToUTC != "2026-02-28T23:59:59.999999999Z" {
		t.Fatalf("unexpected monthly to_utc: %q", monthly.ToUTC)
	}

	bimonthly, err := BuildReportPeriod(ReportPeriodInput{
		Scope:    ReportScopeBimonthly,
		MonthKey: "2026-11",
	})
	if err != nil {
		t.Fatalf("build bimonthly period: %v", err)
	}
	if bimonthly.FromUTC != "2026-11-01T00:00:00Z" {
		t.Fatalf("unexpected bimonthly from_utc: %q", bimonthly.FromUTC)
	}
	if bimonthly.ToUTC != "2026-12-31T23:59:59.999999999Z" {
		t.Fatalf("unexpected bimonthly to_utc: %q", bimonthly.ToUTC)
	}

	quarterly, err := BuildReportPeriod(ReportPeriodInput{
		Scope:    ReportScopeQuarterly,
		MonthKey: "2026-11",
	})
	if err != nil {
		t.Fatalf("build quarterly period: %v", err)
	}
	if quarterly.FromUTC != "2026-11-01T00:00:00Z" {
		t.Fatalf("unexpected quarterly from_utc: %q", quarterly.FromUTC)
	}
	if quarterly.ToUTC != "2027-01-31T23:59:59.999999999Z" {
		t.Fatalf("unexpected quarterly to_utc: %q", quarterly.ToUTC)
	}
}

func TestBuildReportPeriodRangeDateOnlyBounds(t *testing.T) {
	t.Parallel()

	period, err := BuildReportPeriod(ReportPeriodInput{
		Scope:       ReportScopeRange,
		DateFromUTC: "2026-02-10",
		DateToUTC:   "2026-02-11",
	})
	if err != nil {
		t.Fatalf("build range period: %v", err)
	}

	if period.FromUTC != "2026-02-10T00:00:00Z" {
		t.Fatalf("unexpected range from_utc: %q", period.FromUTC)
	}
	if period.ToUTC != "2026-02-11T23:59:59.999999999Z" {
		t.Fatalf("unexpected range to_utc: %q", period.ToUTC)
	}
}

func TestBuildReportPeriodRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	_, err := BuildReportPeriod(ReportPeriodInput{
		Scope: "yearly",
	})
	if !errors.Is(err, ErrInvalidReportScope) {
		t.Fatalf("expected ErrInvalidReportScope, got %v", err)
	}

	_, err = BuildReportPeriod(ReportPeriodInput{
		Scope:       ReportScopeRange,
		DateFromUTC: "2026-02-11T00:00:00Z",
		DateToUTC:   "2026-02-10T00:00:00Z",
	})
	if !errors.Is(err, ErrInvalidReportPeriod) {
		t.Fatalf("expected ErrInvalidReportPeriod, got %v", err)
	}
}

func TestMonthKeysInPeriod(t *testing.T) {
	t.Parallel()

	months, err := MonthKeysInPeriod("2026-02-10T00:00:00Z", "2026-04-01T00:00:00Z")
	if err != nil {
		t.Fatalf("month keys in period: %v", err)
	}

	expected := []string{"2026-02", "2026-03", "2026-04"}
	if len(months) != len(expected) {
		t.Fatalf("expected %d months, got %d (%v)", len(expected), len(months), months)
	}
	for i := range expected {
		if months[i] != expected[i] {
			t.Fatalf("expected month %q at index %d, got %q", expected[i], i, months[i])
		}
	}
}

func TestPeriodKeyForTransaction(t *testing.T) {
	t.Parallel()

	key, err := PeriodKeyForTransaction("2026-02-11T10:00:00Z", ReportGroupingDay)
	if err != nil {
		t.Fatalf("period key day: %v", err)
	}
	if key != "2026-02-11" {
		t.Fatalf("unexpected day key %q", key)
	}

	key, err = PeriodKeyForTransaction("2026-02-11T10:00:00Z", ReportGroupingWeek)
	if err != nil {
		t.Fatalf("period key week: %v", err)
	}
	if key != "2026-W07" {
		t.Fatalf("unexpected week key %q", key)
	}

	key, err = PeriodKeyForTransaction("2026-02-11T10:00:00Z", ReportGroupingMonth)
	if err != nil {
		t.Fatalf("period key month: %v", err)
	}
	if key != "2026-02" {
		t.Fatalf("unexpected month key %q", key)
	}
}
