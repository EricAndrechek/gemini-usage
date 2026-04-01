package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	batchExecURL = "https://gemini.google.com/_/BardChatUi/data/batchexecute"
	initURL      = "https://gemini.google.com/app"
	googleURL    = "https://www.google.com"
	userAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"

	rpcListChats = "MaZiqc"
	rpcReadChat  = "hNvQHb"
)

// Defaults for client options.
const (
	DefaultHTTPTimeout   = 30 * time.Second
	DefaultProLimit      = 100
	DefaultThinkingLimit = 300
	DefaultPollInterval  = 60 * time.Second
)

// ClientOption configures a Client.
type ClientOption func(*clientConfig)

type clientConfig struct {
	httpTimeout   time.Duration
	proLimit      int
	thinkingLimit int
	resetTimezone *time.Location
	logger        *slog.Logger
}

func defaultClientConfig() clientConfig {
	return clientConfig{
		httpTimeout:   DefaultHTTPTimeout,
		proLimit:      DefaultProLimit,
		thinkingLimit: DefaultThinkingLimit,
		resetTimezone: time.Local,
		logger:        slog.Default(),
	}
}

// WithHTTPTimeout sets the HTTP client timeout. Default: 30s.
func WithHTTPTimeout(d time.Duration) ClientOption {
	return func(c *clientConfig) { c.httpTimeout = d }
}

// WithProLimit sets the daily Pro prompt quota. Default: 100.
func WithProLimit(n int) ClientOption {
	return func(c *clientConfig) { c.proLimit = n }
}

// WithThinkingLimit sets the daily Thinking prompt quota. Default: 300.
func WithThinkingLimit(n int) ClientOption {
	return func(c *clientConfig) { c.thinkingLimit = n }
}

// WithResetTimezone sets the timezone for daily quota resets.
// Default: system local timezone.
func WithResetTimezone(loc *time.Location) ClientOption {
	return func(c *clientConfig) { c.resetTimezone = loc }
}

// WithLogger sets the structured logger. Default: slog.Default().
func WithLogger(l *slog.Logger) ClientOption {
	return func(c *clientConfig) { c.logger = l }
}

// Client talks to Gemini's internal web APIs.
type Client struct {
	http        *http.Client
	accessToken string
	buildLabel  string
	sessionID   string
	reqID       int
	cfg         clientConfig
}

// NewClient creates a Client authenticated with the given CookieProvider.
// It fetches cookies, sets up the HTTP client, and extracts session tokens.
func NewClient(ctx context.Context, provider CookieProvider, opts ...ClientOption) (*Client, error) {
	cfg := defaultClientConfig()
	for _, o := range opts {
		o(&cfg)
	}

	cookies, err := provider.Cookies(ctx)
	if err != nil {
		return nil, fmt.Errorf("get cookies: %w", err)
	}

	jar, _ := cookiejar.New(nil)
	googleParsed, _ := url.Parse(googleURL)
	geminiParsed, _ := url.Parse(initURL)
	jar.SetCookies(googleParsed, cookies)
	jar.SetCookies(geminiParsed, cookies)

	c := &Client{
		http: &http.Client{
			Jar:     jar,
			Timeout: cfg.httpTimeout,
		},
		reqID: 10000 + int(time.Now().UnixNano()%90000),
		cfg:   cfg,
	}

	if err := c.init(ctx); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}

	return c, nil
}

var (
	snlm0eRe = regexp.MustCompile(`"SNlM0e":\s*"(.*?)"`)
	cfb2hRe  = regexp.MustCompile(`"cfb2h":\s*"(.*?)"`)
	fdrfjeRe = regexp.MustCompile(`"FdrFJe":\s*"(.*?)"`)
)

func (c *Client) init(ctx context.Context) error {
	// Hit google.com first to pick up extra cookies
	req, _ := http.NewRequestWithContext(ctx, "GET", googleURL, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("google.com: %w", err)
	}
	resp.Body.Close()

	// Fetch the Gemini app page to extract tokens
	req, _ = http.NewRequestWithContext(ctx, "GET", initURL, nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err = c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gemini init: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	text := string(body)

	if m := snlm0eRe.FindStringSubmatch(text); len(m) > 1 {
		c.accessToken = m[1]
	}
	if m := cfb2hRe.FindStringSubmatch(text); len(m) > 1 {
		c.buildLabel = m[1]
	}
	if m := fdrfjeRe.FindStringSubmatch(text); len(m) > 1 {
		c.sessionID = m[1]
	}

	if c.accessToken == "" && c.buildLabel == "" {
		return fmt.Errorf("failed to extract tokens from init page; cookies may be invalid")
	}

	return nil
}

// batchExecute sends a batchexecute RPC call.
func (c *Client) batchExecute(ctx context.Context, rpcID, payload string) (string, error) {
	reqID := c.reqID
	c.reqID += 100000

	params := url.Values{}
	params.Set("rpcids", rpcID)
	params.Set("_reqid", fmt.Sprintf("%d", reqID))
	params.Set("rt", "c")
	params.Set("source-path", "/app")
	if c.buildLabel != "" {
		params.Set("bl", c.buildLabel)
	}
	if c.sessionID != "" {
		params.Set("f.sid", c.sessionID)
	}

	// Build the f.req payload.
	// The payload must be a JSON *string* inside the outer array, not raw JSON.
	payloadQuoted, _ := json.Marshal(payload) // turns `[50]` into `"[50]"`
	serialized := fmt.Sprintf(`[[["%s",%s,null,"generic"]]]`, rpcID, string(payloadQuoted))

	form := url.Values{}
	if c.accessToken != "" {
		form.Set("at", c.accessToken)
	}
	form.Set("f.req", serialized)

	fullURL := batchExecURL + "?" + params.Encode()
	req, _ := http.NewRequestWithContext(ctx, "POST", fullURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", "https://gemini.google.com")
	req.Header.Set("Referer", "https://gemini.google.com/")
	req.Header.Set("X-Same-Domain", "1")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("batchexecute: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("batchexecute: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// ChatInfo holds metadata about a conversation.
type ChatInfo struct {
	CID       string
	Title     string
	Timestamp int64 // Unix seconds
}

// ListChats fetches the conversation list.
func (c *Client) ListChats(ctx context.Context, limit int) ([]ChatInfo, error) {
	payload := fmt.Sprintf("[%d]", limit)
	text, err := c.batchExecute(ctx, rpcListChats, payload)
	if err != nil {
		return nil, err
	}

	parts, err := extractJSONFromResponse(text)
	if err != nil {
		return nil, err
	}

	var chats []ChatInfo
	for _, part := range parts {
		var parsed []any
		if json.Unmarshal(part, &parsed) != nil {
			continue
		}

		bodyStr, ok := getNestedValue(parsed, 2).(string)
		if !ok || len(bodyStr) < 100 {
			continue
		}

		var body []any
		if json.Unmarshal([]byte(bodyStr), &body) != nil {
			continue
		}

		chatList, ok := getNestedValue(body, 2).([]any)
		if !ok {
			continue
		}

		for _, item := range chatList {
			chat, ok := item.([]any)
			if !ok || len(chat) < 6 {
				continue
			}
			cid, _ := chat[0].(string)
			title, _ := chat[1].(string)

			var ts int64
			if tsArr, ok := chat[5].([]any); ok && len(tsArr) > 0 {
				if f, ok := tsArr[0].(float64); ok {
					ts = int64(f)
				}
			}

			chats = append(chats, ChatInfo{CID: cid, Title: title, Timestamp: ts})
		}
		break
	}

	return chats, nil
}

// TurnCounts holds per-bucket turn counts for a chat.
type TurnCounts struct {
	Pro      int
	Thinking int
	Flash    int
}

// CountTurnsInChat reads a conversation and counts user turns by quota bucket.
func (c *Client) CountTurnsInChat(ctx context.Context, cid string) (TurnCounts, error) {
	var counts TurnCounts

	payloadJSON, _ := json.Marshal([]any{cid, 100, nil, 1, []int{0}, []int{4}, nil, 1})
	text, err := c.batchExecute(ctx, rpcReadChat, string(payloadJSON))
	if err != nil {
		return counts, err
	}

	parts, err := extractJSONFromResponse(text)
	if err != nil {
		return counts, err
	}

	for _, part := range parts {
		var parsed []any
		if json.Unmarshal(part, &parsed) != nil {
			continue
		}

		bodyStr, ok := getNestedValue(parsed, 2).(string)
		if !ok || len(bodyStr) < 50 {
			continue
		}

		var body []any
		if json.Unmarshal([]byte(bodyStr), &body) != nil {
			continue
		}

		turns, ok := getNestedValue(body, 0).([]any)
		if !ok {
			continue
		}

		for _, t := range turns {
			turn, ok := t.([]any)
			if !ok || len(turn) < 3 {
				continue
			}

			var modelID string
			if userSection, ok := turn[2].([]any); ok && len(userSection) > 4 {
				if mid, ok := userSection[4].(string); ok && len(mid) == 16 {
					modelID = mid
				}
			}

			hasThoughts := getNestedString(turn, 3, 0, 0, 37, 0, 0) != ""

			bucket := classifyTurn(modelID, hasThoughts)
			switch bucket {
			case "pro":
				counts.Pro++
			case "thinking":
				counts.Thinking++
			default:
				counts.Flash++
			}
		}
		break
	}

	return counts, nil
}

// CountUsageSince counts all user turns by quota bucket since the given time.
func (c *Client) CountUsageSince(ctx context.Context, since time.Time) (TurnCounts, error) {
	var totals TurnCounts
	sinceTS := since.Unix()

	chats, err := c.ListChats(ctx, 200)
	if err != nil {
		return totals, err
	}

	var todayCount int
	for _, chat := range chats {
		if chat.Timestamp >= sinceTS {
			todayCount++
		}
	}
	c.cfg.logger.Info("counting usage", "chats", todayCount, "since", since.Format("2006-01-02 15:04"))

	for _, chat := range chats {
		if chat.Timestamp < sinceTS {
			continue
		}

		counts, err := c.CountTurnsInChat(ctx, chat.CID)
		if err != nil {
			c.cfg.logger.Warn("failed to read chat", "cid", chat.CID, "err", err)
			continue
		}

		totals.Pro += counts.Pro
		totals.Thinking += counts.Thinking
		totals.Flash += counts.Flash
	}

	return totals, nil
}

// resetTime returns midnight in the configured timezone for the current day.
func (c *Client) resetTime() time.Time {
	now := time.Now().In(c.cfg.resetTimezone)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, c.cfg.resetTimezone)
}
