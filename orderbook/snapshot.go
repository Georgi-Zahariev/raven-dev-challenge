package orderbook

import (
	"sort"
	"strconv"
)

// SnapshotMsg is what we get from Binance snapshot API
type SnapshotMsg struct {
	LastUpdateID int64      `json:"lastUpdateId"`
	Bids         [][]string `json:"bids"`
	Asks         [][]string `json:"asks"`
}

// ApplySnapshot loads a fresh snapshot, replacing everything
func (ob *OrderBook) ApplySnapshot(snap SnapshotMsg) error {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	// Create new level maps with the right capacity
	bidCount := len(snap.Bids)
	askCount := len(snap.Asks)

	ob.bids = &levelMap{
		isBid:  true,
		prices: make([]float64, 0, bidCount),
		qtyAt:  make(map[float64]float64, bidCount),
	}
	ob.asks = &levelMap{
		isBid:  false,
		prices: make([]float64, 0, askCount),
		qtyAt:  make(map[float64]float64, askCount),
	}

	// Parse and collect all bid prices
	bidPrices := make([]float64, 0, bidCount)
	for _, lvl := range snap.Bids {
		price, err1 := strconv.ParseFloat(lvl[0], 64)
		qty, err2 := strconv.ParseFloat(lvl[1], 64)
		if err1 != nil || err2 != nil {
			return err1
		}
		if qty > 0 {
			ob.bids.qtyAt[price] = qty
			bidPrices = append(bidPrices, price)
		}
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(bidPrices)))
	ob.bids.prices = bidPrices

	// Same for asks
	askPrices := make([]float64, 0, askCount)
	for _, lvl := range snap.Asks {
		price, err1 := strconv.ParseFloat(lvl[0], 64)
		qty, err2 := strconv.ParseFloat(lvl[1], 64)
		if err1 != nil || err2 != nil {
			return err1
		}
		if qty > 0 {
			ob.asks.qtyAt[price] = qty
			askPrices = append(askPrices, price)
		}
	}
	sort.Float64s(askPrices)
	ob.asks.prices = askPrices

	ob.lastID = snap.LastUpdateID
	return nil
}
