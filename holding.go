package main

import (
	"sort"
	"sync"
)

type Holding struct {
	price            float32
	lastPrice        float32
	volume           int
	lastVolume       int
	lastCandleVolume int
	hasLastCandle    bool
	quantity         float32
}

// Holdings encapsulates the holdings map, its mutex, and the cash balance.
// All map access goes through its methods; callers acquire the lock for bulk operations.
type Holdings struct {
	mu   sync.Mutex
	data map[string]Holding
	Cash float32
}

func NewHoldings(data map[string]Holding, cash float32) *Holdings {
	return &Holdings{data: data, Cash: cash}
}

func (hs *Holdings) Lock()   { hs.mu.Lock() }
func (hs *Holdings) Unlock() { hs.mu.Unlock() }

func (hs *Holdings) Get(symbol string) (Holding, bool) {
	h, ok := hs.data[symbol]
	return h, ok
}

func (hs *Holdings) Set(symbol string, h Holding) {
	hs.data[symbol] = h
}

// Symbols returns a sorted slice of all tracked symbols.
func (hs *Holdings) Symbols() []string {
	syms := make([]string, 0, len(hs.data))
	for s := range hs.data {
		syms = append(syms, s)
	}
	sort.Strings(syms)
	return syms
}

// Buy increases the quantity for symbol by amount and deducts cost + commission from Cash.
// Caller must hold the lock.
func (hs *Holdings) Buy(symbol string, principal float32) {
	h := hs.data[symbol]
	hs.Cash -= principal
	h.quantity += h.price / principal
	hs.data[symbol] = h
}

// Sell decreases the quantity for symbol by amount and adds proceeds minus commission to Cash.
// Caller must hold the lock.
func (hs *Holdings) Sell(symbol string, principal float32) {
	h := hs.data[symbol]
	hs.Cash += principal
	h.quantity -= h.price / principal
	hs.data[symbol] = h
}

// PortfolioValue returns total value of cash plus all held positions at current prices.
func (hs *Holdings) PortfolioValue() float32 {
	total := hs.Cash
	for _, h := range hs.data {
		total += float32(h.quantity) * h.price
	}
	return total
}
