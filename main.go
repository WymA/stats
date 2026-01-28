package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	outputDir  = "public"
	outputFile = "index.html"
	cryptoURL  = "https://api.coingecko.com/api/v3/coins/markets?vs_currency=usd&ids=bitcoin,ethereum"
	historyDir = "history"
	googleTag  = "<!-- Google tag (gtag.js) -->\n\t<script async src=\"https://www.googletagmanager.com/gtag/js?id=G-JNZQWSY5VP\"></script>\n\t<script>\n\twindow.dataLayer = window.dataLayer || [];\n\tfunction gtag(){dataLayer.push(arguments);}\n\tgtag('js', new Date());\n\n\tgtag('config', 'G-JNZQWSY5VP');\n\t</script>"
)

type Coin struct {
	ID                       string  `json:"id"`
	Symbol                   string  `json:"symbol"`
	Name                     string  `json:"name"`
	CurrentPrice             float64 `json:"current_price"`
	PriceChangePercentage24h float64 `json:"price_change_percentage_24h"`
	LastUpdated              string  `json:"last_updated"`
	EMA20                    float64
	EMA50                    float64
	EMABullish               bool
	ChangePct                float64
}

type PageData struct {
	GeneratedAt    string
	Coins          []Coin
	Indices        []IndexSnapshot
	FearGreed      FearGreedSnapshot
	Stocks         []StockSnapshot
	Signals        []StockSignal
	SignalsScanned int
	History        []HistoryEntry
	AssetPath      string
	GoogleTag      template.HTML
	Year           int
}

type IndexSnapshot struct {
	Name       string
	Symbol     string
	Close      float64
	EMA20      float64
	EMA50      float64
	EMABullish bool
	ChangePct  float64
}

type FearGreedSnapshot struct {
	Value     string
	Category  string
	UpdatedAt string
}

type StockSnapshot struct {
	Name       string
	Symbol     string
	Close      float64
	MACD       float64
	Signal     float64
	EMA20      float64
	EMA50      float64
	EMABullish bool
	ChangePct  float64
}

type StockSignal struct {
	Ticker      string
	Signal      string
	IsBuy       bool
	Close       float64
	EMA20       float64
	EMA50       float64
	EMABullish  bool
	MACD        float64
	SignalLine  float64
	MACDBullish bool
	ChangePct   float64
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
		{Name: "S&P 500", Symbol: "^SPX", Close: 0, EMA20: 0, EMA50: 0},
		{Name: "Nasdaq 100", Symbol: "^NDX", Close: 0, EMA20: 0, EMA50: 0},
	}

	stocks := []StockSnapshot{
		{Name: "Tesla", Symbol: "tsla.us"},
		{Name: "Alphabet", Symbol: "goog.us"},
		{Name: "NVIDIA", Symbol: "nvda.us"},
		{Name: "AMD", Symbol: "amd.us"},
		{Name: "Microsoft", Symbol: "msft.us"},
		{Name: "Amazon", Symbol: "amzn.us"},
		{Name: "Occidental Petroleum", Symbol: "oxy.us"},
		{Name: "Coca-Cola", Symbol: "ko.us"},
		{Name: "PDD Holdings", Symbol: "pdd.us"},
		{Name: "Gold/USD", Symbol: "xauusd"},
		{Name: "JPY/USD", Symbol: "jpyusd"},
		{Name: "RMB/USD", Symbol: "cnhusd"},
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

	tickers, err := loadSP500Tickers(http.Client{Timeout: 15 * time.Second})
	if err != nil {
		fmt.Fprintf(os.Stderr, "load S&P 500 tickers: %v\n", err)
		os.Exit(1)
	}
	stockSignals, err := scanTrendRiderSignals(tickers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan signals: %v\n", err)
		os.Exit(1)
	}
	if len(stockSignals) > 0 {
		enriched, err := fetchAdditionalSnapshots(stocks, stockSignals)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch signal stocks: %v\n", err)
			os.Exit(1)
		}
		stocks = append(stocks, enriched...)
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
		GeneratedAt:    time.Now().Format("2006-01-02 15:04 MST"),
		Coins:          coins,
		Indices:        indices,
		FearGreed:      fearGreed,
		Stocks:         stocks,
		Signals:        stockSignals,
		SignalsScanned: len(tickers),
		History:        indexHistoryEntries,
		AssetPath:      "assets",
		GoogleTag:      template.HTML(googleTag),
		Year:           time.Now().Year(),
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

	for i := range coins {
		history, err := fetchYahooHistory(client, strings.ToUpper(coins[i].Symbol+"-USD"))
		if err != nil || len(history) < 50 {
			continue
		}
		ema20 := exponentialMovingAverage(history, 20)
		ema50 := exponentialMovingAverage(history, 50)
		coins[i].EMA20 = ema20
		coins[i].EMA50 = ema50
		coins[i].EMABullish = ema20 > ema50
		coins[i].ChangePct = dailyChangePct(history)
	}

	return coins, nil
}

func fetchIndexSnapshot(index IndexSnapshot) (IndexSnapshot, error) {
	prices, err := fetchStooqHistory(index.Symbol)
	if err != nil {
		return IndexSnapshot{}, err
	}
	if len(prices) < 50 {
		return IndexSnapshot{}, fmt.Errorf("insufficient data for %s", index.Symbol)
	}

	close := prices[len(prices)-1]
	ema20 := exponentialMovingAverage(prices, 20)
	ema50 := exponentialMovingAverage(prices, 50)

	index.Close = close
	index.EMA20 = ema20
	index.EMA50 = ema50
	index.EMABullish = ema20 > ema50
	index.ChangePct = dailyChangePct(prices)

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

	macd, signalLine, _, _ := macdIndicator(prices, 12, 26, 9)
	ema20 := exponentialMovingAverage(prices, 20)
	ema50 := exponentialMovingAverage(prices, 50)
	stock.Close = prices[len(prices)-1]
	stock.MACD = macd
	stock.Signal = signalLine
	stock.EMA20 = ema20
	stock.EMA50 = ema50
	stock.EMABullish = ema20 > ema50
	stock.ChangePct = dailyChangePct(prices)

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

func scanTrendRiderSignals(tickers []string) ([]StockSignal, error) {
	client := http.Client{Timeout: 15 * time.Second}
	workerLimit := 4
	if len(tickers) < workerLimit {
		workerLimit = len(tickers)
	}
	semaphore := make(chan struct{}, workerLimit)
	results := make(chan StockSignal, len(tickers))
	errors := make(chan error, len(tickers))
	var waitGroup sync.WaitGroup

	for _, ticker := range tickers {
		trimmed := strings.TrimSpace(ticker)
		if trimmed == "" {
			continue
		}
		waitGroup.Add(1)
		go func(symbol string) {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			signal, ok, err := fetchTrendRiderSignal(client, symbol)
			if err != nil {
				errors <- fmt.Errorf("%s: %w", symbol, err)
				return
			}
			if ok {
				results <- signal
			}
		}(strings.ToUpper(trimmed))
	}

	waitGroup.Wait()
	close(results)
	close(errors)

	if len(errors) > 0 {
		return nil, <-errors
	}

	var signals []StockSignal
	for signal := range results {
		signals = append(signals, signal)
	}

	sort.Slice(signals, func(i, j int) bool {
		if signals[i].IsBuy == signals[j].IsBuy {
			return signals[i].Ticker < signals[j].Ticker
		}
		return signals[i].IsBuy
	})

	for _, signal := range signals {
		fmt.Printf("%s | Signal: %s | Close: $%.2f | EMA20: $%.2f | EMA50: $%.2f\n", signal.Ticker, signal.Signal, signal.Close, signal.EMA20, signal.EMA50)
	}

	return signals, nil
}

func fetchTrendRiderSignal(client http.Client, ticker string) (StockSignal, bool, error) {
	prices, err := fetchYahooHistory(client, ticker)
	if err != nil {
		return StockSignal{}, false, err
	}
	if len(prices) < 51 {
		return StockSignal{}, false, fmt.Errorf("insufficient data")
	}

	ema20 := exponentialMovingAverage(prices, 20)
	ema50 := exponentialMovingAverage(prices, 50)
	close := prices[len(prices)-1]
	prevEMA20 := exponentialMovingAverage(prices[:len(prices)-1], 20)
	prevEMA50 := exponentialMovingAverage(prices[:len(prices)-1], 50)

	buy := prevEMA20 <= prevEMA50 && ema20 > ema50 && close > ema50
	sell := close < ema50
	if !buy && !sell {
		return StockSignal{}, false, nil
	}

	macd, signalLine, _, _ := macdIndicator(prices, 12, 26, 9)
	macdBullish := macd > signalLine

	signal := "SELL"
	if buy {
		signal = "BUY"
	}

	return StockSignal{
		Ticker:      ticker,
		Signal:      signal,
		IsBuy:       buy,
		Close:       close,
		EMA20:       ema20,
		EMA50:       ema50,
		EMABullish:  ema20 > ema50,
		MACD:        macd,
		SignalLine:  signalLine,
		MACDBullish: macdBullish,
		ChangePct:   dailyChangePct(prices),
	}, true, nil
}

func fetchYahooHistory(client http.Client, ticker string) ([]float64, error) {
	end := time.Now().UTC()
	start := end.AddDate(0, -6, -7)
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&period1=%d&period2=%d", ticker, start.Unix(), end.Unix())
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "stats-dashboard")

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var payload struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Close []*float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error any `json:"error"`
		} `json:"chart"`
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Chart.Error != nil {
		return nil, fmt.Errorf("chart error")
	}
	if len(payload.Chart.Result) == 0 {
		return nil, fmt.Errorf("no chart data")
	}

	result := payload.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 {
		return nil, fmt.Errorf("missing quote data")
	}
	closes := result.Indicators.Quote[0].Close
	if len(closes) == 0 || len(result.Timestamp) == 0 {
		return nil, fmt.Errorf("missing close data")
	}

	prices := make([]float64, 0, len(closes))
	for i, close := range closes {
		if i >= len(result.Timestamp) {
			break
		}
		if close == nil || math.IsNaN(*close) {
			continue
		}
		prices = append(prices, *close)
	}

	if len(prices) < 50 {
		return nil, fmt.Errorf("insufficient data")
	}

	return prices, nil
}

func loadSP500Tickers(client http.Client) ([]string, error) {
	request, err := http.NewRequest("GET", "https://en.wikipedia.org/wiki/List_of_S%26P_500_companies", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "stats-dashboard")

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	content := string(body)
	tableStart := strings.Index(content, "id=\"constituents\"")
	if tableStart == -1 {
		return nil, fmt.Errorf("constituents table not found")
	}
	content = content[tableStart:]
	rows := strings.Split(content, "<tr")
	var tickers []string
	seen := make(map[string]struct{})
	for _, row := range rows {
		if !strings.Contains(row, "<td") {
			continue
		}
		cells := strings.Split(row, "<td>")
		if len(cells) < 2 {
			continue
		}
		cell := cells[1]
		cellEnd := strings.Index(cell, "</td>")
		if cellEnd == -1 {
			continue
		}
		symbolRaw := strings.TrimSpace(stripHTML(cell[:cellEnd]))
		if symbolRaw == "" || strings.Contains(symbolRaw, "Symbol") {
			continue
		}
		symbol := strings.ReplaceAll(symbolRaw, ".", "-")
		symbol = strings.ToUpper(symbol)
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		tickers = append(tickers, symbol)
	}

	if len(tickers) == 0 {
		return nil, fmt.Errorf("no tickers parsed")
	}

	return tickers, nil
}

func stripHTML(value string) string {
	var builder strings.Builder
	inTag := false
	for _, r := range value {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				builder.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func fetchAdditionalSnapshots(existing []StockSnapshot, signals []StockSignal) ([]StockSnapshot, error) {
	if len(signals) == 0 {
		return nil, nil
	}

	existingSymbols := make(map[string]struct{}, len(existing))
	for _, stock := range existing {
		existingSymbols[strings.ToLower(stock.Symbol)] = struct{}{}
	}

	var additional []StockSnapshot
	for _, signal := range signals {
		symbol := strings.ToLower(signal.Ticker) + ".us"
		if _, exists := existingSymbols[symbol]; exists {
			continue
		}
		snapshot, err := fetchStockSnapshot(StockSnapshot{Name: signal.Ticker, Symbol: symbol})
		if err != nil {
			return nil, err
		}
		additional = append(additional, snapshot)
	}

	return additional, nil
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

func dailyChangePct(prices []float64) float64 {
	if len(prices) < 2 {
		return 0
	}
	prev := prices[len(prices)-2]
	if prev == 0 {
		return 0
	}
	latest := prices[len(prices)-1]
	return (latest - prev) / prev * 100
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
	apiKey := strings.TrimSpace(os.Getenv("COINMARKETCAP_API_KEY"))
	if apiKey == "" {
		return FearGreedSnapshot{}, fmt.Errorf("missing COINMARKETCAP_API_KEY")
	}

	client := http.Client{Timeout: 15 * time.Second}
	request, err := http.NewRequest("GET", "https://pro-api.coinmarketcap.com/v3/fear-and-greed/historical?limit=1", nil)
	if err != nil {
		return FearGreedSnapshot{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-CMC_PRO_API_KEY", apiKey)

	resp, err := client.Do(request)
	if err != nil {
		return FearGreedSnapshot{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FearGreedSnapshot{}, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var payload struct {
		Data []struct {
			Value               json.Number `json:"value"`
			ValueClassification string      `json:"value_classification"`
			Timestamp           string      `json:"timestamp"`
		} `json:"data"`
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return FearGreedSnapshot{}, err
	}

	if len(payload.Data) == 0 {
		return FearGreedSnapshot{}, fmt.Errorf("no fear & greed data")
	}

	entry := payload.Data[0]
	updatedAt := ""
	if entry.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
			updatedAt = parsed.UTC().Format("2006-01-02")
		} else if parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
			updatedAt = parsed.UTC().Format("2006-01-02")
		}
	}

	return FearGreedSnapshot{
		Value:     entry.Value.String(),
		Category:  entry.ValueClassification,
		UpdatedAt: updatedAt,
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
    <meta name="description" content="Daily market dashboard from Wym with S&P 500, Nasdaq 100, Bitcoin, Ethereum, and 20/50 Trend Rider signals for US stocks." />
    <meta name="keywords" content="Wym, daily investment snapshot, market dashboard, S&P 500, Nasdaq 100, Bitcoin, Ethereum, trend rider, SMA20, SMA50, buy signal, sell signal" />
    <link rel="icon" href="{{.AssetPath}}/favicon.ico" />
    <title>Wym Stats</title>
    {{.GoogleTag}}
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css" />
    <link rel="stylesheet" href="{{.AssetPath}}/style.css" />
  </head>
  <body>
    <header>
      <div class="toolbar">
        <a class="icon-button" href="https://stats.matthias2wym.com/" aria-label="Home">
          <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
            <path d="M3 10.5 12 3l9 7.5v9a1.5 1.5 0 0 1-1.5 1.5H15v-6h-6v6H4.5A1.5 1.5 0 0 1 3 19.5v-9z" fill="currentColor" />
          </svg>
          <span class="sr-only">Home</span>
        </a>
        <div class="toolbar-actions">
          <details class="history-menu">
            <summary class="icon-button" aria-label="History">
              <i class="fa-solid fa-clock-rotate-left" aria-hidden="true"></i>
              <span class="sr-only">History</span>
            </summary>
            <div class="history-list">
              {{range .History}}
              <a href="{{.File}}">{{.Date}}</a>
              {{end}}
            </div>
          </details>
          <button id="theme-toggle" class="icon-button" type="button" aria-label="Toggle theme">
            <i class="fa-regular fa-moon" aria-hidden="true"></i>
            <span class="sr-only">Theme</span>
          </button>
        </div>
      </div>
      <h1>Wym Stats</h1>
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
            <div class="change">Last {{printf "%.2f" .ChangePct}}%</div>
            <div class="metric">EMA20: ${{printf "%.2f" .EMA20}}</div>
            <div class="metric">EMA50: ${{printf "%.2f" .EMA50}}</div>
            <div class="badge {{if .EMABullish}}bullish{{else}}bearish{{end}}">EMA20 {{if .EMABullish}}Above{{else}}Below{{end}} EMA50</div>
          </article>
          {{end}}
        </div>
      </section>
      <section>
        <div class="section-title">Predefined</div>
        <div class="grid">
          {{range .Stocks}}
          <article class="card">
            <div class="symbol">{{.Symbol}}</div>
            <h2>{{.Name}}</h2>
            <div class="price">${{formatPrice .Close}}</div>
            <div class="change">Last {{printf "%.2f" .ChangePct}}%</div>
            <div class="badge {{if .EMABullish}}bullish{{else}}bearish{{end}}">EMA20 {{if .EMABullish}}Above{{else}}Below{{end}} EMA50</div>
            <div class="stock-metrics">
              <span>EMA20 ${{printf "%.2f" .EMA20}}</span>
              <span>EMA50 ${{printf "%.2f" .EMA50}}</span>
              <span>MACD {{printf "%.2f" .MACD}}</span>
              <span>Signal {{printf "%.2f" .Signal}}</span>
            </div>
          </article>
          {{end}}
        </div>
      </section>
      <section>
        <div class="section-title">Auto Screener</div>
        <div class="metric">Scanned {{.SignalsScanned}} tickers • Last scan {{.GeneratedAt}}</div>
        <div class="grid">
          {{if .Signals}}
          {{range .Signals}}
          <article class="card {{if .IsBuy}}bullish{{else}}bearish{{end}}">
            <div class="symbol">{{.Ticker}}</div>
            <h2>{{.Signal}}</h2>
            <div class="price">${{printf "%.2f" .Close}}</div>
            <div class="change">Last {{printf "%.2f" .ChangePct}}%</div>
            <div class="badge {{if .EMABullish}}bullish{{else}}bearish{{end}}">EMA20 {{if .EMABullish}}Above{{else}}Below{{end}} EMA50</div>
            <div class="badge {{if .MACDBullish}}bullish{{else}}bearish{{end}}">MACD {{if .MACDBullish}}Bullish{{else}}Bearish{{end}}</div>
            <div class="stock-metrics">
              <span>EMA20 ${{printf "%.2f" .EMA20}}</span>
              <span>EMA50 ${{printf "%.2f" .EMA50}}</span>
              <span>MACD {{printf "%.2f" .MACD}}</span>
              <span>Signal {{printf "%.2f" .SignalLine}}</span>
            </div>
          </article>
          {{end}}
          {{else}}
          <article class="card">
            <div class="symbol">No signals today</div>
            <h2>Scan complete</h2>
            <div class="metric">No buy or sell triggers met the criteria.</div>
          </article>
          {{end}}
        </div>
      </section>
      <section>
        <div class="section-title">Fear &amp; Greed Index</div>
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
            <div class="change">Last {{printf "%.2f" .ChangePct}}%</div>
            {{if .EMA20}}
            <div class="metric">EMA20: ${{printf "%.2f" .EMA20}}</div>
            <div class="metric">EMA50: ${{printf "%.2f" .EMA50}}</div>
            <div class="badge {{if .EMABullish}}bullish{{else}}bearish{{end}}">EMA20 {{if .EMABullish}}Above{{else}}Below{{end}} EMA50</div>
            {{end}}
          </article>
          {{end}}
        </div>
      </section>
      <footer>
        <p>Data sources: CoinGecko, Stooq, CoinMarketCap, Yahoo Finance</p>
        <p>All rights reserved {{.Year}}</p>
      </footer>
    </main>
    <script src="{{.AssetPath}}/app.js"></script>
  </body>
</html>`))
