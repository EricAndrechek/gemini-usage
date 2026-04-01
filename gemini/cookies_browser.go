//go:build !nobrowser

package gemini

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/browserutils/kooky"
	_ "github.com/browserutils/kooky/browser/brave"
	_ "github.com/browserutils/kooky/browser/chrome"
)

// BraveCookies returns a CookieProvider that reads cookies from Brave's
// encrypted cookie database. On macOS this triggers a Keychain prompt.
//
// If profile is empty, it defaults to "Profile 1".
// Wrap with CachedProvider to avoid repeated Keychain prompts.
//
//	provider := gemini.CachedProvider(gemini.BraveCookies(""))
func BraveCookies(profile string) CookieProvider {
	if profile == "" {
		profile = "Profile 1"
	}
	return &browserProvider{browser: "brave", profile: profile}
}

// ChromeCookies returns a CookieProvider that reads cookies from Chrome's
// encrypted cookie database. On macOS this triggers a Keychain prompt.
//
// If profile is empty, it defaults to "Default".
// Wrap with CachedProvider to avoid repeated Keychain prompts.
//
//	provider := gemini.CachedProvider(gemini.ChromeCookies(""))
func ChromeCookies(profile string) CookieProvider {
	if profile == "" {
		profile = "Default"
	}
	return &browserProvider{browser: "chrome", profile: profile}
}

type browserProvider struct {
	browser string
	profile string
}

func (b *browserProvider) CacheKey() string {
	return b.browser + "-" + strings.ReplaceAll(b.profile, " ", "_")
}

func (b *browserProvider) Cookies(ctx context.Context) ([]*http.Cookie, error) {
	cookieFile, err := browserCookiePath(b.browser, b.profile)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(cookieFile); err != nil {
		return nil, fmt.Errorf("cookie file not found at %s: %w", cookieFile, err)
	}

	stores := kooky.FindAllCookieStores(ctx)
	var store kooky.CookieStore
	for _, s := range stores {
		if s.FilePath() == cookieFile {
			store = s
			break
		}
	}
	if store == nil {
		return nil, fmt.Errorf("no kooky store found for %s", cookieFile)
	}
	defer store.Close()

	allCookies, err := store.TraverseCookies(
		kooky.DomainHasSuffix("google.com"),
	).ReadAllCookies(ctx)
	if err != nil {
		return nil, fmt.Errorf("read cookies: %w", err)
	}

	var httpCookies []*http.Cookie
	var foundPSID bool
	for _, c := range allCookies {
		hc := &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
		}
		httpCookies = append(httpCookies, hc)
		if c.Name == "__Secure-1PSID" && c.Value != "" {
			foundPSID = true
		}
	}

	if !foundPSID {
		return nil, fmt.Errorf("__Secure-1PSID not found; are you logged into gemini.google.com in %s?", b.browser)
	}

	return httpCookies, nil
}

func browserCookiePath(browser, profile string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		switch browser {
		case "brave":
			return filepath.Join(home, "Library", "Application Support",
				"BraveSoftware", "Brave-Browser", profile, "Cookies"), nil
		case "chrome":
			return filepath.Join(home, "Library", "Application Support",
				"Google", "Chrome", profile, "Cookies"), nil
		}
	case "linux":
		switch browser {
		case "brave":
			return filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser", profile, "Cookies"), nil
		case "chrome":
			return filepath.Join(home, ".config", "google-chrome", profile, "Cookies"), nil
		}
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		switch browser {
		case "brave":
			return filepath.Join(localAppData, "BraveSoftware", "Brave-Browser", "User Data", profile, "Cookies"), nil
		case "chrome":
			return filepath.Join(localAppData, "Google", "Chrome", "User Data", profile, "Cookies"), nil
		}
	}

	return "", fmt.Errorf("unsupported browser %q on %s", browser, runtime.GOOS)
}
