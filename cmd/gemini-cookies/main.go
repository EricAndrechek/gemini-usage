// Command gemini-cookies extracts Google cookies from a browser and prints
// them as JSON to stdout. This is a stable helper binary for use with
// gemini.ExecCookies() — compile it once, grant it macOS Keychain access,
// and your main program can be recompiled freely without re-prompting.
//
// Usage:
//
//	gemini-cookies -browser brave -profile "Profile 2"
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/ericandrechek/gemini-usage/gemini"
)

type cookieJSON struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	HttpOnly bool   `json:"http_only"`
}

func main() {
	browser := flag.String("browser", "brave", "browser to read cookies from (brave, chrome)")
	profile := flag.String("profile", "", "browser profile name (default: auto)")
	flag.Parse()

	var provider gemini.CookieProvider
	switch *browser {
	case "brave":
		provider = gemini.BraveCookies(*profile)
	case "chrome":
		provider = gemini.ChromeCookies(*profile)
	default:
		log.Fatalf("unsupported browser: %s", *browser)
	}

	cookies, err := provider.Cookies(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	out := make([]cookieJSON, len(cookies))
	for i, c := range cookies {
		out[i] = toCookieJSON(c)
	}

	data, _ := json.Marshal(out)
	fmt.Println(string(data))
}

func toCookieJSON(c *http.Cookie) cookieJSON {
	return cookieJSON{
		Name:     c.Name,
		Value:    c.Value,
		Domain:   c.Domain,
		Path:     c.Path,
		Secure:   c.Secure,
		HttpOnly: c.HttpOnly,
	}
}
