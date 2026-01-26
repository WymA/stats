package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	outputDir  = "public"
	outputFile = "index.html"
	cryptoURL  = "https://api.coingecko.com/api/v3/coins/markets?vs_currency=usd&ids=bitcoin,ethereum"
	historyDir = "history"
)

type Coin struct {
	ID                       string  `json:"id"`
	Symbol                   string  `json:"symbol"`
	Name                     string  `json:"name"`
	CurrentPrice             float64 `json:"current_price"`
	PriceChangePercentage24h float64 `json:"price_change_percentage_24h"`
	LastUpdated              string  `json:"last_updated"`
}

type PageData struct {
	GeneratedAt string
	Coins       []Coin
	Indices     []IndexSnapshot
	FearGreed   FearGreedSnapshot
	Stocks      []StockSnapshot
	History     []HistoryEntry
}

type IndexSnapshot struct {
	Name   string
	Symbol string
	Close  float64
	MA20   float64
	EMA20  float64
}

type FearGreedSnapshot struct {
	Value     string
	Category  string
	UpdatedAt string
}

type StockSnapshot struct {
	Name         string
	Symbol       string
	Close        float64
	MACD         float64
	Signal       float64
	Hist         float64
	BullishCross bool
}

type HistoryEntry struct {
	Date string
	File string
}

func main() {
	coins, err := fetchCoins()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch data: %v\n", err)
		os.Exit(1)
	}

	indices := []IndexSnapshot{
		{Name: "S&P 500", Symbol: "^SPX", Close: 0, MA20: 0, EMA20: 0},
		{Name: "Nasdaq 100", Symbol: "^NDX", Close: 0, MA20: 0, EMA20: 0},
	}

	stocks := []StockSnapshot{
		{Name: "Tesla", Symbol: "tsla.us"},
		{Name: "Alphabet", Symbol: "goog.us"},
		{Name: "NVIDIA", Symbol: "nvda.us"},
		{Name: "Occidental Petroleum", Symbol: "oxy.us"},
		{Name: "Coca-Cola", Symbol: "ko.us"},
		{Name: "PDD Holdings", Symbol: "pdd.us"},
		{Name: "SPDR Gold Shares", Symbol: "gld.us"},
		{Name: "JPY/USD", Symbol: "jpyusd"},
	}

	for i, index := range indices {
		snapshot, err := fetchIndexSnapshot(index)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch index %s: %v\n", index.Name, err)
			os.Exit(1)
		}
		indices[i] = snapshot
	}

	for i, stock := range stocks {
		snapshot, err := fetchStockSnapshot(stock)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch stock %s: %v\n", stock.Name, err)
			os.Exit(1)
		}
		stocks[i] = snapshot
	}

	fearGreed, err := fetchFearGreed()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch fear & greed: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create output dir: %v\n", err)
		os.Exit(1)
	}

	historyPath := filepath.Join(outputDir, historyDir)
	if err := os.MkdirAll(historyPath, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create history dir: %v\n", err)
		os.Exit(1)
	}

	historyEntries, err := loadHistoryEntries(historyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load history: %v\n", err)
		os.Exit(1)
	}

	todayFilename := time.Now().Format("20060102") + ".html"
	todayEntry := HistoryEntry{
		Date: time.Now().Format("2006-01-02"),
		File: filepath.ToSlash(filepath.Join(historyDir, todayFilename)),
	}
	historyEntries = upsertHistoryEntry(historyEntries, todayEntry)

	outputPath := filepath.Join(outputDir, outputFile)
	file, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create output file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	data := PageData{
		GeneratedAt: time.Now().Format("2006-01-02 15:04 MST"),
		Coins:       coins,
		Indices:     indices,
		FearGreed:   fearGreed,
		Stocks:      stocks,
		History:     historyEntries,
	}

	var buffer bytes.Buffer
	if err := pageTemplate.Execute(&buffer, data); err != nil {
		fmt.Fprintf(os.Stderr, "render template: %v\n", err)
		os.Exit(1)
	}

	if _, err := file.Write(buffer.Bytes()); err != nil {
		fmt.Fprintf(os.Stderr, "write output file: %v\n", err)
		os.Exit(1)
	}

	historyFilePath := filepath.Join(historyPath, todayFilename)
	historyFile, err := os.Create(historyFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create history file: %v\n", err)
		os.Exit(1)
	}
	defer historyFile.Close()
	if _, err := historyFile.Write(buffer.Bytes()); err != nil {
		fmt.Fprintf(os.Stderr, "write history file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Report generated: %s\n", outputPath)
}

func fetchCoins() ([]Coin, error) {
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(cryptoURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var coins []Coin
	if err := json.NewDecoder(resp.Body).Decode(&coins); err != nil {
		return nil, err
	}

	return coins, nil
}

func fetchIndexSnapshot(index IndexSnapshot) (IndexSnapshot, error) {
	prices, err := fetchStooqHistory(index.Symbol)
	if err != nil {
		return IndexSnapshot{}, err
	}
	if len(prices) < 20 {
		return IndexSnapshot{}, fmt.Errorf("insufficient data for %s", index.Symbol)
	}

	close := prices[len(prices)-1]
	ma20 := simpleMovingAverage(prices, 20)
	ema20 := exponentialMovingAverage(prices, 20)

	index.Close = close
	index.MA20 = ma20
	index.EMA20 = ema20

	return index, nil
}

func fetchStockSnapshot(stock StockSnapshot) (StockSnapshot, error) {
	prices, err := fetchStooqHistory(stock.Symbol)
	if err != nil {
		return StockSnapshot{}, err
	}
	if len(prices) < 40 {
		return StockSnapshot{}, fmt.Errorf("insufficient data for %s", stock.Symbol)
	}

	macd, signal, hist, bullishCross := macdIndicator(prices, 12, 26, 9)
	stock.Close = prices[len(prices)-1]
	stock.MACD = macd
	stock.Signal = signal
	stock.Hist = hist
	stock.BullishCross = bullishCross

	return stock, nil
}

func fetchStooqHistory(symbol string) ([]float64, error) {
	url := fmt.Sprintf("https://stooq.com/q/d/l/?s=%s&i=d", symbol)
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = -1

	_, err = reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	var prices []float64
	for {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("read record: %w", err)
		}
		if len(record) < 5 {
			continue
		}
		close, err := strconv.ParseFloat(record[4], 64)
		if err != nil {
			continue
		}
		prices = append(prices, close)
	}

	if len(prices) == 0 {
		return nil, fmt.Errorf("no price data")
	}

	return prices, nil
}

func simpleMovingAverage(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}

	start := len(prices) - period
	var sum float64
	for _, price := range prices[start:] {
		sum += price
	}

	return sum / float64(period)
}

func exponentialMovingAverage(prices []float64, period int) float64 {
	emaSeries := exponentialMovingAverageSeries(prices, period)
	if len(emaSeries) == 0 {
		return 0
	}

	return emaSeries[len(emaSeries)-1]
}

func exponentialMovingAverageSeries(prices []float64, period int) []float64 {
	if len(prices) < period {
		return nil
	}

	alpha := 2.0 / float64(period+1)
	var emaSeries []float64
	var seed float64
	for i := 0; i < period; i++ {
		seed += prices[i]
	}
	ema := seed / float64(period)
	emaSeries = append(emaSeries, ema)

	for i := period; i < len(prices); i++ {
		ema = alpha*prices[i] + (1-alpha)*ema
		emaSeries = append(emaSeries, ema)
	}

	return emaSeries
}

func macdIndicator(prices []float64, fastPeriod, slowPeriod, signalPeriod int) (float64, float64, float64, bool) {
	fast := exponentialMovingAverageSeries(prices, fastPeriod)
	slow := exponentialMovingAverageSeries(prices, slowPeriod)
	if len(fast) == 0 || len(slow) == 0 {
		return 0, 0, 0, false
	}

	offset := slowPeriod - fastPeriod
	if offset < 0 || len(fast) <= offset {
		return 0, 0, 0, false
	}

	macdSeries := make([]float64, 0, len(slow))
	for i := 0; i < len(slow); i++ {
		macdSeries = append(macdSeries, fast[i+offset]-slow[i])
	}

	signalSeries := exponentialMovingAverageSeries(macdSeries, signalPeriod)
	if len(signalSeries) < 2 || len(macdSeries) < 2 {
		return 0, 0, 0, false
	}

	macd := macdSeries[len(macdSeries)-1]
	signal := signalSeries[len(signalSeries)-1]
	prevMacd := macdSeries[len(macdSeries)-2]
	prevSignal := signalSeries[len(signalSeries)-2]
	bullishCross := prevMacd <= prevSignal && macd > signal
	return macd, signal, macd - signal, bullishCross
}

func formatPrice(value float64) string {
	if value == 0 {
		return "0.00"
	}
	if value < 1 {
		return fmt.Sprintf("%.5f", value)
	}
	return fmt.Sprintf("%.2f", value)
}

func fetchFearGreed() (FearGreedSnapshot, error) {
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get("https://api.alternative.me/fng/?limit=1")
	if err != nil {
		return FearGreedSnapshot{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FearGreedSnapshot{}, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var payload struct {
		Data []struct {
			Value               string `json:"value"`
			ValueClassification string `json:"value_classification"`
			Timestamp           string `json:"timestamp"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return FearGreedSnapshot{}, err
	}

	if len(payload.Data) == 0 {
		return FearGreedSnapshot{}, fmt.Errorf("no fear & greed data")
	}

	entry := payload.Data[0]
	stamp, err := strconv.ParseInt(entry.Timestamp, 10, 64)
	if err != nil {
		return FearGreedSnapshot{Value: entry.Value, Category: entry.ValueClassification, UpdatedAt: ""}, nil
	}

	return FearGreedSnapshot{
		Value:     entry.Value,
		Category:  entry.ValueClassification,
		UpdatedAt: time.Unix(stamp, 0).UTC().Format("2006-01-02"),
	}, nil
}

func loadHistoryEntries(historyPath string) ([]HistoryEntry, error) {
	entries, err := os.ReadDir(historyPath)
	if err != nil {
		return nil, err
	}

	var history []HistoryEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		date := strings.TrimSuffix(name, ".html")
		if len(date) != 8 {
			continue
		}
		history = append(history, HistoryEntry{
			Date: formatHistoryDate(date),
			File: filepath.ToSlash(filepath.Join(historyDir, name)),
		})
	}

	sort.Slice(history, func(i, j int) bool {
		return history[i].Date > history[j].Date
	})

	return history, nil
}

func upsertHistoryEntry(entries []HistoryEntry, entry HistoryEntry) []HistoryEntry {
	for i, existing := range entries {
		if existing.File == entry.File {
			entries[i] = entry
			return entries
		}
	}
	return append([]HistoryEntry{entry}, entries...)
}

func formatHistoryDate(value string) string {
	if len(value) != 8 {
		return value
	}
	return value[:4] + "-" + value[4:6] + "-" + value[6:]
}

var pageTemplate = template.Must(template.New("dashboard").Funcs(template.FuncMap{
	"formatPrice": formatPrice,
}).Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Daily Investment Snapshot</title>
    <style>
      :root {
        --ink: #111827;
        --muted: #6b7280;
        --accent: #2563eb;
        --accent-soft: #dbeafe;
        --surface: #f8fafc;
        --card: #ffffff;
        --bullish: #16a34a;
        --bullish-soft: #dcfce7;
        --shadow: rgba(15, 23, 42, 0.3);
        --border: rgba(148, 163, 184, 0.2);
      }
      body.dark {
        --ink: #e2e8f0;
        --muted: #94a3b8;
        --accent: #38bdf8;
        --accent-soft: rgba(56, 189, 248, 0.2);
        --surface: #0b1120;
        --card: #0f172a;
        --bullish: #4ade80;
        --bullish-soft: rgba(74, 222, 128, 0.16);
        --shadow: rgba(15, 23, 42, 0.6);
        --border: rgba(148, 163, 184, 0.18);
      }
      * { box-sizing: border-box; }
      body {
        margin: 0;
        font-family: "Segoe UI", "Helvetica Neue", Arial, sans-serif;
        color: var(--ink);
        background: radial-gradient(circle at top, #eff6ff, #ffffff 55%);
        transition: background 0.4s ease, color 0.3s ease;
      }
      body.dark {
        background: radial-gradient(circle at top, #172554, #0b1120 60%);
      }
      header {
        padding: 48px 24px 24px;
        text-align: center;
        position: relative;
      }
      .toolbar {
        position: absolute;
        top: 24px;
        right: 24px;
        display: flex;
        gap: 12px;
        align-items: center;
      }
      .toolbar button,
      .toolbar summary {
        font: inherit;
        cursor: pointer;
        background: var(--card);
        color: var(--ink);
        border: 1px solid var(--border);
        padding: 8px 12px;
        border-radius: 999px;
        box-shadow: 0 10px 30px -24px var(--shadow);
        list-style: none;
      }
      .toolbar details[open] summary {
        border-color: var(--accent);
      }
      .history-menu {
        position: relative;
      }
      .history-list {
        position: absolute;
        right: 0;
        margin-top: 10px;
        background: var(--card);
        border: 1px solid var(--border);
        border-radius: 12px;
        padding: 12px;
        min-width: 220px;
        max-height: 260px;
        overflow: auto;
        box-shadow: 0 20px 40px -30px var(--shadow);
      }
      .history-list a {
        display: block;
        padding: 6px 8px;
        color: var(--ink);
        text-decoration: none;
        border-radius: 8px;
      }
      .history-list a:hover {
        background: var(--accent-soft);
      }
      header h1 {
        margin: 0 0 8px;
        font-size: 2.2rem;
      }
      header p {
        margin: 0;
        color: var(--muted);
      }
      main {
        max-width: 1080px;
        margin: 0 auto 48px;
        padding: 0 24px 32px;
      }
      section {
        margin-bottom: 32px;
      }
      .section-title {
        font-size: 1.1rem;
        text-transform: uppercase;
        letter-spacing: 0.18em;
        color: var(--muted);
        margin-bottom: 16px;
      }
      .grid {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
        gap: 20px;
      }
      .card {
        background: var(--card);
        border-radius: 16px;
        padding: 20px;
        box-shadow: 0 20px 40px -30px var(--shadow);
        border: 1px solid var(--border);
      }
      .card.bullish {
        border-color: var(--bullish);
        box-shadow: 0 24px 50px -30px rgba(22, 163, 74, 0.45);
      }
      .card h2 {
        margin: 0 0 4px;
        font-size: 1.2rem;
      }
      .metric {
        font-size: 0.95rem;
        color: var(--muted);
        margin: 8px 0;
      }
      .symbol {
        text-transform: uppercase;
        font-size: 0.8rem;
        color: var(--muted);
        letter-spacing: 0.08em;
      }
      .price {
        margin: 16px 0 8px;
        font-size: 1.7rem;
        color: var(--accent);
      }
      .change {
        font-weight: 600;
        padding: 6px 10px;
        border-radius: 999px;
        display: inline-block;
        background: var(--accent-soft);
      }
      .badge {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 0.75rem;
        text-transform: uppercase;
        letter-spacing: 0.12em;
        padding: 6px 10px;
        border-radius: 999px;
        font-weight: 700;
      }
      .badge.bullish {
        background: var(--bullish-soft);
        color: var(--bullish);
      }
      .fear-card {
        text-align: center;
      }
      .fear-value {
        font-size: 2.2rem;
        margin: 12px 0 4px;
        color: var(--accent);
      }
      .fear-label {
        color: var(--muted);
        font-weight: 600;
      }
      .stock-metrics {
        display: flex;
        gap: 12px;
        flex-wrap: wrap;
        margin-top: 12px;
        font-size: 0.9rem;
        color: var(--muted);
      }
      footer {
        margin-top: 32px;
        text-align: center;
        color: var(--muted);
        font-size: 0.9rem;
      }
    </style>
  </head>
  <body>
    <header>
      <div class="toolbar">
        <details class="history-menu">
          <summary>History</summary>
          <div class="history-list">
            {{range .History}}
            <a href="{{.File}}">{{.Date}}</a>
            {{end}}
          </div>
        </details>
        <button id="theme-toggle" type="button">Dark mode</button>
      </div>
      <h1>Daily Investment Snapshot</h1>
      <p>Generated {{.GeneratedAt}}</p>
    </header>
    <main>
      <section>
        <div class="section-title">US Indices</div>
        <div class="grid">
          {{range .Indices}}
          <article class="card">
            <div class="symbol">{{.Symbol}}</div>
            <h2>{{.Name}}</h2>
            <div class="price">${{printf "%.2f" .Close}}</div>
            <div class="metric">MA20: ${{printf "%.2f" .MA20}}</div>
            <div class="metric">EMA20: ${{printf "%.2f" .EMA20}}</div>
          </article>
          {{end}}
        </div>
      </section>
      <section>
        <div class="section-title">Stocks &amp; Macro</div>
        <div class="grid">
          {{range .Stocks}}
          <article class="card {{if .BullishCross}}bullish{{end}}">
            <div class="symbol">{{.Symbol}}</div>
            <h2>{{.Name}}</h2>
            <div class="price">${{formatPrice .Close}}</div>
            {{if .BullishCross}}
            <div class="badge bullish">MACD Bullish</div>
            {{end}}
            <div class="stock-metrics">
              <span>MACD {{printf "%.2f" .MACD}}</span>
              <span>Signal {{printf "%.2f" .Signal}}</span>
              <span>Hist {{printf "%.2f" .Hist}}</span>
            </div>
          </article>
          {{end}}
        </div>
      </section>
      <section>
        <div class="section-title">Fear &amp; Greed</div>
        <div class="grid">
          <article class="card fear-card">
            <div class="symbol">Market Sentiment</div>
            <div class="fear-value">{{.FearGreed.Value}}</div>
            <div class="fear-label">{{.FearGreed.Category}}</div>
            {{if .FearGreed.UpdatedAt}}
            <div class="metric">Updated {{.FearGreed.UpdatedAt}}</div>
            {{end}}
          </article>
        </div>
      </section>
      <section>
        <div class="section-title">Crypto Snapshot</div>
        <div class="grid">
          {{range .Coins}}
          <article class="card">
            <div class="symbol">{{.Symbol}}</div>
            <h2>{{.Name}}</h2>
            <div class="price">${{printf "%.2f" .CurrentPrice}}</div>
            <div class="change">24h {{printf "%.2f" .PriceChangePercentage24h}}%</div>
          </article>
          {{end}}
        </div>
      </section>
      <footer>
        <p>Data sources: CoinGecko, Stooq, alternative.me</p>
      </footer>
    </main>
    <script>
      const toggle = document.getElementById('theme-toggle');
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      const stored = localStorage.getItem('theme');
      const initial = stored || (prefersDark ? 'dark' : 'light');
      if (initial === 'dark') {
        document.body.classList.add('dark');
        toggle.textContent = 'Light mode';
      }
      toggle.addEventListener('click', () => {
        const isDark = document.body.classList.toggle('dark');
        toggle.textContent = isDark ? 'Light mode' : 'Dark mode';
        localStorage.setItem('theme', isDark ? 'dark' : 'light');
      });
    </script>
  </body>
</html>`))
