package main

import (
	"sort"
	"sync"
)

type Position struct {
	price            float32
	lastPrice        float32
	volume           int
	lastVolume       int
	lastCandleVolume int
	hasLastCandle    bool
	quantity         float32
}

type Trade struct {
	Symbol    string
	Side      string
	Price     float32
	Quantity  float32
	Principal float32
}

// Positions encapsulates the positions map, its mutex, and the cash balance.
// All map access goes through its methods; callers acquire the lock for bulk operations.
type Positions struct {
	mu   sync.Mutex
	data map[string]Position
	Cash float32
}

func NewPositions(data map[string]Position, cash float32) *Positions {
	return &Positions{data: data, Cash: cash}
}

func (hs *Positions) Lock()   { hs.mu.Lock() }
func (hs *Positions) Unlock() { hs.mu.Unlock() }

func (hs *Positions) Get(symbol string) (Position, bool) {
	h, ok := hs.data[symbol]
	return h, ok
}

func (hs *Positions) Set(symbol string, h Position) {
	hs.data[symbol] = h
}

// Symbols returns a sorted slice of all tracked symbols.
func (hs *Positions) Symbols() []string {
	syms := make([]string, 0, len(hs.data))
	for s := range hs.data {
		syms = append(syms, s)
	}
	sort.Strings(syms)
	return syms
}

// TryBuy increases the quantity for symbol by the number of shares principal can buy.
// Caller must hold the lock.
func (hs *Positions) TryBuy(symbol string, principal float32) (Trade, bool) {
	h := hs.data[symbol]
	if hs.Cash < principal || principal <= 0 || h.price <= 0 {
		return Trade{}, false
	}
	quantity := principal / h.price
	hs.Cash -= principal
	h.quantity += quantity
	hs.data[symbol] = h
	return Trade{
		Symbol:    symbol,
		Side:      "BUY",
		Price:     h.price,
		Quantity:  quantity,
		Principal: principal,
	}, true
}

// TrySell decreases the quantity for symbol by the number of shares principal represents.
// Caller must hold the lock.
func (hs *Positions) TrySell(symbol string, principal float32) (Trade, bool) {
	h := hs.data[symbol]
	if h.quantity <= 0 || principal <= 0 || h.price <= 0 {
		return Trade{}, false
	}
	quantity := principal / h.price
	if quantity > h.quantity {
		quantity = h.quantity
		principal = quantity * h.price
	}
	h.quantity -= quantity
	hs.Cash += principal
	hs.data[symbol] = h
	return Trade{
		Symbol:    symbol,
		Side:      "SELL",
		Price:     h.price,
		Quantity:  quantity,
		Principal: principal,
	}, true
}

// PortfolioValue returns total value of cash plus all held positions at current prices.
func (hs *Positions) PortfolioValue() float32 {
	total := hs.Cash
	for _, h := range hs.data {
		total += float32(h.quantity) * h.price
	}
	return total
}
