package service

import (
	"context"
	"fmt"
	"time"

	"budgetto/internal/domain"
)

type SetupSettingsRepository interface {
	Upsert(ctx context.Context, input domain.SettingsUpsertInput) (domain.Settings, error)
	Get(ctx context.Context) (domain.Settings, error)
}

type SetupService struct {
	settingsRepo SetupSettingsRepository
	entryService *EntryService
	capService   *CapService
	nowFn        func() time.Time
}

type SetupInitInput struct {
	DefaultCurrencyCode  string
	DisplayTimezone      string
	OpeningBalanceMinor  int64
	OpeningBalanceCode   string
	OpeningBalanceDate   string
	CurrentMonthCapMinor int64
	CurrentMonthCapCode  string
	CurrentMonthKey      string
}

type SetupInitResult struct {
	Settings         domain.Settings          `json:"settings"`
	OpeningBalance   *domain.Entry            `json:"opening_balance_entry,omitempty"`
	OpeningWarnings  []domain.Warning         `json:"opening_warnings"`
	CurrentMonthCap  *domain.MonthlyCap       `json:"current_month_cap,omitempty"`
	CurrentCapChange *domain.MonthlyCapChange `json:"current_month_cap_change,omitempty"`
}

func NewSetupService(settingsRepo SetupSettingsRepository, entryService *EntryService, capService *CapService) (*SetupService, error) {
	if settingsRepo == nil {
		return nil, fmt.Errorf("setup service: settings repo is required")
	}

	return &SetupService{
		settingsRepo: settingsRepo,
		entryService: entryService,
		capService:   capService,
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
	}, nil
}

func (s *SetupService) Init(ctx context.Context, input SetupInitInput) (SetupInitResult, error) {
	nowUTC := s.nowFn().UTC().Format(time.RFC3339Nano)

	settings, err := s.settingsRepo.Upsert(ctx, domain.SettingsUpsertInput{
		DefaultCurrencyCode:        input.DefaultCurrencyCode,
		DisplayTimezone:            input.DisplayTimezone,
		OrphanCountThreshold:       domain.DefaultOrphanCountThresholdValue,
		OrphanSpendingThresholdBPS: domain.DefaultOrphanSpendingThresholdBPSValue,
		OnboardingCompletedAtUTC:   &nowUTC,
	})
	if err != nil {
		return SetupInitResult{}, err
	}

	result := SetupInitResult{
		Settings:        settings,
		OpeningWarnings: []domain.Warning{},
	}

	if input.OpeningBalanceMinor > 0 {
		if s.entryService == nil {
			return SetupInitResult{}, fmt.Errorf("setup service: entry service is required for opening balance")
		}

		currency := input.OpeningBalanceCode
		if currency == "" {
			currency = input.DefaultCurrencyCode
		}

		dateValue := input.OpeningBalanceDate
		if dateValue == "" {
			dateValue = nowUTC
		}

		opening, err := s.entryService.AddWithWarnings(ctx, domain.EntryAddInput{
			Type:               domain.EntryTypeIncome,
			AmountMinor:        input.OpeningBalanceMinor,
			CurrencyCode:       currency,
			TransactionDateUTC: dateValue,
			Note:               "Opening balance",
		})
		if err != nil {
			return SetupInitResult{}, err
		}

		result.OpeningBalance = &opening.Entry
		result.OpeningWarnings = opening.Warnings
	}

	if input.CurrentMonthCapMinor > 0 {
		if s.capService == nil {
			return SetupInitResult{}, fmt.Errorf("setup service: cap service is required for month cap")
		}

		monthKey := input.CurrentMonthKey
		if monthKey == "" {
			monthKey = s.nowFn().UTC().Format("2006-01")
		}

		currency := input.CurrentMonthCapCode
		if currency == "" {
			currency = input.DefaultCurrencyCode
		}

		capValue, capChange, err := s.capService.Set(ctx, domain.CapSetInput{
			MonthKey:     monthKey,
			AmountMinor:  input.CurrentMonthCapMinor,
			CurrencyCode: currency,
		})
		if err != nil {
			return SetupInitResult{}, err
		}

		result.CurrentMonthCap = &capValue
		result.CurrentCapChange = &capChange
	}

	return result, nil
}

func (s *SetupService) Show(ctx context.Context) (domain.Settings, error) {
	return s.settingsRepo.Get(ctx)
}
