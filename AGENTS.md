# Agent Guidance for This Repo

This file is for automated coding agents working in this repository. Follow these
conventions to keep changes consistent, safe, and easy to review.

## Project Overview
- Go CLI generates a static HTML dashboard into `public/index.html`.
- GitHub Actions runs daily (`.github/workflows/daily-report.yml`) and commits the
  refreshed report.
- Data sources:
  - Crypto pricing: CoinGecko API.
  - US indices + stocks: Stooq CSV endpoints.
  - Fear & Greed: alternative.me API.
- Main entry point: `main.go`.

## Build / Run / Test Commands
Use Go 1.22.

Build:
- `go build ./...`

Run locally:
- `go run .`

Format:
- `gofmt -w .`

Static analysis (no dedicated linter configured):
- `go vet ./...`

Tests:
- `go test ./...`

Run a single test:
- `go test ./... -run TestName`
- `go test ./... -run '^TestName$'`

No additional test runner or lint config is defined.

## Output / Generated Files
- The report HTML is generated to `public/index.html`.
- Do not hand-edit generated HTML; update the template in `main.go` instead.
- `public/.gitkeep` keeps the output directory in git.

## Code Style Guidelines

### Formatting
- Always run `gofmt` on modified Go files.
- Use tabs for indentation in Go (gofmt will handle this).
- Keep line length reasonable; wrap long string literals with formatting helpers
  where practical.

### Imports
- Use standard gofmt import grouping:
  - Standard library only (current code has no external packages).
- Keep imports minimal; remove unused imports.
- Prefer explicit imports over dot/blank imports unless required.

### Naming
- Exported types/functions use PascalCase; unexported use camelCase.
- Keep names short but descriptive: `fetchCoins`, `fetchStooqHistory`.
- Use `URL` in constants/vars for URLs (`cryptoURL`), not `Url`.

### Types and Structs
- Use concrete struct types for data decoded from APIs (see `Coin`,
  `IndexSnapshot`, `StockSnapshot`, `FearGreedSnapshot`).
- Keep JSON struct tags accurate and minimal.
- Prefer `float64` for numeric market data to avoid rounding errors.

### Error Handling
- Always check `error` returns; fail fast in `main` with a clear message.
- When returning errors from helpers, wrap with context using `fmt.Errorf` and
  `%w` where helpful.
- Treat non-2xx HTTP responses as errors.

### HTTP / IO
- Use a reasonable timeout on HTTP clients (`15s` is current standard).
- Close response bodies with `defer resp.Body.Close()` immediately after checking
  for errors.
- Validate data length before computing indicators (e.g., MA/EMA/MACD).

### Time
- Use explicit format strings in `time.Format` and keep them consistent.
- Prefer UTC for API timestamps unless local time is specifically needed.

### HTML Template
- The HTML template lives in `main.go` as a raw string literal.
- Keep the dashboard a single HTML file (no additional assets).
- Maintain semantic sections and keep CSS in the `<style>` tag.
- Avoid adding external dependencies (no JS/CSS CDN) unless required.

### Indicators
- MA/EMA periods are computed over the most recent values.
- MACD uses EMA(12), EMA(26), signal EMA(9).
- Ensure enough history before computing indicators; return an error otherwise.

## Git / Workflow Notes
- The action commits `public/index.html` only.
- Do not add secrets or API keys to the repo.
- Avoid changing GitHub Actions schedule unless requested.

## Adding New Data Sources
- Prefer public, no-auth endpoints.
- Document the source in the footer and in this file if you add new providers.
- Keep network calls minimal to avoid rate limits in daily runs.

## Repo Structure
- `main.go`: data fetch, calculations, template rendering.
- `public/`: generated HTML output (committed daily by GitHub Actions).
- `.github/workflows/daily-report.yml`: schedule + CI job.
- `README.md`: minimal usage info.

## Notes on Missing Tooling
- No lint config (golangci-lint, staticcheck, etc.) is present.
- No test suite is defined yet. Keep functions small to be testable if tests are
  added later.

## When Making Changes
- Update the HTML template if display changes are needed.
- Update `README.md` when user-facing usage changes.
- Keep changes focused; avoid unrelated refactors.
