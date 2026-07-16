package rate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	client  *http.Client
}

func NewClient() *Client {
	return &Client{
		baseURL: "https://api.frankfurter.dev/v1",
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

type latestResponse struct {
	Amount float64            `json:"amount"`
	Base   string             `json:"base"`
	Date   string             `json:"date"`
	Rates  map[string]float64 `json:"rates"`
}

type timeseriesResponse struct {
	Amount    float64                       `json:"amount"`
	Base      string                        `json:"base"`
	StartDate string                        `json:"start_date"`
	EndDate   string                        `json:"end_date"`
	Rates     map[string]map[string]float64 `json:"rates"` // date -> target -> rate
}

// Timeseries fetches Frankfurter's time-series endpoint once for a whole range,
// returning a map of rate date -> (currency code -> rate) for every business day
// in [start, end]. Non-business days are simply absent from the outer map — the
// provider omits weekends and holidays. The base currency is included at 1.0 on
// each returned date so a cross-rate has its anchor. from/end are YYYY-MM-DD.
func (c *Client) Timeseries(ctx context.Context, base, start, end string) (map[string]map[string]float64, error) {
	url := fmt.Sprintf("%s/%s..%s?from=%s", c.baseURL, start, end, base)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch timeseries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frankfurter returned %d", resp.StatusCode)
	}

	var result timeseriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode timeseries: %w", err)
	}

	for _, rates := range result.Rates {
		rates[result.Base] = 1.0 // include the base itself so cross-rates anchor
	}
	return result.Rates, nil
}

// Latest fetches the latest exchange rates from the given base currency.
// Returns a map of currency code -> rate, the rate date, and any error.
func (c *Client) Latest(ctx context.Context, base string) (map[string]float64, string, error) {
	url := fmt.Sprintf("%s/latest?from=%s", c.baseURL, base)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("frankfurter returned %d", resp.StatusCode)
	}

	var result latestResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("decode rates: %w", err)
	}

	// Include the base currency at 1.0
	result.Rates[result.Base] = 1.0

	return result.Rates, result.Date, nil
}
