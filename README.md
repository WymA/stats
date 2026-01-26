# Investment Dashboard Generator

[![Daily Report](https://github.com/WymA/stats/actions/workflows/daily-report.yml/badge.svg?branch=master)](https://github.com/WymA/stats/actions/workflows/daily-report.yml) [![Live Dashboard](https://img.shields.io/badge/Live-Dashboard-16a34a)](https://stats.matthias2wym.com/)

This is a pure OpenCode Vibe Coding project. This project fetches public investment data daily and generates a single-page HTML dashboard in `public/index.html`.


## Run locally

```bash
go run .
```

## GitHub Actions

The workflow in `.github/workflows/daily-report.yml` runs every day and commits the refreshed HTML report.

## Everything Else

See `AGENTS.md`.
