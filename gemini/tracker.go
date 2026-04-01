package gemini

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// TrackerOption configures a Tracker.
type TrackerOption func(*trackerConfig)

type trackerConfig struct {
	interval      time.Duration
	cookieRefresh time.Duration
	callback      func(*Usage)
	clientOpts    []ClientOption
	logger        *slog.Logger
}

func defaultTrackerConfig() trackerConfig {
	return trackerConfig{
		interval:      DefaultPollInterval,
		cookieRefresh: 6 * time.Hour,
		logger:        slog.Default(),
	}
}

// WithInterval sets the polling interval. Default: 60s.
func WithInterval(d time.Duration) TrackerOption {
	return func(t *trackerConfig) { t.interval = d }
}

// WithCookieRefresh sets how often to re-extract cookies from the provider.
// This creates a fresh Client with new cookies on the given interval.
// Default: 6 hours.
func WithCookieRefresh(d time.Duration) TrackerOption {
	return func(t *trackerConfig) { t.cookieRefresh = d }
}

// WithCallback sets a function called on each usage update.
// The callback runs in the tracker's goroutine; keep it fast.
func WithCallback(fn func(*Usage)) TrackerOption {
	return func(t *trackerConfig) { t.callback = fn }
}

// WithClientOptions passes ClientOption values through to the underlying Client.
func WithClientOptions(opts ...ClientOption) TrackerOption {
	return func(t *trackerConfig) { t.clientOpts = append(t.clientOpts, opts...) }
}

// Tracker continuously polls Gemini usage and delivers results via
// a channel and/or callback. It automatically refreshes cookies on
// a configurable interval and on auth errors.
//
// The Tracker stops when the context is cancelled.
type Tracker struct {
	provider  CookieProvider
	cfg       trackerConfig
	client    *Client
	updates   chan *Usage
	lastTotal int // last non-zero total count, for detecting suspicious drops
}

// NewTracker creates a Tracker that polls usage at a regular interval.
// The tracker starts immediately; consume results from Updates().
// Cancel the context to stop the tracker.
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	tracker, err := gemini.NewTracker(ctx, provider, gemini.WithInterval(30*time.Second))
//	for usage := range tracker.Updates() { ... }
func NewTracker(ctx context.Context, provider CookieProvider, opts ...TrackerOption) (*Tracker, error) {
	cfg := defaultTrackerConfig()
	for _, o := range opts {
		o(&cfg)
	}

	// Extract the logger from client options if set, or use tracker's default
	for _, o := range cfg.clientOpts {
		var probe clientConfig
		o(&probe)
		if probe.logger != nil {
			cfg.logger = probe.logger
		}
	}

	t := &Tracker{
		provider: provider,
		cfg:      cfg,
		updates:  make(chan *Usage, 1),
	}

	// Create initial client
	client, err := t.newClient(ctx)
	if err != nil {
		// Try invalidating cache and retrying
		if inv, ok := provider.(Invalidator); ok {
			cfg.logger.Info("auth failed, invalidating cookie cache and retrying")
			inv.Invalidate()
			time.Sleep(500 * time.Millisecond)
			client, err = t.newClient(ctx)
		}
		if err != nil {
			close(t.updates)
			return nil, err
		}
	}
	t.client = client

	go t.run(ctx)
	return t, nil
}

// Updates returns a channel that receives usage snapshots on each poll.
// The channel is closed when the tracker stops (context cancelled).
func (t *Tracker) Updates() <-chan *Usage {
	return t.updates
}

func (t *Tracker) newClient(ctx context.Context) (*Client, error) {
	return NewClient(ctx, t.provider, t.cfg.clientOpts...)
}

func (t *Tracker) run(ctx context.Context) {
	defer close(t.updates)

	// Initial poll
	t.poll(ctx)

	ticker := time.NewTicker(t.cfg.interval)
	defer ticker.Stop()

	var cookieTimer *time.Timer
	if t.cfg.cookieRefresh > 0 {
		cookieTimer = time.NewTimer(t.cfg.cookieRefresh)
		defer cookieTimer.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.poll(ctx)
		case <-cookieTimerChan(cookieTimer):
			t.refreshClient(ctx)
			cookieTimer.Reset(t.cfg.cookieRefresh)
		}
	}
}

func (t *Tracker) poll(ctx context.Context) {
	resetTime := t.client.resetTime()
	counts, err := t.client.CountUsageSince(ctx, resetTime)

	if err != nil && errors.Is(err, ErrAuthExpired) {
		// Auth expired — refresh cookies and retry
		t.cfg.logger.Warn("auth expired, refreshing cookies", "err", err)
		if t.refreshClient(ctx) == nil {
			counts, err = t.client.CountUsageSince(ctx, resetTime)
		}
	} else if err != nil {
		t.cfg.logger.Warn("poll failed", "err", err)
	}

	// Detect suspicious zero results: if we previously saw usage but now see
	// zero, it likely means the session expired silently. Try refreshing.
	if err == nil {
		total := counts.Pro + counts.Thinking + counts.Flash
		if total == 0 && t.lastTotal > 0 {
			t.cfg.logger.Warn("usage dropped to zero unexpectedly, refreshing cookies",
				"previous_total", t.lastTotal)
			if t.refreshClient(ctx) == nil {
				counts, err = t.client.CountUsageSince(ctx, resetTime)
			}
		}
		if total > 0 || (counts.Pro+counts.Thinking+counts.Flash) > 0 {
			t.lastTotal = counts.Pro + counts.Thinking + counts.Flash
		}
	}

	usage := &Usage{
		LastUpdated: time.Now().UTC(),
	}

	if err != nil {
		usage.Error = err.Error()
	} else {
		usage.Pro = QuotaInfo{
			Used:      counts.Pro,
			Limit:     t.client.cfg.proLimit,
			Remaining: max(0, t.client.cfg.proLimit-counts.Pro),
		}
		usage.Thinking = QuotaInfo{
			Used:      counts.Thinking,
			Limit:     t.client.cfg.thinkingLimit,
			Remaining: max(0, t.client.cfg.thinkingLimit-counts.Thinking),
		}
		usage.Flash = counts.Flash
	}

	if t.cfg.callback != nil {
		t.cfg.callback(usage)
	}

	select {
	case t.updates <- usage:
	default: // Don't block if consumer is slow
	}
}

func (t *Tracker) refreshClient(ctx context.Context) error {
	// Invalidate cache if the provider supports it
	if inv, ok := t.provider.(Invalidator); ok {
		inv.Invalidate()
	}

	client, err := t.newClient(ctx)
	if err != nil {
		t.cfg.logger.Error("cookie refresh failed", "err", err)
		return err
	}

	t.client = client
	t.cfg.logger.Info("cookies refreshed successfully")
	return nil
}

func cookieTimerChan(t *time.Timer) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}
