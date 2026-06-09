package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	finnhub "github.com/Finnhub-Stock-API/finnhub-go/v2"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

var candleInterval = 500 * time.Millisecond
var holdingsMu sync.Mutex
var symbols []string
var cash float32 = 100000

type wsMessage struct {
	Type string    `json:"type"`
	Data []wsTrade `json:"data"`
}

type wsTrade struct {
	Price  float32 `json:"p"`
	Symbol string  `json:"s"`
	Volume float64 `json:"v"`
}

// holdings maps each symbol to its holding data.
var holdings = map[string]Holding{
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
}

func main() {
	if err := godotenv.Load(); err != nil {
		panic("failed to load .env: " + err.Error())
	}
	token := os.Getenv("FINNHUB_TOKEN")
	if token == "" {
		panic("FINNHUB_TOKEN is not set")
	}

	initSymbols()
	fetchInitialPrices(token)
	initScreen()

	w, _, err := websocket.DefaultDialer.Dial("wss://ws.finnhub.io?token="+token, nil)
	if err != nil {
		panic(err)
	}
	defer w.Close()

	for symbol := range holdings {
		msg, _ := json.Marshal(map[string]any{"type": "subscribe", "symbol": symbol})
		w.WriteMessage(websocket.TextMessage, msg)
	}

	go func() {
		for {
			time.Sleep(candleInterval)
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

		holdingsMu.Lock()
		for _, trade := range msg.Data {
			currHolding, ok := holdings[trade.Symbol]
			if !ok {
				continue
			}

			currHolding.price = trade.Price
			currHolding.volume += int(trade.Volume)
			holdings[trade.Symbol] = currHolding
		}
		holdingsMu.Unlock()
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
		h := holdings[symbol]
		h.price = *res.C
		h.lastPrice = *res.C
		holdings[symbol] = h
	}
}

func initSymbols() {
	symbols = make([]string, 0, len(holdings))
	for symbol := range holdings {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
}

func initScreen() {
	// Clear screen and place cursor at top-left once.
	fmt.Print("\033[2J\033[H")
	fmt.Printf("Live %v ms Candles\n", candleInterval.Milliseconds())
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
	var total float32 = cash
	for _, h := range holdings {
		total += float32(h.quantity) * h.price
	}
	return total
}

func renderDashboard() {
	// Move cursor to row 3, col 1 (first symbol line) and overwrite rows.
	fmt.Print("\033[3;1H")

	for _, symbol := range symbols {
		h := holdings[symbol]
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

		qtyText := fmt.Sprintf("%-8d", h.quantity)
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

	fmt.Printf("\033[2KCash:      $%12.2f\n", cash)
	fmt.Printf("\033[2KPortfolio: $%12.2f\n", portfolioValue())
}

func candle() {
	holdingsMu.Lock()
	defer holdingsMu.Unlock()

	renderDashboard()

	for _, symbol := range symbols {
		h := holdings[symbol]

		currentCandleVolume := h.volume - h.lastVolume
		delta := h.price - h.lastPrice

		if h.hasLastCandle {
			if delta > 0 && currentCandleVolume > h.lastCandleVolume {
				buy(symbol, 1)
				h = holdings[symbol]
			} else if delta < 0 && currentCandleVolume < h.lastCandleVolume {
				sell(symbol, 1)
				h = holdings[symbol]
			}
		}

		h.lastPrice = h.price
		h.lastCandleVolume = currentCandleVolume
		h.hasLastCandle = true
		h.lastVolume = h.volume
		holdings[symbol] = h
	}
}

func buy(symbol string, amount int) {
	h := holdings[symbol]
	buyPrice := float32(amount) * h.price
	h.quantity += amount
	holdings[symbol] = h
	cash -= buyPrice + 1 // +1 commission
}

func sell(symbol string, amount int) {
	h := holdings[symbol]
	sellPrice := float32(amount) * h.price
	h.quantity -= amount
	holdings[symbol] = h
	cash += sellPrice - 1 // -1 commission
}
