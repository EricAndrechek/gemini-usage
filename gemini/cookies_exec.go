package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
)

// ExecCookies returns a CookieProvider that obtains cookies by executing
// an external binary and reading JSON from its stdout. This is useful for
// macOS launchd services where Keychain prompts cannot appear interactively:
// compile the helper binary (cmd/gemini-cookies) once, grant it Keychain
// access, and point ExecCookies at it. Recompiling your own program won't
// re-trigger Keychain prompts since the helper binary stays unchanged.
//
//	provider := gemini.CachedProvider(
//	    gemini.ExecCookies("/usr/local/bin/gemini-cookies", "-browser", "brave", "-profile", "Profile 2"),
//	)
func ExecCookies(binPath string, args ...string) CookieProvider {
	return &execProvider{binPath: binPath, args: args}
}

type execProvider struct {
	binPath string
	args    []string
}

func (e *execProvider) Cookies(ctx context.Context) ([]*http.Cookie, error) {
	cmd := exec.CommandContext(ctx, e.binPath, e.args...)
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		return nil, fmt.Errorf("exec cookies %s: %w: %s", e.binPath, err, stderr)
	}

	var cookies []cachedCookie
	if err := json.Unmarshal(out, &cookies); err != nil {
		return nil, fmt.Errorf("parse exec cookie output: %w", err)
	}
	if len(cookies) == 0 {
		return nil, fmt.Errorf("exec cookies: no cookies returned")
	}

	return cacheToCookies(cookies), nil
}
