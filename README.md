# gemini-usage

[![Go Reference](https://pkg.go.dev/badge/github.com/ericandrechek/gemini-usage/gemini.svg)](https://pkg.go.dev/github.com/ericandrechek/gemini-usage/gemini)
[![Go Report Card](https://goreportcard.com/badge/github.com/ericandrechek/gemini-usage)](https://goreportcard.com/report/github.com/ericandrechek/gemini-usage)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Track your Google Gemini prompt quota usage in semi-realtime.

Google provides no way to see how many Pro or Thinking prompts you've used today — they only warn you when you're "close" to the limit. This tool counts your actual usage by reading your conversation history via Gemini's internal web APIs.

Works across all devices (desktop + phone) since it reads account-level conversation data.

## Install

```bash
# Install the CLI
go install github.com/ericandrechek/gemini-usage/cmd/gemini-usage@latest

# Use as a library
go get github.com/ericandrechek/gemini-usage
```

## Quick Start (CLI)

```bash
# One-shot: print current usage and exit
gemini-usage

# Continuous polling (prints JSON every 60s)
gemini-usage -poll

# Custom interval
gemini-usage -poll -interval 30s

# Use a specific browser profile (quote the name if it contains spaces)
gemini-usage -profile "Profile 2"

# Use Chrome instead of Brave
gemini-usage -browser chrome

# Use a cookie file directly (no browser needed)
gemini-usage -cookies /path/to/cookies.json

# Force re-read cookies from browser (triggers macOS Keychain prompt)
gemini-usage -refresh
```

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-poll` | `false` | Continuously poll and print updates |
| `-interval` | `60s` | Polling interval (used with `-poll`) |
| `-browser` | `brave` | Browser to read cookies from (`brave` or `chrome`) |
| `-profile` | `"Profile 1"` (Brave) / `"Default"` (Chrome) | Browser profile directory name |
| `-cookies` | | Path to cookie JSON file (bypasses browser) |
| `-refresh` | `false` | Force re-read cookies from browser |

Output:
```json
{
  "pro": { "used": 37, "limit": 100, "remaining": 63 },
  "thinking": { "used": 2, "limit": 300, "remaining": 298 },
  "flash": 3,
  "last_updated": "2026-04-01T19:38:39.207403Z"
}
```

### Build from Source

```bash
git clone https://github.com/ericandrechek/gemini-usage.git
cd gemini-usage
go build -o gemini-usage ./cmd/gemini-usage/
go build -o gemini-cookies ./cmd/gemini-cookies/
```

## Prerequisites

- **Brave** or **Chrome** on macOS/Linux/Windows with an active login to gemini.google.com
- **Google Workspace Business Standard** (or any plan with Gemini quotas)
- **Go 1.22+** for building

The first run reads cookies from the browser's encrypted cookie database, which triggers a macOS Keychain prompt on Mac. After that, cookies are cached for 12 hours.

## Library Usage

Full API documentation: [pkg.go.dev/github.com/ericandrechek/gemini-usage/gemini](https://pkg.go.dev/github.com/ericandrechek/gemini-usage/gemini)

### Cookie Providers

The library uses a `CookieProvider` interface for authentication. Several built-in providers are available:

```go
// Browser-based (reads from encrypted cookie DB, may trigger Keychain prompt)
provider := gemini.BraveCookies("")              // Brave, default profile ("Profile 1")
provider := gemini.ChromeCookies("")             // Chrome, default profile ("Default")
provider := gemini.BraveCookies("Profile 2")     // specific profile

// File-based (reads from a JSON cookie file)
provider := gemini.FileCookies("/path/to/cookies.json")

// Static (hardcoded cookie values)
provider := gemini.StaticCookies("PSID_VALUE", "PSIDTS_VALUE")

// Wrap any provider with caching to avoid repeated Keychain prompts
provider := gemini.CachedProvider(
    gemini.BraveCookies(""),
    gemini.WithCacheTTL(6*time.Hour),
)
```

### One-shot Fetch

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/ericandrechek/gemini-usage/gemini"
)

func main() {
    ctx := context.Background()
    provider := gemini.CachedProvider(gemini.BraveCookies(""))

    usage, err := gemini.FetchUsage(ctx, provider)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Pro: %d/%d remaining\n", usage.Pro.Remaining, usage.Pro.Limit)
    fmt.Printf("Thinking: %d/%d remaining\n", usage.Thinking.Remaining, usage.Thinking.Limit)
}
```

### Continuous Tracker

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

provider := gemini.CachedProvider(gemini.BraveCookies(""))
tracker, err := gemini.NewTracker(ctx, provider,
    gemini.WithInterval(30*time.Second),
    gemini.WithCookieRefresh(6*time.Hour),
)
if err != nil {
    log.Fatal(err)
}

for usage := range tracker.Updates() {
    fmt.Printf("Pro: %d/%d, Thinking: %d/%d\n",
        usage.Pro.Used, usage.Pro.Limit,
        usage.Thinking.Used, usage.Thinking.Limit)
}
```

### Tracker with Callback

```go
tracker, err := gemini.NewTracker(ctx, provider,
    gemini.WithInterval(60*time.Second),
    gemini.WithCallback(func(u *gemini.Usage) {
        log.Printf("Pro: %d remaining", u.Pro.Remaining)
    }),
)
```

### HTTP Server (Gin)

```go
package main

import (
    "context"
    "log"
    "sync"
    "time"

    "github.com/ericandrechek/gemini-usage/gemini"
    "github.com/gin-gonic/gin"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    provider := gemini.CachedProvider(gemini.BraveCookies(""))

    var (
        mu          sync.RWMutex
        latestUsage *gemini.Usage
    )

    tracker, err := gemini.NewTracker(ctx, provider,
        gemini.WithInterval(60*time.Second),
        gemini.WithCallback(func(u *gemini.Usage) {
            mu.Lock()
            latestUsage = u
            mu.Unlock()
        }),
    )
    if err != nil {
        log.Fatal(err)
    }
    // Drain the channel since we're using callbacks
    go func() { for range tracker.Updates() {} }()

    r := gin.Default()

    r.GET("/usage", func(c *gin.Context) {
        mu.RLock()
        defer mu.RUnlock()
        if latestUsage == nil {
            c.JSON(503, gin.H{"error": "not ready"})
            return
        }
        c.JSON(200, latestUsage)
    })

    r.Run(":8420")
}
```

## Client Options

```go
gemini.NewClient(ctx, provider,
    gemini.WithHTTPTimeout(30*time.Second),     // HTTP request timeout
    gemini.WithProLimit(100),                    // daily Pro quota
    gemini.WithThinkingLimit(300),               // daily Thinking quota
    gemini.WithResetTimezone(loc),               // quota reset timezone (default: system local)
    gemini.WithLogger(slog.Default()),           // structured logger
)
```

These options can also be passed to `NewTracker` via `WithClientOptions(...)`.

## macOS Keychain / launchd Services

When using this library inside a long-running service (e.g. a macOS `launchd` daemon), direct browser cookie extraction is problematic: Keychain prompts can't appear without a GUI session, and recompiling your program changes its code signature, re-triggering permission requests.

The solution is a stable helper binary that handles Keychain access:

```bash
# Install the helper binary
go install github.com/ericandrechek/gemini-usage/cmd/gemini-cookies@latest

# Grant it Keychain access by running it once interactively
gemini-cookies -browser brave -profile "Profile 2"
# (approve the Keychain prompt — the binary is now authorized)
```

Then in your service code, use [`ExecCookies`](https://pkg.go.dev/github.com/ericandrechek/gemini-usage/gemini#ExecCookies) to shell out to the helper:

```go
provider := gemini.CachedProvider(
    gemini.ExecCookies("/path/to/gemini-cookies",
        "-browser", "brave", "-profile", "Profile 2"),
)
```

Your service binary can be recompiled freely — only the helper binary's signature matters to Keychain. Find the installed binary path with `which gemini-cookies` or `go env GOPATH`/bin.

## Without Browser Dependencies

If you don't need browser cookie extraction (e.g. on a headless server), build with:

```bash
go build -tags nobrowser ./...
```

Then provide cookies via `FileCookies`, `StaticCookies`, or `ExecCookies`.

## How It Works

1. Extracts Google auth cookies from browser's encrypted cookie database
2. Authenticates by fetching `gemini.google.com/app` to get session tokens
3. Calls `LIST_CHATS` internal RPC to get today's conversations
4. For each conversation, calls `READ_CHAT` to get individual turns
5. Classifies each turn by model type using the variant ID embedded in the turn data
6. Pro-only variant IDs count against Pro quota; shared IDs with "thoughts" count against Thinking quota; everything else is Flash (free)

See [_docs/GEMINI_API_INTERNALS.md](_docs/GEMINI_API_INTERNALS.md) for the full reverse-engineering documentation.

## Troubleshooting

**"Keychain prompt every time"**: The CLI wraps browser providers with `CachedProvider` automatically. Run once with `-refresh` to force a fresh cache.

**"__Secure-1PSID not found"**: Make sure you're logged into gemini.google.com in your browser and using the correct profile (e.g. `-profile "Profile 2"` if your Google account is on a non-default profile).

**"Failed to extract tokens from init page"**: Cookies have expired. Run with `-refresh`.

**Counts seem wrong**: Google may have changed model variant IDs. See [_docs/GEMINI_API_INTERNALS.md](_docs/GEMINI_API_INTERNALS.md) for how to re-discover them.

## Project Structure

```
├── gemini/                  # Importable Go package
│   ├── client.go            # Gemini API client (batchexecute RPCs)
│   ├── config.go            # Model variant IDs, classification logic
│   ├── cookies.go           # CookieProvider interface + StaticCookies, FileCookies, CachedProvider
│   ├── cookies_browser.go   # BraveCookies, ChromeCookies (build tag: !nobrowser)
│   ├── cookies_exec.go      # ExecCookies (shell out to helper binary)
│   ├── parse.go             # Google batchexecute response parser
│   ├── tracker.go           # Continuous polling tracker
│   └── usage.go             # FetchUsage + types (QuotaInfo, Usage)
├── cmd/gemini-usage/        # CLI binary
│   └── main.go
├── cmd/gemini-cookies/      # Keychain helper binary (stable for launchd)
│   └── main.go
├── _python/                 # Python prototype (reference)
├── _docs/                   # API internals docs + recon scripts
│   ├── GEMINI_API_INTERNALS.md
│   └── recon/
├── go.mod
└── README.md
```
