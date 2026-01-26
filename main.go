package main

import (
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
	AssetPath   string
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
	MA20         float64
	AboveMA20    bool
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

	assetsPath := filepath.Join(outputDir, "assets")
	if err := os.MkdirAll(assetsPath, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create assets dir: %v\n", err)
		os.Exit(1)
	}

	historyPath := filepath.Join(outputDir, historyDir)
	if err := os.MkdirAll(historyPath, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create history dir: %v\n", err)
		os.Exit(1)
	}

	baseHistoryEntries, err := loadHistoryEntries(historyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load history: %v\n", err)
		os.Exit(1)
	}

	todayFilename := time.Now().Format("20060102") + ".html"
	todayEntry := HistoryEntry{
		Date: time.Now().Format("2006-01-02"),
		File: todayFilename,
	}
	baseHistoryEntries = upsertHistoryEntry(baseHistoryEntries, todayEntry)

	indexHistoryEntries := withHistoryPrefix(baseHistoryEntries, historyDir+"/")
	historyHistoryEntries := withHistoryPrefix(baseHistoryEntries, "")

	outputPath := filepath.Join(outputDir, outputFile)
	file, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create output file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	indexData := PageData{
		GeneratedAt: time.Now().Format("2006-01-02 15:04 MST"),
		Coins:       coins,
		Indices:     indices,
		FearGreed:   fearGreed,
		Stocks:      stocks,
		History:     indexHistoryEntries,
		AssetPath:   "assets",
	}

	historyData := indexData
	historyData.History = historyHistoryEntries
	historyData.AssetPath = "../assets"

	if err := renderPage(file, indexData); err != nil {
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
	if err := renderPage(historyFile, historyData); err != nil {
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
	ma20 := simpleMovingAverage(prices, 20)
	stock.Close = prices[len(prices)-1]
	stock.MACD = macd
	stock.Signal = signal
	stock.Hist = hist
	stock.BullishCross = bullishCross
	stock.MA20 = ma20
	stock.AboveMA20 = stock.Close >= ma20

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

func renderPage(writer io.Writer, data PageData) error {
	return pageTemplate.Execute(writer, data)
}

func withHistoryPrefix(entries []HistoryEntry, prefix string) []HistoryEntry {
	if prefix == "" {
		return entries
	}
	cloned := make([]HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		cloned = append(cloned, HistoryEntry{
			Date: entry.Date,
			File: prefix + entry.File,
		})
	}
	return cloned
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
			File: name,
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
    <link rel="icon" href="{{.AssetPath}}/favicon.ico" />
    <title>Daily Investment Snapshot</title>
    <link rel="stylesheet" href="{{.AssetPath}}/style.css" />
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
            {{if .AboveMA20}}
            <div class="badge bullish">Above MA20</div>
            {{else}}
            <div class="badge bearish">Below MA20</div>
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
    <script src="{{.AssetPath}}/app.js"></script>
  </body>
</html>`))
