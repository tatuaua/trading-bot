package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	finnhub "github.com/Finnhub-Stock-API/finnhub-go/v2"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	_ "github.com/ncruces/go-sqlite3/driver"
)

var candleInterval = 500
var candleIntervalMs = time.Duration(candleInterval) * time.Millisecond
var symbols []string
var positions *Positions
var initialPortfolioValue float32

type wsMessage struct {
	Type string    `json:"type"`
	Data []wsTrade `json:"data"`
}

type wsTrade struct {
	Price  float32 `json:"p"`
	Symbol string  `json:"s"`
	Volume float64 `json:"v"`
}

var db *sql.DB

func main() {
	if err := godotenv.Load(); err != nil {
		panic("failed to load .env: " + err.Error())
	}
	token := os.Getenv("FINNHUB_TOKEN")
	if token == "" {
		panic("FINNHUB_TOKEN is not set")
	}

	var err error

	db, err = sql.Open("sqlite3", "demo.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	createTableSQL := `CREATE TABLE IF NOT EXISTS candles (
		symbol TEXT NOT NULL,
		timestamp BIGINT,
		volume INT NOT NULL,
		delta REAL NOT NULL,
		interval INT NOT NULL
	);`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatal("Error creating table:", err)
	}

	positions = NewPositions(map[string]Position{
		"AAPL":  {quantity: 100},
		"MSFT":  {quantity: 100},
		"GOOG":  {quantity: 100},
		"AMZN":  {quantity: 100},
		"NVDA":  {quantity: 100},
		"META":  {quantity: 100},
		"TSLA":  {quantity: 100},
		"AVGO":  {quantity: 100},
		"BRK.B": {quantity: 100},
		"JPM":   {quantity: 100},
		"SPCX":  {quantity: 100},
	}, 100000)
	symbols = positions.Symbols()
	fetchInitialPrices(token)
	initialPortfolioValue = portfolioValue()
	initScreen()

	w, _, err := websocket.DefaultDialer.Dial("wss://ws.finnhub.io?token="+token, nil)
	if err != nil {
		panic(err)
	}
	defer w.Close()

	for _, symbol := range symbols {
		msg, _ := json.Marshal(map[string]any{"type": "subscribe", "symbol": symbol})
		w.WriteMessage(websocket.TextMessage, msg)
	}

	go func() {
		for {
			time.Sleep(candleIntervalMs)
			candle()
		}
	}()

	var msg wsMessage
	for {
		err := w.ReadJSON(&msg)
		if err != nil {
			panic(err)
		}

		if msg.Type != "trade" {
			continue
		}

		positions.Lock()
		for _, trade := range msg.Data {
			currPosition, ok := positions.Get(trade.Symbol)
			if !ok {
				continue
			}
			currPosition.price = trade.Price
			currPosition.volume += int(trade.Volume)
			positions.Set(trade.Symbol, currPosition)
		}
		positions.Unlock()
	}
}

func fetchInitialPrices(token string) {
	cfg := finnhub.NewConfiguration()
	cfg.AddDefaultHeader("X-Finnhub-Token", token)
	client := finnhub.NewAPIClient(cfg).DefaultApi

	for _, symbol := range symbols {
		res, _, err := client.Quote(context.Background()).Symbol(symbol).Execute()
		if err != nil || res.C == nil {
			continue
		}
		h, _ := positions.Get(symbol)
		h.price = *res.C
		h.lastPrice = *res.C
		positions.Set(symbol, h)
	}
}

func initScreen() {
	// Clear screen and place cursor at top-left once.
	fmt.Print("\033[2J\033[H")
	fmt.Printf("Live %v ms Candles\n", candleIntervalMs.Milliseconds())
	fmt.Println("Symbol   Price      Delta      VolDelta    Qty")
	for range symbols {
		fmt.Println()
	}
	fmt.Println() // cash line
	fmt.Println() // portfolio line
}

func colorize(text, color string) string {
	return color + text + "\033[0m"
}

func portfolioValue() float32 {
	return positions.PortfolioValue()
}

func renderDashboard() {
	// Move cursor to row 3, col 1 (first symbol line) and overwrite rows.
	fmt.Print("\033[3;1H")

	for _, symbol := range symbols {
		h, _ := positions.Get(symbol)
		delta := h.price - h.lastPrice
		currentCandleVolume := h.volume - h.lastVolume

		priceText := fmt.Sprintf("%8.2f", h.price)
		deltaText := fmt.Sprintf("%+8.2f", delta)

		deltaColor := "\033[33m"
		if delta > 0 {
			deltaColor = "\033[32m"
		} else if delta < 0 {
			deltaColor = "\033[31m"
		}

		var volDeltaText string
		volDelta := currentCandleVolume - h.lastCandleVolume
		volColor := "\033[36m"
		if !h.hasLastCandle {
			volDeltaText = fmt.Sprintf("%+8d", currentCandleVolume)
		} else {
			volDeltaText = fmt.Sprintf("%+8d", volDelta)
			if volDelta > 0 {
				volColor = "\033[32m"
			} else if volDelta < 0 {
				volColor = "\033[31m"
			} else {
				volColor = "\033[33m"
			}
		}

		qtyText := fmt.Sprintf("%-12.2f", h.quantity)
		line := fmt.Sprintf(
			"%-7s  %s  %s  %s  %s",
			symbol,
			colorize(priceText, "\033[37m"),
			colorize(deltaText, deltaColor),
			colorize(volDeltaText, volColor),
			colorize(qtyText, "\033[37m"),
		)

		fmt.Printf("\033[2K%s\n", line)
	}

	current := portfolioValue()
	profit := current - initialPortfolioValue
	fmt.Printf("\033[2KCash:      $%12.2f\n", positions.Cash)
	fmt.Printf("\033[2KPortfolio: $%12.2f  Profit: %+.2f\n", current, profit)
}

func candle() {
	positions.Lock()
	defer positions.Unlock()

	renderDashboard()

	for _, symbol := range symbols {
		h, _ := positions.Get(symbol)

		currentCandleVolume := h.volume - h.lastVolume
		delta := h.price - h.lastPrice

		avgVol, errV := getAverageCandleVolume(symbol)
		avgDelta, errD := getAverageCandleDelta(symbol)
		if errV == nil && errD == nil {
			if delta > avgDelta && float32(currentCandleVolume) > avgVol {
				positions.TryBuy(symbol, positions.Cash*0.01)
				h, _ = positions.Get(symbol)
			} else if delta < avgDelta && float32(currentCandleVolume) > avgVol {
				positions.TrySell(symbol, positions.Cash*0.01)
				h, _ = positions.Get(symbol)
			}
		}

		insertCandle(symbol, time.Now().Unix(), currentCandleVolume, delta, candleInterval)

		h.lastPrice = h.price
		h.lastCandleVolume = currentCandleVolume
		h.hasLastCandle = true
		h.lastVolume = h.volume
		positions.Set(symbol, h)
	}
}

func insertCandle(symbol string, timestamp int64, volume int, price float32, interval int) {
	insertSQL := `INSERT INTO candles (symbol, timestamp, volume, delta, interval) VALUES (?,?,?,?,?);`
	_, err := db.Exec(insertSQL, symbol, timestamp, volume, price, interval)
	if err != nil {
		log.Fatal("Error inserting data:", err)
	}
}

func getAverageCandleVolume(symbol string) (float32, error) {
	rows, err := db.Query("SELECT volume FROM candles WHERE symbol = ?", symbol)
	if err != nil {
		log.Fatal("Error querying volume:", err)
	}
	defer rows.Close()

	var sum int
	var amount int

	for rows.Next() {
		var volume int
		err := rows.Scan(&volume)
		if err != nil {
			log.Fatal(err)
		}

		sum += volume
		amount++
	}

	if amount == 0 {
		return 0, errors.New("No volume entries to calculate average from")
	}

	if err = rows.Err(); err != nil {
		log.Fatal(err)
	}

	return float32(sum) / float32(amount), nil
}

func getAverageCandleDelta(symbol string) (float32, error) {
	rows, err := db.Query("SELECT delta FROM candles WHERE symbol = ?", symbol)
	if err != nil {
		log.Fatal("Error querying delta:", err)
	}
	defer rows.Close()

	var sum float32
	var amount int

	for rows.Next() {
		var delta float32
		err := rows.Scan(&delta)
		if err != nil {
			log.Fatal(err)
		}

		sum += delta
		amount++
	}

	if amount == 0 {
		return 0, errors.New("No delta entries to calculate average from")
	}

	if err = rows.Err(); err != nil {
		log.Fatal(err)
	}

	return sum / float32(amount), nil
}
