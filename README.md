# Investment Dashboard Generator

This project fetches public investment data daily and generates a single-page HTML dashboard in `public/index.html`.

## Run locally

```bash
go run .
```

## GitHub Actions

The workflow in `.github/workflows/daily-report.yml` runs every day and commits the refreshed HTML report.
