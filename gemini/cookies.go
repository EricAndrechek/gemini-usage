package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CookieProvider retrieves Google cookies for Gemini authentication.
type CookieProvider interface {
	Cookies(ctx context.Context) ([]*http.Cookie, error)
}

// Invalidator is an optional interface that CookieProviders can implement
// to allow cache invalidation (e.g. CachedProvider).
type Invalidator interface {
	Invalidate() error
}

// StaticCookies returns a CookieProvider that always returns the same cookies.
// Use this when you already have cookie values from another source.
//
//	provider := gemini.StaticCookies("PSID_VALUE", "PSIDTS_VALUE")
func StaticCookies(psid, psidts string) CookieProvider {
	return &staticProvider{psid: psid, psidts: psidts}
}

type staticProvider struct {
	psid   string
	psidts string
}

func (s *staticProvider) Cookies(_ context.Context) ([]*http.Cookie, error) {
	cookies := []*http.Cookie{
		{Name: "__Secure-1PSID", Value: s.psid, Domain: ".google.com", Path: "/", Secure: true, HttpOnly: true},
	}
	if s.psidts != "" {
		cookies = append(cookies, &http.Cookie{
			Name: "__Secure-1PSIDTS", Value: s.psidts, Domain: ".google.com", Path: "/", Secure: true, HttpOnly: true,
		})
	}
	return cookies, nil
}

// FileCookies returns a CookieProvider that reads cookies from a JSON file.
// The file format matches the cookie cache format: either a cache object with
// metadata or a plain array of cookie objects.
//
//	provider := gemini.FileCookies("/path/to/cookies.json")
func FileCookies(path string) CookieProvider {
	return &fileProvider{path: path}
}

type fileProvider struct {
	path string
}

func (f *fileProvider) Cookies(_ context.Context) ([]*http.Cookie, error) {
	return loadCookieFile(f.path)
}

// CacheOption configures the CachedProvider.
type CacheOption func(*cachedProvider)

// WithCachePath sets the file path for the cookie cache.
// Default: ~/.config/gemini-usage/cookies.json (via os.UserConfigDir).
func WithCachePath(path string) CacheOption {
	return func(c *cachedProvider) { c.path = path }
}

// WithCacheTTL sets how long cached cookies are reused before refreshing
// from the inner provider. Default: 12 hours.
func WithCacheTTL(d time.Duration) CacheOption {
	return func(c *cachedProvider) { c.ttl = d }
}

// CachedProvider wraps another CookieProvider with on-disk caching.
// This avoids repeated Keychain prompts when using browser-based providers.
//
//	provider := gemini.CachedProvider(gemini.BraveCookies(""), gemini.WithCacheTTL(6*time.Hour))
func CachedProvider(inner CookieProvider, opts ...CacheOption) CookieProvider {
	c := &cachedProvider{
		inner: inner,
		ttl:   12 * time.Hour,
	}
	for _, o := range opts {
		o(c)
	}
	if c.path == "" {
		c.path = defaultCachePath()
	}
	return c
}

type cachedProvider struct {
	inner CookieProvider
	path  string
	ttl   time.Duration
	mu    sync.Mutex
}

func (c *cachedProvider) Cookies(ctx context.Context) ([]*http.Cookie, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cookies, err := loadCachedCookies(c.path); err == nil {
		return cookies, nil
	}

	cookies, err := c.inner.Cookies(ctx)
	if err != nil {
		return nil, err
	}

	saveCookieCache(c.path, cookies, c.ttl)
	return cookies, nil
}

// Invalidate removes the cached cookie file, forcing the next call to
// fetch fresh cookies from the inner provider.
func (c *cachedProvider) Invalidate() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return os.Remove(c.path)
}

// cachedCookie is the serializable form of an http.Cookie.
type cachedCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	HttpOnly bool   `json:"http_only"`
}

// cookieCache holds cached cookies with a timestamp.
type cookieCache struct {
	Cookies   []cachedCookie `json:"cookies"`
	CachedAt  time.Time      `json:"cached_at"`
	ExpiresAt time.Time      `json:"expires_at"`
}

// DefaultCacheDir returns the directory used for cookie cache files.
func DefaultCacheDir() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "gemini-usage")
}

func defaultCachePath() string {
	return filepath.Join(DefaultCacheDir(), "cookies.json")
}

func loadCachedCookies(path string) ([]*http.Cookie, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cache cookieCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	if time.Now().After(cache.ExpiresAt) {
		return nil, fmt.Errorf("cache expired")
	}

	return cacheToCookies(cache.Cookies), nil
}

func loadCookieFile(path string) ([]*http.Cookie, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cookie file: %w", err)
	}

	// Try the cache format first (has metadata)
	var cache cookieCache
	if err := json.Unmarshal(data, &cache); err == nil && len(cache.Cookies) > 0 {
		return cacheToCookies(cache.Cookies), nil
	}

	// Try plain array of cookies
	var cookies []cachedCookie
	if err := json.Unmarshal(data, &cookies); err == nil && len(cookies) > 0 {
		return cacheToCookies(cookies), nil
	}

	return nil, fmt.Errorf("unrecognized cookie file format")
}

func cacheToCookies(cached []cachedCookie) []*http.Cookie {
	var cookies []*http.Cookie
	for _, c := range cached {
		cookies = append(cookies, &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
		})
	}
	return cookies
}

func saveCookieCache(path string, cookies []*http.Cookie, ttl time.Duration) {
	var cached []cachedCookie
	for _, c := range cookies {
		cached = append(cached, cachedCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
		})
	}

	cache := cookieCache{
		Cookies:   cached,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}

	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, data, 0600)
}
