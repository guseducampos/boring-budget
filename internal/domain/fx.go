package domain

import (
	"errors"
	"strconv"
	"strings"
)

const (
	WarningCodeFXEstimateUsed = "FX_ESTIMATE_USED"
	FXEstimateWarningMessage  = "Future-dated conversion used latest available FX rate estimate."
)

var (
	ErrFXRateUnavailable = errors.New("fx rate unavailable")
	ErrInvalidFXRate     = errors.New("invalid fx rate")
)

type FXRateSnapshot struct {
	ID            int64  `json:"id"`
	Provider      string `json:"provider"`
	BaseCurrency  string `json:"base_currency"`
	QuoteCurrency string `json:"quote_currency"`
	Rate          string `json:"rate"`
	RateDate      string `json:"rate_date"`
	IsEstimate    bool   `json:"is_estimate"`
	FetchedAtUTC  string `json:"fetched_at_utc"`
}

type FXRateSnapshotCreateInput struct {
	Provider      string
	BaseCurrency  string
	QuoteCurrency string
	Rate          string
	RateDate      string
	IsEstimate    bool
	FetchedAtUTC  string
}

type ConvertedAmount struct {
	AmountMinor int64
	Snapshot    FXRateSnapshot
}

type ConvertedSummary struct {
	TargetCurrency   string `json:"target_currency"`
	EarningsMinor    int64  `json:"earnings_minor"`
	SpendingMinor    int64  `json:"spending_minor"`
	NetMinor         int64  `json:"net_minor"`
	UsedEstimateRate bool   `json:"used_estimate_rate"`
}

func ValidateFXRate(rate string) error {
	if strings.TrimSpace(rate) == "" {
		return ErrInvalidFXRate
	}

	parsed, err := strconv.ParseFloat(strings.TrimSpace(rate), 64)
	if err != nil || parsed <= 0 {
		return ErrInvalidFXRate
	}

	return nil
}
