// benchmarks/benchmark_test.go
package benchmarks

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"raven-dev-challenge/orderbook"
)

// ---------- helpers ----------------------------------------------------

// seedBook loads a synthetic snapshot with `levels` bids & asks
// evenly spaced around a mid-price.
func seedBook(ob *orderbook.OrderBook, levels int) {
	const mid = 10_000.0
	snap := orderbook.SnapshotMsg{
		LastUpdateID: 1,
	}
	snap.Bids = make([][]string, 0, levels)
	snap.Asks = make([][]string, 0, levels)

	for i := 0; i < levels; i++ {
		// bids step downward, asks step upward
		bidPx := mid - float64(i+1)
		askPx := mid + float64(i+1)

		snap.Bids = append(snap.Bids, []string{
			fmt.Sprintf("%.2f", bidPx),
			"1.0",
		})
		snap.Asks = append(snap.Asks, []string{
			fmt.Sprintf("%.2f", askPx),
			"1.0",
		})
	}
	_ = ob.ApplySnapshot(snap)
}

var prng = rand.New(rand.NewSource(time.Now().UnixNano()))

func randPrice() string {
	// jitter ±50 around 10 000
	return fmt.Sprintf("%.2f", 10_000+prng.Float64()*100-50)
}
func randQty() string {
	return fmt.Sprintf("%.4f", 0.1+prng.Float64()*5)
}

// ---------- actual benchmarks -------------------------------------------

// BenchmarkApplyUpdate tests update throughput with different book sizes
func BenchmarkApplyUpdate_100(b *testing.B) {
	ob := orderbook.New()
	seedBook(ob, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		upd := orderbook.UpdateMsg{
			FinalID: int64(i + 2),
			Bids:    [][]string{{randPrice(), randQty()}},
			Asks:    [][]string{{randPrice(), randQty()}},
		}
		_ = ob.ApplyUpdate(upd)
	}
}

func BenchmarkApplyUpdate_1k(b *testing.B) {
	ob := orderbook.New()
	seedBook(ob, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		upd := orderbook.UpdateMsg{
			FinalID: int64(i + 2),
			Bids:    [][]string{{randPrice(), randQty()}},
			Asks:    [][]string{{randPrice(), randQty()}},
		}
		_ = ob.ApplyUpdate(upd)
	}
}

func BenchmarkApplyUpdate_10k(b *testing.B) {
	ob := orderbook.New()
	seedBook(ob, 10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		upd := orderbook.UpdateMsg{
			FinalID: int64(i + 2),
			Bids:    [][]string{{randPrice(), randQty()}},
			Asks:    [][]string{{randPrice(), randQty()}},
		}
		_ = ob.ApplyUpdate(upd)
	}
}

// BenchmarkBestBidAsk tests read performance
func BenchmarkBestBidAsk(b *testing.B) {
	ob := orderbook.New()
	seedBook(ob, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = ob.BestBid()
		_, _, _ = ob.BestAsk()
	}
}

// BenchmarkSnapshot tests snapshot loading performance
func BenchmarkSnapshot_1k(b *testing.B) {
	const levels = 1000
	snap := orderbook.SnapshotMsg{LastUpdateID: 1}
	snap.Bids = make([][]string, levels)
	snap.Asks = make([][]string, levels)
	for i := 0; i < levels; i++ {
		snap.Bids[i] = []string{fmt.Sprintf("%.2f", 10000.0-float64(i)), "1.0"}
		snap.Asks[i] = []string{fmt.Sprintf("%.2f", 10000.0+float64(i)), "1.0"}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob := orderbook.New()
		_ = ob.ApplySnapshot(snap)
	}
}

// Memory allocation benchmark
func BenchmarkMemoryAlloc(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ob := orderbook.New()
		seedBook(ob, 100)
	}
}

// BenchmarkRealisticWorkload simulates a realistic trading scenario
func BenchmarkRealisticWorkload(b *testing.B) {
	ob := orderbook.New()

	// Load an initial snapshot
	snap := createTestSnapshot(1000)
	ob.ApplySnapshot(snap)

	// Prepare mixed updates (inserts, updates, deletes)
	updates := make([]orderbook.UpdateMsg, 100)
	for i := 0; i < 100; i++ {
		updates[i] = orderbook.UpdateMsg{
			FirstID: int64(i + 1000),
			FinalID: int64(i + 1000),
			Bids: [][]string{
				{fmt.Sprintf("%.2f", 50000.0+float64(i)), fmt.Sprintf("%.8f", 0.1+float64(i)*0.01)},
			},
			Asks: [][]string{
				{fmt.Sprintf("%.2f", 50100.0+float64(i)), fmt.Sprintf("%.8f", 0.1+float64(i)*0.01)},
			},
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Apply updates
		for _, update := range updates {
			ob.ApplyUpdate(update)
		}

		// Read best bid/ask frequently (as would happen in real trading)
		for j := 0; j < 10; j++ {
			ob.BestBid()
			ob.BestAsk()
		}

		// Occasionally get top levels
		if i%10 == 0 {
			ob.GetBids(10)
			ob.GetAsks(10)
		}
	}
}

// BenchmarkConcurrentAccess tests concurrent read/write performance
func BenchmarkConcurrentAccess(b *testing.B) {
	ob := orderbook.New()

	// Load initial snapshot
	snap := createTestSnapshot(1000)
	ob.ApplySnapshot(snap)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		updateCounter := int64(0)
		for pb.Next() {
			// Mix of reads and writes
			switch updateCounter % 4 {
			case 0, 1: // 50% reads
				ob.BestBid()
				ob.BestAsk()
			case 2: // 25% level reads
				ob.GetBids(5)
				ob.GetAsks(5)
			case 3: // 25% updates
				update := orderbook.UpdateMsg{
					FirstID: updateCounter + 1000,
					FinalID: updateCounter + 1000,
					Bids: [][]string{
						{fmt.Sprintf("%.2f", 50000.0+float64(updateCounter)), "0.1"},
					},
					Asks: [][]string{
						{fmt.Sprintf("%.2f", 50100.0+float64(updateCounter)), "0.1"},
					},
				}
				ob.ApplyUpdate(update)
			}
			updateCounter++
		}
	})
}

// createTestSnapshot creates a test snapshot with the specified number of levels
func createTestSnapshot(levels int) orderbook.SnapshotMsg {
	bids := make([][]string, levels)
	asks := make([][]string, levels)

	for i := 0; i < levels; i++ {
		bids[i] = []string{
			fmt.Sprintf("%.2f", 50000.0-float64(i)),
			fmt.Sprintf("%.8f", 0.1+float64(i)*0.001),
		}
		asks[i] = []string{
			fmt.Sprintf("%.2f", 50100.0+float64(i)),
			fmt.Sprintf("%.8f", 0.1+float64(i)*0.001),
		}
	}

	return orderbook.SnapshotMsg{
		LastUpdateID: 1000,
		Bids:         bids,
		Asks:         asks,
	}
}
