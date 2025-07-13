package orderbook

import (
	"errors"
	"sort"
	"sync"
)

// levelMap stores price levels as both a map (for O(1) lookups) and a sorted slice
// (for easy iteration). Bids are sorted high-to-low, asks low-to-high.
type levelMap struct {
	isBid  bool
	prices []float64
	qtyAt  map[float64]float64
}

func newLevelMap(isBid bool) *levelMap {
	return &levelMap{
		isBid: isBid,
		qtyAt: make(map[float64]float64),
	}
}

// set updates the quantity at a given price. Zero quantity removes the level.
func (lm *levelMap) set(price, qty float64) {
	_, exists := lm.qtyAt[price]

	if qty == 0 {
		if exists {
			delete(lm.qtyAt, price)
			lm.removePrice(price)
		}
		return
	}

	lm.qtyAt[price] = qty
	if !exists {
		lm.insertPrice(price)
	}
}

// insertPrice adds a new price to the sorted slice using binary search
func (lm *levelMap) insertPrice(price float64) {
	n := len(lm.prices)

	// Find where to insert this price
	var insertPos int
	if lm.isBid {
		// bids: highest prices first
		insertPos = sort.Search(n, func(i int) bool {
			return lm.prices[i] <= price
		})
	} else {
		// asks: lowest prices first
		insertPos = sort.Search(n, func(i int) bool {
			return lm.prices[i] >= price
		})
	}

	// Make room and insert
	lm.prices = append(lm.prices, 0)
	copy(lm.prices[insertPos+1:], lm.prices[insertPos:])
	lm.prices[insertPos] = price
}

// removePrice removes a price from the sorted slice
func (lm *levelMap) removePrice(price float64) {
	n := len(lm.prices)

	// Find the price using binary search
	var pos int
	if lm.isBid {
		pos = sort.Search(n, func(i int) bool {
			return lm.prices[i] <= price
		})
	} else {
		pos = sort.Search(n, func(i int) bool {
			return lm.prices[i] >= price
		})
	}

	// Remove it if we found it
	if pos < n && lm.prices[pos] == price {
		copy(lm.prices[pos:], lm.prices[pos+1:])
		lm.prices = lm.prices[:n-1]
	}
}

// best returns the top price and quantity, or false if empty
func (lm *levelMap) best() (price, qty float64, ok bool) {
	if len(lm.prices) == 0 {
		return 0, 0, false
	}
	p := lm.prices[0]
	return p, lm.qtyAt[p], true
}

type OrderBook struct {
	mu     sync.RWMutex
	lastID int64
	bids   *levelMap
	asks   *levelMap
}

func New() *OrderBook {
	return &OrderBook{
		bids: newLevelMap(true),
		asks: newLevelMap(false),
	}
}

// Quick access to best prices - these get called a lot
func (ob *OrderBook) BestBid() (price, qty float64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.bids.best()
}

func (ob *OrderBook) BestAsk() (price, qty float64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.asks.best()
}

var ErrGap = errors.New("sequence gap detected")

func (ob *OrderBook) GetLastID() int64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.lastID
}

type Level struct {
	Price    float64
	Quantity float64
}

// GetBids returns top bid levels in price order (highest first)
func (ob *OrderBook) GetBids(maxLevels int) []Level {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	count := min(len(ob.bids.prices), maxLevels)
	levels := make([]Level, count)
	for i := 0; i < count; i++ {
		price := ob.bids.prices[i]
		levels[i] = Level{
			Price:    price,
			Quantity: ob.bids.qtyAt[price],
		}
	}
	return levels
}

// GetAsks returns top ask levels in price order (lowest first)
func (ob *OrderBook) GetAsks(maxLevels int) []Level {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	count := min(len(ob.asks.prices), maxLevels)
	levels := make([]Level, count)
	for i := 0; i < count; i++ {
		price := ob.asks.prices[i]
		levels[i] = Level{
			Price:    price,
			Quantity: ob.asks.qtyAt[price],
		}
	}
	return levels
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
