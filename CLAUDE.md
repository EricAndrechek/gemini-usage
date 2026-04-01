# gemini-usage

Tracks Google Gemini prompt quota usage by reverse-engineering the Gemini web app's internal APIs.

## Architecture

- **Go package** (`gemini/`): Importable library with pluggable CookieProvider interface.
- **CLI binary** (`cmd/gemini-usage/`): One-shot and continuous polling modes.
- **Python prototype** (`_python/`): Reference implementation with FastAPI HTTP server.
- **Recon scripts** (`_docs/recon/`): Python scripts for API exploration when Google changes things.

## How it works

1. CookieProvider fetches auth cookies (browser extraction, file, or static values)
2. Fetches `gemini.google.com/app` to extract session tokens (SNlM0e, cfb2h, FdrFJe)
3. Calls `MaZiqc` (LIST_CHATS) batchexecute RPC to get conversation list
4. Calls `hNvQHb` (READ_CHAT) for each today's conversation
5. Classifies each turn by model variant ID at `turn[2][4]` + presence of thoughts at `turn[3][0][0][37][0][0]`

## Key files to modify when things break

- `gemini/config.go` — Model variant IDs (hex strings), classification logic
- `gemini/client.go` — RPC IDs, payload formats, response parsing paths, client options
- `gemini/cookies.go` — CookieProvider interface, caching
- `gemini/cookies_browser.go` — Brave/Chrome cookie extraction (uses kooky, build tag `!nobrowser`)
- `gemini/parse.go` — Google batchexecute response frame parser
- `_docs/GEMINI_API_INTERNALS.md` — Full reverse-engineering documentation

## Building & testing

```bash
go build ./...                                    # build all
go run ./cmd/gemini-usage/                        # one-shot test
go run ./cmd/gemini-usage/ -refresh               # force fresh cookies
go run ./cmd/gemini-usage/ -browser chrome        # use Chrome
go run ./cmd/gemini-usage/ -cookies cookies.json  # use cookie file
go build -tags nobrowser ./gemini/                # build without kooky dependency
go vet ./...                                      # lint
```

## Common fixes

- **Model IDs changed**: Run `_docs/recon/recon4.py` to discover new IDs, update `config.go`
- **RPC IDs changed**: Run `_docs/recon/recon2.py` to find new RPC IDs, update `client.go`
- **Response structure changed**: Run `_docs/recon/recon3.py`, check field indices in `client.go`
- **Auth broken**: Check cookie extraction, run `_docs/recon/recon.py` to test basic connectivity
