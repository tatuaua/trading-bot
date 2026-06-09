package main

type Holding struct {
	price            float32
	lastPrice        float32
	volume           int
	lastVolume       int
	lastCandleVolume int
	hasLastCandle    bool
	quantity         int
}
