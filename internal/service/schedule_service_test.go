package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"boring-budget/internal/domain"
)

type scheduleRepoStub struct {
	addFn                  func(ctx context.Context, input domain.ScheduledPaymentAddInput) (domain.ScheduledPayment, error)
	listFn                 func(ctx context.Context, includeDeleted bool) ([]domain.ScheduledPayment, error)
	getActiveByIDFn        func(ctx context.Context, id int64) (domain.ScheduledPayment, error)
	deleteFn               func(ctx context.Context, id int64) (domain.ScheduledPaymentDeleteResult, error)
	listExecutionsByIDFn   func(ctx context.Context, scheduleID int64) ([]domain.ScheduledPaymentExecution, error)
	claimExecutionFn       func(ctx context.Context, scheduleID int64, monthKey, createdAtUTC string) (bool, error)
	attachExecutionEntryFn func(ctx context.Context, scheduleID int64, monthKey string, entryID int64) error
}

func (s *scheduleRepoStub) Add(ctx context.Context, input domain.ScheduledPaymentAddInput) (domain.ScheduledPayment, error) {
	return s.addFn(ctx, input)
}

func (s *scheduleRepoStub) List(ctx context.Context, includeDeleted bool) ([]domain.ScheduledPayment, error) {
	return s.listFn(ctx, includeDeleted)
}

func (s *scheduleRepoStub) GetActiveByID(ctx context.Context, id int64) (domain.ScheduledPayment, error) {
	return s.getActiveByIDFn(ctx, id)
}

func (s *scheduleRepoStub) Delete(ctx context.Context, id int64) (domain.ScheduledPaymentDeleteResult, error) {
	return s.deleteFn(ctx, id)
}

func (s *scheduleRepoStub) ListExecutionsByScheduleID(ctx context.Context, scheduleID int64) ([]domain.ScheduledPaymentExecution, error) {
	return s.listExecutionsByIDFn(ctx, scheduleID)
}

func (s *scheduleRepoStub) ClaimExecution(ctx context.Context, scheduleID int64, monthKey, createdAtUTC string) (bool, error) {
	return s.claimExecutionFn(ctx, scheduleID, monthKey, createdAtUTC)
}

func (s *scheduleRepoStub) AttachExecutionEntry(ctx context.Context, scheduleID int64, monthKey string, entryID int64) error {
	return s.attachExecutionEntryFn(ctx, scheduleID, monthKey, entryID)
}

type scheduleEntryCreatorStub struct {
	addFn func(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error)
}

func (s *scheduleEntryCreatorStub) Add(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error) {
	return s.addFn(ctx, input)
}

func TestNewScheduleServiceRequiresRepo(t *testing.T) {
	t.Parallel()

	_, err := NewScheduleService(nil, nil)
	if err == nil {
		t.Fatalf("expected error for nil repo")
	}
}

func TestScheduleServiceAddNormalizesInput(t *testing.T) {
	t.Parallel()

	var received domain.ScheduledPaymentAddInput
	svc, err := NewScheduleService(&scheduleRepoStub{
		addFn: func(ctx context.Context, input domain.ScheduledPaymentAddInput) (domain.ScheduledPayment, error) {
			received = input
			return domain.ScheduledPayment{ID: 1, Name: input.Name}, nil
		},
		listFn: func(context.Context, bool) ([]domain.ScheduledPayment, error) {
			return nil, nil
		},
		getActiveByIDFn: func(context.Context, int64) (domain.ScheduledPayment, error) {
			return domain.ScheduledPayment{}, nil
		},
		deleteFn: func(context.Context, int64) (domain.ScheduledPaymentDeleteResult, error) {
			return domain.ScheduledPaymentDeleteResult{}, nil
		},
		listExecutionsByIDFn: func(context.Context, int64) ([]domain.ScheduledPaymentExecution, error) {
			return nil, nil
		},
		claimExecutionFn: func(context.Context, int64, string, string) (bool, error) {
			return false, nil
		},
		attachExecutionEntryFn: func(context.Context, int64, string, int64) error {
			return nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("new schedule service: %v", err)
	}

	categoryID := int64(9)
	_, err = svc.Add(context.Background(), domain.ScheduledPaymentAddInput{
		Name:          "  Rent  ",
		AmountMinor:   125000,
		CurrencyCode:  " usd ",
		DayOfMonth:    10,
		StartMonthKey: " 2026-01 ",
		CategoryID:    &categoryID,
		Note:          "  fixed payment  ",
	})
	if err != nil {
		t.Fatalf("add schedule: %v", err)
	}

	if received.Name != "Rent" {
		t.Fatalf("expected normalized name Rent, got %q", received.Name)
	}
	if received.CurrencyCode != "USD" {
		t.Fatalf("expected normalized currency USD, got %q", received.CurrencyCode)
	}
	if received.StartMonthKey != "2026-01" {
		t.Fatalf("expected start month 2026-01, got %q", received.StartMonthKey)
	}
	if received.Note != "fixed payment" {
		t.Fatalf("expected trimmed note, got %q", received.Note)
	}
}

func TestScheduleServiceRunIsIdempotentPerScheduleMonth(t *testing.T) {
	t.Parallel()

	schedule := domain.ScheduledPayment{
		ID:            41,
		Name:          "Rent",
		AmountMinor:   120000,
		CurrencyCode:  "USD",
		DayOfMonth:    15,
		StartMonthKey: "2026-01",
	}

	entryIDJanuary := int64(9001)
	executionState := map[string]*int64{
		"2026-01": &entryIDJanuary,
	}
	createdEntryInputs := make([]domain.EntryAddInput, 0)
	nextEntryID := int64(1)

	repo := &scheduleRepoStub{
		addFn: func(context.Context, domain.ScheduledPaymentAddInput) (domain.ScheduledPayment, error) {
			return domain.ScheduledPayment{}, nil
		},
		listFn: func(context.Context, bool) ([]domain.ScheduledPayment, error) {
			return []domain.ScheduledPayment{schedule}, nil
		},
		getActiveByIDFn: func(context.Context, int64) (domain.ScheduledPayment, error) {
			return schedule, nil
		},
		deleteFn: func(context.Context, int64) (domain.ScheduledPaymentDeleteResult, error) {
			return domain.ScheduledPaymentDeleteResult{}, nil
		},
		listExecutionsByIDFn: func(context.Context, int64) ([]domain.ScheduledPaymentExecution, error) {
			executions := make([]domain.ScheduledPaymentExecution, 0, len(executionState))
			for monthKey, entryID := range executionState {
				execution := domain.ScheduledPaymentExecution{
					ScheduleID: schedule.ID,
					MonthKey:   monthKey,
				}
				if entryID != nil {
					value := *entryID
					execution.EntryID = &value
				}
				executions = append(executions, execution)
			}
			return executions, nil
		},
		claimExecutionFn: func(_ context.Context, _ int64, monthKey string, _ string) (bool, error) {
			// Unique execution by (schedule, month).
			if _, exists := executionState[monthKey]; exists {
				return false, nil
			}
			executionState[monthKey] = nil
			return true, nil
		},
		attachExecutionEntryFn: func(_ context.Context, _ int64, monthKey string, entryID int64) error {
			value := entryID
			executionState[monthKey] = &value
			return nil
		},
	}

	entryCreator := &scheduleEntryCreatorStub{
		addFn: func(_ context.Context, input domain.EntryAddInput) (domain.Entry, error) {
			createdEntryInputs = append(createdEntryInputs, input)
			entry := domain.Entry{ID: nextEntryID}
			nextEntryID++
			return entry, nil
		},
	}

	svc, err := NewScheduleService(repo, entryCreator)
	if err != nil {
		t.Fatalf("new schedule service: %v", err)
	}

	firstRun, err := svc.Run(context.Background(), domain.ScheduledPaymentRunInput{
		ThroughDateUTC: "2026-03-20",
	})
	if err != nil {
		t.Fatalf("run schedules first time: %v", err)
	}
	if firstRun.CreatedCount != 2 || firstRun.SkippedCount != 1 {
		t.Fatalf("expected created=2 skipped=1, got created=%d skipped=%d", firstRun.CreatedCount, firstRun.SkippedCount)
	}

	expectedDates := []string{"2026-02-15T00:00:00Z", "2026-03-15T00:00:00Z"}
	actualDates := []string{createdEntryInputs[0].TransactionDateUTC, createdEntryInputs[1].TransactionDateUTC}
	if !reflect.DeepEqual(expectedDates, actualDates) {
		t.Fatalf("unexpected created entry dates: want=%v got=%v", expectedDates, actualDates)
	}

	secondRun, err := svc.Run(context.Background(), domain.ScheduledPaymentRunInput{
		ThroughDateUTC: "2026-03-20",
	})
	if err != nil {
		t.Fatalf("run schedules second time: %v", err)
	}
	if secondRun.CreatedCount != 0 || secondRun.SkippedCount != 3 {
		t.Fatalf("expected created=0 skipped=3 on rerun, got created=%d skipped=%d", secondRun.CreatedCount, secondRun.SkippedCount)
	}
	if len(createdEntryInputs) != 2 {
		t.Fatalf("expected exactly 2 created entries after rerun, got %d", len(createdEntryInputs))
	}
}

func TestScheduleServiceRunDryRunDoesNotMutateState(t *testing.T) {
	t.Parallel()

	schedule := domain.ScheduledPayment{
		ID:            52,
		Name:          "Internet",
		AmountMinor:   6000,
		CurrencyCode:  "USD",
		DayOfMonth:    10,
		StartMonthKey: "2026-01",
	}

	claimCalls := 0
	entryCalls := 0
	repo := &scheduleRepoStub{
		addFn: func(context.Context, domain.ScheduledPaymentAddInput) (domain.ScheduledPayment, error) {
			return domain.ScheduledPayment{}, nil
		},
		listFn: func(context.Context, bool) ([]domain.ScheduledPayment, error) {
			return []domain.ScheduledPayment{schedule}, nil
		},
		getActiveByIDFn: func(context.Context, int64) (domain.ScheduledPayment, error) {
			return schedule, nil
		},
		deleteFn: func(context.Context, int64) (domain.ScheduledPaymentDeleteResult, error) {
			return domain.ScheduledPaymentDeleteResult{}, nil
		},
		listExecutionsByIDFn: func(context.Context, int64) ([]domain.ScheduledPaymentExecution, error) {
			return nil, nil
		},
		claimExecutionFn: func(context.Context, int64, string, string) (bool, error) {
			claimCalls++
			return true, nil
		},
		attachExecutionEntryFn: func(context.Context, int64, string, int64) error {
			return nil
		},
	}
	entryCreator := &scheduleEntryCreatorStub{
		addFn: func(context.Context, domain.EntryAddInput) (domain.Entry, error) {
			entryCalls++
			return domain.Entry{ID: 1}, nil
		},
	}

	svc, err := NewScheduleService(repo, entryCreator)
	if err != nil {
		t.Fatalf("new schedule service: %v", err)
	}

	result, err := svc.Run(context.Background(), domain.ScheduledPaymentRunInput{
		ThroughDateUTC: "2026-03-20",
		DryRun:         true,
	})
	if err != nil {
		t.Fatalf("run schedules dry-run: %v", err)
	}
	if result.CreatedCount != 3 || result.SkippedCount != 0 {
		t.Fatalf("expected created=3 skipped=0, got created=%d skipped=%d", result.CreatedCount, result.SkippedCount)
	}
	if claimCalls != 0 {
		t.Fatalf("expected claim not to be called in dry-run, got %d", claimCalls)
	}
	if entryCalls != 0 {
		t.Fatalf("expected entry creator not to be called in dry-run, got %d", entryCalls)
	}
}

func TestScheduleServiceRunReturnsNotFoundForUnknownScheduleID(t *testing.T) {
	t.Parallel()

	repo := &scheduleRepoStub{
		addFn: func(context.Context, domain.ScheduledPaymentAddInput) (domain.ScheduledPayment, error) {
			return domain.ScheduledPayment{}, nil
		},
		listFn: func(context.Context, bool) ([]domain.ScheduledPayment, error) {
			return nil, nil
		},
		getActiveByIDFn: func(context.Context, int64) (domain.ScheduledPayment, error) {
			return domain.ScheduledPayment{}, domain.ErrScheduleNotFound
		},
		deleteFn: func(context.Context, int64) (domain.ScheduledPaymentDeleteResult, error) {
			return domain.ScheduledPaymentDeleteResult{}, nil
		},
		listExecutionsByIDFn: func(context.Context, int64) ([]domain.ScheduledPaymentExecution, error) {
			return nil, nil
		},
		claimExecutionFn: func(context.Context, int64, string, string) (bool, error) {
			return false, nil
		},
		attachExecutionEntryFn: func(context.Context, int64, string, int64) error {
			return nil
		},
	}

	svc, err := NewScheduleService(repo, &scheduleEntryCreatorStub{
		addFn: func(context.Context, domain.EntryAddInput) (domain.Entry, error) {
			return domain.Entry{}, nil
		},
	})
	if err != nil {
		t.Fatalf("new schedule service: %v", err)
	}

	scheduleID := int64(999)
	_, err = svc.Run(context.Background(), domain.ScheduledPaymentRunInput{
		ThroughDateUTC: "2026-03-20",
		ScheduleID:     &scheduleID,
	})
	if !errors.Is(err, domain.ErrScheduleNotFound) {
		t.Fatalf("expected ErrScheduleNotFound, got %v", err)
	}
}
