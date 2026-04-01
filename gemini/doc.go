// Package gemini tracks Google Gemini prompt quota usage by reading
// conversation history via Gemini's internal web APIs.
//
// Google provides no way to see how many Pro or Thinking prompts you've
// used today. This package counts your actual usage by authenticating
// with browser cookies, listing today's conversations, and classifying
// each turn by model type.
//
// # Authentication
//
// All operations require a [CookieProvider] to supply Google session cookies.
// Several built-in providers are available:
//
//   - [BraveCookies] / [ChromeCookies] — read from the browser's encrypted cookie database
//   - [FileCookies] — read from a JSON file
//   - [StaticCookies] — use hardcoded cookie values
//   - [ExecCookies] — shell out to a helper binary (useful for launchd services)
//
// Browser providers should be wrapped with [CachedProvider] to avoid
// repeated macOS Keychain prompts:
//
//	provider := gemini.CachedProvider(gemini.BraveCookies("Profile 2"))
//
// # One-shot usage
//
// [FetchUsage] performs a single query and returns the current quota snapshot:
//
//	usage, err := gemini.FetchUsage(ctx, provider)
//	fmt.Printf("Pro: %d/%d remaining\n", usage.Pro.Remaining, usage.Pro.Limit)
//
// # Continuous tracking
//
// [NewTracker] polls at a regular interval and delivers results via a channel
// and/or callback. It automatically refreshes cookies and stops when the
// context is cancelled:
//
//	tracker, err := gemini.NewTracker(ctx, provider,
//	    gemini.WithInterval(30*time.Second),
//	    gemini.WithCookieRefresh(6*time.Hour),
//	)
//	for usage := range tracker.Updates() {
//	    // ...
//	}
//
// # Configuration
//
// Both [NewClient] and [NewTracker] accept functional options for
// customization: [WithProLimit], [WithThinkingLimit], [WithResetTimezone],
// [WithHTTPTimeout], [WithLogger], [WithInterval], [WithCookieRefresh],
// and [WithCallback].
package gemini
