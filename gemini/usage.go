package gemini

import (
	"context"
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
// If authentication fails, it invalidates the cookie cache (if supported)
// and retries with fresh cookies.
func FetchUsage(ctx context.Context, provider CookieProvider, opts ...ClientOption) (*Usage, error) {
	client, err := NewClient(ctx, provider, opts...)
	if err != nil {
		// Auth failure — try invalidating cache and retrying
		if inv, ok := provider.(Invalidator); ok {
			inv.Invalidate()
			time.Sleep(500 * time.Millisecond)
			client, err = NewClient(ctx, provider, opts...)
		}
		if err != nil {
			return nil, fmt.Errorf("create client: %w", err)
		}
	}

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
