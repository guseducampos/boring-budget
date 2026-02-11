package fx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	FrankfurterProviderName = "frankfurter"
	FrankfurterBaseURL      = "https://api.frankfurter.app"
)

type FrankfurterClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewFrankfurterClient(httpClient *http.Client) *FrankfurterClient {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return &FrankfurterClient{
		baseURL:    FrankfurterBaseURL,
		httpClient: client,
	}
}

func (c *FrankfurterClient) Name() string {
	return FrankfurterProviderName
}

func (c *FrankfurterClient) HistoricalRate(ctx context.Context, baseCurrency, quoteCurrency, date string) (RateQuote, error) {
	path := fmt.Sprintf("/%s", strings.TrimSpace(date))
	return c.fetchRate(ctx, path, baseCurrency, quoteCurrency)
}

func (c *FrankfurterClient) LatestRate(ctx context.Context, baseCurrency, quoteCurrency string) (RateQuote, error) {
	return c.fetchRate(ctx, "/latest", baseCurrency, quoteCurrency)
}

func (c *FrankfurterClient) fetchRate(ctx context.Context, path, baseCurrency, quoteCurrency string) (RateQuote, error) {
	base := strings.ToUpper(strings.TrimSpace(baseCurrency))
	quote := strings.ToUpper(strings.TrimSpace(quoteCurrency))

	u, err := url.Parse(strings.TrimRight(c.baseURL, "/") + path)
	if err != nil {
		return RateQuote{}, err
	}

	q := u.Query()
	q.Set("from", base)
	q.Set("to", quote)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return RateQuote{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return RateQuote{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return RateQuote{}, fmt.Errorf("frankfurter response status %d", resp.StatusCode)
	}

	var payload frankfurterResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return RateQuote{}, err
	}

	rate, ok := payload.Rates[quote]
	if !ok || rate <= 0 {
		return RateQuote{}, fmt.Errorf("frankfurter missing rate for %s", quote)
	}

	return RateQuote{
		Provider:      FrankfurterProviderName,
		BaseCurrency:  base,
		QuoteCurrency: quote,
		Rate:          formatRate(rate),
		RateDate:      payload.Date,
	}, nil
}

type frankfurterResponse struct {
	Amount float64            `json:"amount"`
	Base   string             `json:"base"`
	Date   string             `json:"date"`
	Rates  map[string]float64 `json:"rates"`
}
