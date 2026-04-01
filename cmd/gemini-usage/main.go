// Command gemini-usage prints Gemini quota usage as JSON.
//
// Usage:
//
//	gemini-usage                       # one-shot: print usage and exit
//	gemini-usage -poll                 # poll every 60s, print each update
//	gemini-usage -poll -interval 30s
//	gemini-usage -refresh              # force re-read cookies from Brave
//	gemini-usage -browser chrome       # use Chrome instead of Brave
//	gemini-usage -cookies cookies.json # use a cookie file directly
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/ericandrechek/gemini-usage/gemini"
)

func main() {
	poll := flag.Bool("poll", false, "continuously poll and print updates")
	interval := flag.Duration("interval", gemini.DefaultPollInterval, "polling interval")
	refresh := flag.Bool("refresh", false, "force re-read cookies from browser (triggers Keychain prompt)")
	browser := flag.String("browser", "brave", "browser to read cookies from (brave, chrome)")
	profile := flag.String("profile", "", "browser profile directory name (default: auto)")
	cookieFile := flag.String("cookies", "", "path to cookie JSON file (bypasses browser)")
	flag.Parse()

	// Build the cookie provider
	var provider gemini.CookieProvider
	if *cookieFile != "" {
		provider = gemini.FileCookies(*cookieFile)
	} else {
		var inner gemini.CookieProvider
		switch *browser {
		case "brave":
			inner = gemini.BraveCookies(*profile)
		case "chrome":
			inner = gemini.ChromeCookies(*profile)
		default:
			log.Fatalf("unsupported browser: %s (use brave or chrome)", *browser)
		}
		// Resolve the actual profile name for cache path keying
		resolvedProfile := *profile
		if resolvedProfile == "" {
			if *browser == "brave" {
				resolvedProfile = "Profile 1"
			} else {
				resolvedProfile = "Default"
			}
		}
		cacheName := fmt.Sprintf("cookies-%s-%s.json", *browser, strings.ReplaceAll(resolvedProfile, " ", "_"))
		provider = gemini.CachedProvider(inner,
			gemini.WithCachePath(filepath.Join(gemini.DefaultCacheDir(), cacheName)),
		)
	}

	if *refresh {
		if inv, ok := provider.(gemini.Invalidator); ok {
			inv.Invalidate()
			log.Println("Cookie cache invalidated")
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if *poll {
		runPoll(ctx, provider, *interval)
	} else {
		runOnce(ctx, provider)
	}
}

func runOnce(ctx context.Context, provider gemini.CookieProvider) {
	usage, err := gemini.FetchUsage(ctx, provider)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	printJSON(usage)
}

func runPoll(ctx context.Context, provider gemini.CookieProvider, interval time.Duration) {
	tracker, err := gemini.NewTracker(ctx, provider, gemini.WithInterval(interval))
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	for usage := range tracker.Updates() {
		printJSON(usage)
	}
}

func printJSON(v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "json error: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
