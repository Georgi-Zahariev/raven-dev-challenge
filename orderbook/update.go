package orderbook

import "strconv"

// UpdateMsg is what we get from the Binance WebSocket stream
type UpdateMsg struct {
	FirstID int64      `json:"U"`
	FinalID int64      `json:"u"`
	Bids    [][]string `json:"b"`
	Asks    [][]string `json:"a"`
}

// ApplyUpdate applies an incremental update to the order book
func (ob *OrderBook) ApplyUpdate(upd UpdateMsg) error {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	// Skip old/duplicate updates
	if upd.FinalID < ob.lastID+1 {
		return nil
	}

	// Check for gaps in sequence
	if ob.lastID+1 < upd.FirstID || upd.FinalID < ob.lastID+1 {
		return ErrGap
	}

	// Helper to parse price levels
	parseLevels := func(levels [][]string, m *levelMap) error {
		for _, lvl := range levels {
			price, err1 := strconv.ParseFloat(lvl[0], 64)
			qty, err2 := strconv.ParseFloat(lvl[1], 64)
			if err1 != nil || err2 != nil {
				return err1
			}
			m.set(price, qty)
		}
		return nil
	}

	if err := parseLevels(upd.Bids, ob.bids); err != nil {
		return err
	}
	if err := parseLevels(upd.Asks, ob.asks); err != nil {
		return err
	}

	ob.lastID = upd.FinalID
	return nil
}
