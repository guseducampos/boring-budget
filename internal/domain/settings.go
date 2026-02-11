package domain

import (
	"errors"
	"time"
)

const (
	DefaultOrphanCountThresholdValue       = 5
	DefaultOrphanSpendingThresholdBPSValue = 500
)

var (
	ErrSettingsNotFound = errors.New("settings not found")
)

type Settings struct {
	ID                         int64   `json:"id"`
	DefaultCurrencyCode        string  `json:"default_currency_code"`
	DisplayTimezone            string  `json:"display_timezone"`
	OrphanCountThreshold       int64   `json:"orphan_count_threshold"`
	OrphanSpendingThresholdBPS int64   `json:"orphan_spending_threshold_bps"`
	OnboardingCompletedAtUTC   *string `json:"onboarding_completed_at_utc,omitempty"`
	CreatedAtUTC               string  `json:"created_at_utc"`
	UpdatedAtUTC               string  `json:"updated_at_utc"`
}

type SettingsUpsertInput struct {
	DefaultCurrencyCode        string
	DisplayTimezone            string
	OrphanCountThreshold       int64
	OrphanSpendingThresholdBPS int64
	OnboardingCompletedAtUTC   *string
}

func NormalizeSettingsInput(input SettingsUpsertInput) (SettingsUpsertInput, error) {
	currencyCode, err := NormalizeCurrencyCode(input.DefaultCurrencyCode)
	if err != nil {
		return SettingsUpsertInput{}, err
	}

	if _, err := time.LoadLocation(input.DisplayTimezone); err != nil {
		return SettingsUpsertInput{}, err
	}

	orphanCountThreshold := input.OrphanCountThreshold
	if orphanCountThreshold <= 0 {
		orphanCountThreshold = DefaultOrphanCountThresholdValue
	}

	orphanSpendingThresholdBPS := input.OrphanSpendingThresholdBPS
	if orphanSpendingThresholdBPS <= 0 {
		orphanSpendingThresholdBPS = DefaultOrphanSpendingThresholdBPSValue
	}

	return SettingsUpsertInput{
		DefaultCurrencyCode:        currencyCode,
		DisplayTimezone:            input.DisplayTimezone,
		OrphanCountThreshold:       orphanCountThreshold,
		OrphanSpendingThresholdBPS: orphanSpendingThresholdBPS,
		OnboardingCompletedAtUTC:   input.OnboardingCompletedAtUTC,
	}, nil
}
