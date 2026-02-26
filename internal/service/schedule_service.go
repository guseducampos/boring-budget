package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"boring-budget/internal/domain"
)

type ScheduledPaymentRepository interface {
	Add(ctx context.Context, input domain.ScheduledPaymentAddInput) (domain.ScheduledPayment, error)
	List(ctx context.Context, includeDeleted bool) ([]domain.ScheduledPayment, error)
	GetActiveByID(ctx context.Context, id int64) (domain.ScheduledPayment, error)
	Delete(ctx context.Context, id int64) (domain.ScheduledPaymentDeleteResult, error)
	ListExecutionsByScheduleID(ctx context.Context, scheduleID int64) ([]domain.ScheduledPaymentExecution, error)
	ClaimExecution(ctx context.Context, scheduleID int64, monthKey, createdAtUTC string) (bool, error)
	AttachExecutionEntry(ctx context.Context, scheduleID int64, monthKey string, entryID int64) error
}

type ScheduledPaymentEntryCreator interface {
	Add(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error)
}

type ScheduleService struct {
	repo         ScheduledPaymentRepository
	entryCreator ScheduledPaymentEntryCreator
}

func NewScheduleService(repo ScheduledPaymentRepository, entryCreator ScheduledPaymentEntryCreator) (*ScheduleService, error) {
	if repo == nil {
		return nil, fmt.Errorf("schedule service: repo is required")
	}

	return &ScheduleService{
		repo:         repo,
		entryCreator: entryCreator,
	}, nil
}

func (s *ScheduleService) Add(ctx context.Context, input domain.ScheduledPaymentAddInput) (domain.ScheduledPayment, error) {
	normalized, err := domain.NormalizeScheduledPaymentAddInput(input)
	if err != nil {
		return domain.ScheduledPayment{}, err
	}

	return s.repo.Add(ctx, normalized)
}

func (s *ScheduleService) List(ctx context.Context, includeDeleted bool) ([]domain.ScheduledPayment, error) {
	return s.repo.List(ctx, includeDeleted)
}

func (s *ScheduleService) Delete(ctx context.Context, id int64) (domain.ScheduledPaymentDeleteResult, error) {
	if err := domain.ValidateScheduleID(id); err != nil {
		return domain.ScheduledPaymentDeleteResult{}, err
	}

	return s.repo.Delete(ctx, id)
}

func (s *ScheduleService) Run(ctx context.Context, input domain.ScheduledPaymentRunInput) (domain.ScheduledPaymentRunResult, error) {
	normalizedInput, throughDateUTC, err := domain.NormalizeScheduledPaymentRunInput(input)
	if err != nil {
		return domain.ScheduledPaymentRunResult{}, err
	}

	schedules, err := s.loadSchedulesForRun(ctx, normalizedInput.ScheduleID)
	if err != nil {
		return domain.ScheduledPaymentRunResult{}, err
	}

	sort.Slice(schedules, func(i, j int) bool {
		return schedules[i].ID < schedules[j].ID
	})

	result := domain.ScheduledPaymentRunResult{
		ThroughDateUTC: normalizedInput.ThroughDateUTC,
		ScheduleID:     normalizedInput.ScheduleID,
		DryRun:         normalizedInput.DryRun,
		CreatedCount:   0,
		SkippedCount:   0,
	}

	nowUTC := time.Now().UTC().Format(time.RFC3339Nano)

	for _, schedule := range schedules {
		months, err := domain.ScheduledPaymentOccurrenceMonths(schedule, throughDateUTC)
		if err != nil {
			return domain.ScheduledPaymentRunResult{}, err
		}
		if len(months) == 0 {
			continue
		}

		executionByMonth, err := s.executionIndexByMonth(ctx, schedule.ID)
		if err != nil {
			return domain.ScheduledPaymentRunResult{}, err
		}

		for _, monthKey := range months {
			existingExecution, exists := executionByMonth[monthKey]
			if exists {
				if normalizedInput.DryRun || existingExecution.EntryID != nil {
					result.SkippedCount++
					continue
				}

				if err := s.createAndAttachEntry(ctx, schedule, monthKey); err != nil {
					return domain.ScheduledPaymentRunResult{}, err
				}
				result.CreatedCount++
				continue
			}

			if normalizedInput.DryRun {
				result.CreatedCount++
				continue
			}

			claimed, err := s.repo.ClaimExecution(ctx, schedule.ID, monthKey, nowUTC)
			if err != nil {
				return domain.ScheduledPaymentRunResult{}, err
			}
			if !claimed {
				result.SkippedCount++
				continue
			}

			if err := s.createAndAttachEntry(ctx, schedule, monthKey); err != nil {
				return domain.ScheduledPaymentRunResult{}, err
			}
			result.CreatedCount++
		}
	}

	return result, nil
}

func (s *ScheduleService) loadSchedulesForRun(ctx context.Context, scheduleID *int64) ([]domain.ScheduledPayment, error) {
	if scheduleID != nil {
		schedule, err := s.repo.GetActiveByID(ctx, *scheduleID)
		if err != nil {
			return nil, err
		}
		return []domain.ScheduledPayment{schedule}, nil
	}

	return s.repo.List(ctx, false)
}

func (s *ScheduleService) executionIndexByMonth(ctx context.Context, scheduleID int64) (map[string]domain.ScheduledPaymentExecution, error) {
	executions, err := s.repo.ListExecutionsByScheduleID(ctx, scheduleID)
	if err != nil {
		return nil, err
	}

	indexed := make(map[string]domain.ScheduledPaymentExecution, len(executions))
	for _, execution := range executions {
		indexed[execution.MonthKey] = execution
	}

	return indexed, nil
}

func (s *ScheduleService) createAndAttachEntry(ctx context.Context, schedule domain.ScheduledPayment, monthKey string) error {
	if s.entryCreator == nil {
		return fmt.Errorf("schedule run: entry creator is required")
	}

	occurrenceDateUTC, err := domain.ScheduledPaymentOccurrenceDateUTC(monthKey, schedule.DayOfMonth)
	if err != nil {
		return err
	}

	entry, err := s.entryCreator.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        schedule.AmountMinor,
		CurrencyCode:       schedule.CurrencyCode,
		TransactionDateUTC: occurrenceDateUTC,
		CategoryID:         schedule.CategoryID,
		Note:               schedule.Note,
	})
	if err != nil {
		return err
	}

	return s.repo.AttachExecutionEntry(ctx, schedule.ID, monthKey, entry.ID)
}
