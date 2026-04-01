package gemini

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// QuotaInfo holds usage data for a single quota bucket.
type QuotaInfo struct {
	Used      int `json:"used"`
	Limit     int `json:"limit"`
	Remaining int `json:"remaining"`
}

// Usage holds the complete usage snapshot.
type Usage struct {
	Pro         QuotaInfo `json:"pro"`
	Thinking    QuotaInfo `json:"thinking"`
	Flash       int       `json:"flash"`
	LastUpdated time.Time `json:"last_updated"`
	Error       string    `json:"error,omitempty"`
}

// FetchUsage performs a single usage fetch and returns the result.
// If authentication fails (including silent failures detected during API calls),
// it invalidates the cookie cache and retries with fresh cookies.
func FetchUsage(ctx context.Context, provider CookieProvider, opts ...ClientOption) (*Usage, error) {
	client, err := NewClient(ctx, provider, opts...)
	if err != nil {
		if !tryInvalidate(provider) {
			return nil, fmt.Errorf("create client: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
		client, err = NewClient(ctx, provider, opts...)
		if err != nil {
			return nil, fmt.Errorf("create client (retry): %w", err)
		}
	}

	usage, err := fetchWithClient(ctx, client)
	if err != nil && errors.Is(err, ErrAuthExpired) {
		// Auth expired during API call — refresh cookies and rebuild client
		if tryInvalidate(provider) {
			time.Sleep(500 * time.Millisecond)
			client, err = NewClient(ctx, provider, opts...)
			if err != nil {
				return nil, fmt.Errorf("create client (auth retry): %w", err)
			}
			return fetchWithClient(ctx, client)
		}
	}
	return usage, err
}

func fetchWithClient(ctx context.Context, client *Client) (*Usage, error) {
	resetTime := client.resetTime()
	counts, err := client.CountUsageSince(ctx, resetTime)
	if err != nil {
		return nil, fmt.Errorf("count usage: %w", err)
	}

	return &Usage{
		Pro: QuotaInfo{
			Used:      counts.Pro,
			Limit:     client.cfg.proLimit,
			Remaining: max(0, client.cfg.proLimit-counts.Pro),
		},
		Thinking: QuotaInfo{
			Used:      counts.Thinking,
			Limit:     client.cfg.thinkingLimit,
			Remaining: max(0, client.cfg.thinkingLimit-counts.Thinking),
		},
		Flash:       counts.Flash,
		LastUpdated: time.Now().UTC(),
	}, nil
}

// tryInvalidate attempts to invalidate the provider's cookie cache.
// Returns true if the provider supports invalidation.
func tryInvalidate(provider CookieProvider) bool {
	if inv, ok := provider.(Invalidator); ok {
		inv.Invalidate()
		return true
	}
	return false
}
