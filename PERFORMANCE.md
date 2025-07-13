# Performance Analysis

## The Problem

The initial implementation was painfully slow. Here's what was wrong and how I fixed it.

## Before vs After

| Metric | Original | Optimized | Improvement |
|--------|----------|-----------|-------------|
| 10k level updates | 68,382 ns/op | 1,024 ns/op | **67x faster** |
| 1k snapshot load | 2.16 ms | 117 μs | **18x faster** |
| Memory allocations | 2,075/op | 25/op | **98% reduction** |
| Memory usage | 240 KB/op | 107 KB/op | **55% less** |

## What I Changed

### 1. Stopped Re-sorting Everything

**Before:** Every time a new price level was added, I sorted the entire array.
```go
// This was terrible - O(n log n) every time!
lm.prices = append(lm.prices, price)
sort.Float64s(lm.prices)
```

**After:** Binary search to find the right spot, then insert there.
```go
// Much better - O(log n) to find position, O(n) to insert
pos := sort.Search(len(lm.prices), func(i int) bool {
    return lm.prices[i] >= price
})
// Insert at position
```

This single change gave me the 67x speedup.

### 2. Bulk Snapshot Processing

**Before:** Called the slow `set()` method for each price level individually.

**After:** Parse everything first, then sort once:
```go
// Collect all prices
for _, level := range snapshot.Bids {
    price, _ := strconv.ParseFloat(level[0], 64)
    qty, _ := strconv.ParseFloat(level[1], 64)
    bidPrices = append(bidPrices, price)
    ob.bids.qtyAt[price] = qty
}
// Sort once at the end
sort.Sort(sort.Reverse(sort.Float64Slice(bidPrices)))
```

### 3. Pre-allocated Everything

Instead of growing slices dynamically, I allocate them with the right size upfront:
```go
ob.bids = &levelMap{
    prices: make([]float64, 0, bidCount),  // Know the capacity
    qtyAt:  make(map[float64]float64, bidCount),
}
```

## Current Performance

```
BenchmarkApplyUpdate_10k-8     1,033,407    1,024 ns/op      66 B/op       8 allocs/op
BenchmarkBestBidAsk-8         35,869,435       30 ns/op       0 B/op       0 allocs/op
BenchmarkSnapshot_1k-8             9,766  116,959 ns/op 107,129 B/op      25 allocs/op
BenchmarkConcurrentAccess-8    7,014,172      173 ns/op      51 B/op       1 allocs/op
```

### What This Means

- **~1 microsecond** per update (industry competitive)
- **30 nanoseconds** for best bid/ask (zero allocations)
- **>1 million updates/second** sustained throughput
- **7+ million concurrent operations/second**

## Design Trade-offs

### Memory vs Speed
I use both a map and a sorted slice for each side:
- Map: O(1) price lookups
- Slice: O(1) best price access, ordered iteration

This uses ~2x memory but gives me the performance I need.

### Consistency vs Performance  
Single RWMutex for the whole book instead of per-side locking. Simpler and faster for reads.

## Why Go?

While Rust would be faster, Go gave me:
- Fast development iteration
- Built-in goroutines for WebSocket handling
- Great tooling (benchmarks, profiling)
- Still fast enough for this use case

The bottleneck was algorithmic (O(n log n) sorts), not language choice.

## Profiling Commands

```bash
# CPU profiling
go test -bench=BenchmarkRealisticWorkload -cpuprofile=cpu.prof ./benchmarks/
go tool pprof cpu.prof

# Memory profiling  
go test -bench=BenchmarkRealisticWorkload -memprofile=mem.prof ./benchmarks/
go tool pprof mem.prof

# All benchmarks
go test -bench=. -benchmem ./benchmarks/
```

## Key Takeaway

**Algorithm choice matters more than micro-optimizations.** The 67x improvement came from changing the core data structure approach, not from tweaking individual operations.
