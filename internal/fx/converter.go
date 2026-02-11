package fx

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"boring-budget/internal/domain"
)

type RateQuote struct {
	Provider      string
	BaseCurrency  string
	QuoteCurrency string
	Rate          string
	RateDate      string
}

type Provider interface {
	Name() string
	HistoricalRate(ctx context.Context, baseCurrency, quoteCurrency, date string) (RateQuote, error)
	LatestRate(ctx context.Context, baseCurrency, quoteCurrency string) (RateQuote, error)
}

type SnapshotStore interface {
	GetSnapshotByKey(ctx context.Context, provider, baseCurrency, quoteCurrency, rateDate string, isEstimate bool) (domain.FXRateSnapshot, error)
	CreateSnapshot(ctx context.Context, input domain.FXRateSnapshotCreateInput) (domain.FXRateSnapshot, error)
}

type Converter struct {
	provider  Provider
	snapshots SnapshotStore
	nowFn     func() time.Time
}

func NewConverter(provider Provider, snapshots SnapshotStore) (*Converter, error) {
	if provider == nil {
		return nil, fmt.Errorf("fx converter: provider is required")
	}
	if snapshots == nil {
		return nil, fmt.Errorf("fx converter: snapshot store is required")
	}

	return &Converter{
		provider:  provider,
		snapshots: snapshots,
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
	}, nil
}

func (c *Converter) Convert(ctx context.Context, amountMinor int64, fromCurrency, toCurrency, transactionDateUTC string) (domain.ConvertedAmount, error) {
	if amountMinor < 0 {
		return domain.ConvertedAmount{}, domain.ErrInvalidAmountMinor
	}

	from, err := domain.NormalizeCurrencyCode(fromCurrency)
	if err != nil {
		return domain.ConvertedAmount{}, err
	}
	to, err := domain.NormalizeCurrencyCode(toCurrency)
	if err != nil {
		return domain.ConvertedAmount{}, err
	}

	dateValue, err := domain.NormalizeTransactionDateUTC(transactionDateUTC)
	if err != nil {
		return domain.ConvertedAmount{}, err
	}

	if from == to {
		dateKey, err := toDateKey(dateValue)
		if err != nil {
			return domain.ConvertedAmount{}, err
		}
		return domain.ConvertedAmount{
			AmountMinor: amountMinor,
			Snapshot: domain.FXRateSnapshot{
				Provider:      "identity",
				BaseCurrency:  from,
				QuoteCurrency: to,
				Rate:          "1",
				RateDate:      dateKey,
				IsEstimate:    false,
				FetchedAtUTC:  c.nowFn().Format(time.RFC3339Nano),
			},
		}, nil
	}

	txDate, err := time.Parse(time.RFC3339Nano, dateValue)
	if err != nil {
		return domain.ConvertedAmount{}, err
	}

	txDateUTC := txDate.UTC()
	nowUTC := c.nowFn().UTC()
	isEstimate := txDateUTC.After(nowUTC)
	rateDate := txDateUTC.Format("2006-01-02")

	if !isEstimate {
		cached, err := c.snapshots.GetSnapshotByKey(ctx, c.provider.Name(), from, to, rateDate, false)
		if err == nil {
			rateValue, parseErr := strconv.ParseFloat(cached.Rate, 64)
			if parseErr != nil || rateValue <= 0 {
				return domain.ConvertedAmount{}, domain.ErrInvalidFXRate
			}
			converted := int64(math.Round(float64(amountMinor) * rateValue))
			if converted < 0 {
				converted = 0
			}
			return domain.ConvertedAmount{
				AmountMinor: converted,
				Snapshot:    cached,
			}, nil
		}
	}

	quote := RateQuote{}
	if isEstimate {
		quote, err = c.provider.LatestRate(ctx, from, to)
		if err != nil {
			return domain.ConvertedAmount{}, fmt.Errorf("latest fx rate: %w", domain.ErrFXRateUnavailable)
		}
		rateDate = strings.TrimSpace(quote.RateDate)
	} else {
		quote, err = c.provider.HistoricalRate(ctx, from, to, rateDate)
		if err != nil {
			return domain.ConvertedAmount{}, fmt.Errorf("historical fx rate: %w", domain.ErrFXRateUnavailable)
		}
	}

	rateDate = strings.TrimSpace(rateDate)
	if strings.TrimSpace(quote.RateDate) != "" {
		rateDate = strings.TrimSpace(quote.RateDate)
	}

	snapshot, err := c.getOrCreateSnapshot(ctx, quote, rateDate, isEstimate)
	if err != nil {
		return domain.ConvertedAmount{}, err
	}

	rateValue, err := strconv.ParseFloat(snapshot.Rate, 64)
	if err != nil || rateValue <= 0 {
		return domain.ConvertedAmount{}, domain.ErrInvalidFXRate
	}

	converted := int64(math.Round(float64(amountMinor) * rateValue))
	if converted < 0 {
		converted = 0
	}

	return domain.ConvertedAmount{
		AmountMinor: converted,
		Snapshot:    snapshot,
	}, nil
}

func (c *Converter) getOrCreateSnapshot(ctx context.Context, quote RateQuote, rateDate string, isEstimate bool) (domain.FXRateSnapshot, error) {
	provider := strings.TrimSpace(c.provider.Name())
	if strings.TrimSpace(quote.Provider) != "" {
		provider = strings.TrimSpace(quote.Provider)
	}

	existing, err := c.snapshots.GetSnapshotByKey(ctx, provider, quote.BaseCurrency, quote.QuoteCurrency, rateDate, isEstimate)
	if err == nil {
		return existing, nil
	}
	if err != nil && err != domain.ErrFXRateUnavailable {
		return domain.FXRateSnapshot{}, err
	}

	if err := domain.ValidateFXRate(quote.Rate); err != nil {
		return domain.FXRateSnapshot{}, err
	}

	created, err := c.snapshots.CreateSnapshot(ctx, domain.FXRateSnapshotCreateInput{
		Provider:      provider,
		BaseCurrency:  quote.BaseCurrency,
		QuoteCurrency: quote.QuoteCurrency,
		Rate:          quote.Rate,
		RateDate:      rateDate,
		IsEstimate:    isEstimate,
		FetchedAtUTC:  c.nowFn().Format(time.RFC3339Nano),
	})
	if err != nil {
		return domain.FXRateSnapshot{}, err
	}

	return created, nil
}

func toDateKey(transactionDateUTC string) (string, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(transactionDateUTC))
	if err != nil {
		return "", err
	}
	return parsed.UTC().Format("2006-01-02"), nil
}

func formatRate(rate float64) string {
	return strconv.FormatFloat(rate, 'g', 16, 64)
}
