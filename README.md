# High-Performance Order Book

A real-time cryptocurrency order book implementation that connects to Binance WebSocket streams. Built for the Raven Developer Challenge.
YouTube link: https://youtu.be/oGTKVbSRoYY , it shows some basic commands and that my app is working properly from my machine.

## Quick Start

```bash
# Clone and run
git clone <your-repo>
cd raven-dev-challenge
go mod tidy  # Install github.com/gorilla/websocket
go run main.go
```

That's it! You'll see live Bitcoin prices updating every second.

## What It Does

- Connects to Binance WebSocket API for real-time market data
- Maintains a sorted order book with great update performance
- Handles snapshot loading and incremental updates automatically
- Shows live bid/ask prices with spread calculation

## Language Choice: Why Go Over Rust

While the challenge suggests Rust for maximum performance, I chose Go for this implementation:

Isn't Go the future? 
Just kidding. Every language has its pros and cons. As a student/developer with advanced knowledge in Java, C, Python, a few months ago, I decided to try Go and this project is perfect for showing Go's advantages.

**Go Advantages:**

- Faster development iteration for prototyping and debugging
- Built-in goroutines perfect for WebSocket handling - very helpful
- Excellent benchmarking and profiling tools - saves time for developing my own, and also adds great visualization
- Memory management that's good enough for financial applications
- Still fast enough to achieve sub-microsecond latency

**Performance Reality:**
I expected to have some kind of implementation issue or poor performance due to my language choice. As shared above, I chose Go because of my knowledge and brief research on whether or not it would manage to be good for what I needed in my demo application. And I was right! My language choice brought simplicity to my project and the bottleneck was algorithmic (O(n log n) sorts). The 67x improvement came from data structure optimization, not low-level language features. Binary search vs sorting was the deal breaker. 

**When Rust Would Be Better:**

- Custom memory allocators if needed
- For ultra-low latency requirements that will be needed in production HFT

For this challenge, Go provided the right balance of performance and development speed. Rust would only overcomplicate my solution and bring some performance updates that are not that important in my case. Moreover, my knowledge in Rust is limited to what it is used for and the learning curve will be to much for me having in mind the short deadline.



## Data Structure Selection & Optimization Strategy

#### Note: PERFORMANCE.md for more performance data
### Initial Design Considerations

I evaluated several approaches for maintaining sorted price levels:

#### Option 1: Simple Map Only

```go
type levelMap struct {
    qtyAt map[float64]float64
}
```

**Pros:** O(1) lookups, simple
**Cons:** No ordering, need to sort for iteration = O(n log n)
**=>** Too slow for frequent best bid/ask access

#### Option 2: Balanced Tree (Red-Black Tree)

**Pros:** O(log n) for all operations
**Cons:** Complex implementation, higher memory overhead, it will overcomplicate the solution without bringing much benefit.
**=>** Overkill for this use case, Go doesn't have built-in trees

#### Option 3: Sorted Slice Only

```go
type levelMap struct {
    levels []PriceLevel // sorted by price
}
```

**Pros:** O(1) best price access, it is cache-friendly
**Cons:** O(n) search for updates, O(n) insertion/deletion 
**Verdict:** Too slow for updates - not HFT satisfactory

#### Option 4: Hybrid Map + Sorted Slice (Selected)

```go
type levelMap struct {
    isBid  bool                    // bid or ask side 
    prices []float64               // sorted prices (desc for bids, asc for asks)
    qtyAt  map[float64]float64     // price -> quantity mapping
}
```

**Why I chose this:**

- **O(1) best price access:** `prices[0]` gives me top of book instantly
- **O(1) quantity lookups:** `qtyAt[price]` for any price level
- **O(log n) insertions:** Binary search to find position + O(n) array insertion

There are some disadvantages, but the benefits exceed the drawbacks:
- **Memory trade-off:** ~2x memory usage but acceptable for the performance gain

This hybrid approach optimizes for the most common operations in trading systems:

1. **Best bid/ask queries** (happens constantly) = O(1)
2. **Price level updates** (frequent) = O(log n)
3. **Full book traversal** (occasional) = O(n)

There is perfect correlation between need and efficiency. The more it's done, the faster it is. This is a valid point that shows the data structure and overall approach are good!

### Binary Search Implementation Strategy

My initial idea was simple and trivial - add it to the list and sort the list. But when tested - too slow!
The key optimization was replacing O(n log n) sorts with O(log n) binary search insertions:

#### Before: Naive Approach

```go
func (lm *levelMap) set(price, qty float64) {
    lm.qtyAt[price] = qty
    // Terrible - resort entire array every time!
    lm.prices = append(lm.prices, price)
    if lm.isBid {
        sort.Sort(sort.Reverse(sort.Float64Slice(lm.prices)))
    } else {
        sort.Float64s(lm.prices)
    }
}
```

**Issue:** O(n log n) complexity for every single update

#### After: Binary Search Insertion

```go
func (lm *levelMap) insertPrice(price float64) {
    n := len(lm.prices)
    var insertPos int
    if lm.isBid {
        // Find position for descending order (highest first) - isBid is True
        insertPos = sort.Search(n, func(i int) bool {
            return lm.prices[i] <= price
        })
    } else {
        // Find position for ascending order (lowest first) - isBid is false (it is an Ask)
        insertPos = sort.Search(n, func(i int) bool {
            return lm.prices[i] >= price
        })
    }
    // Insert at found position
    lm.prices = append(lm.prices, 0)
    copy(lm.prices[insertPos+1:], lm.prices[insertPos:])
    lm.prices[insertPos] = price
}
```

**Result:** O(log n) to find position + O(n) to insert = Much more satisfactory!

### Memory Optimization Strategies

#### 1. Pre-allocation Strategy

Growing slices dynamically during snapshot loading takes too much memory.
Instead of that approach: 

```go
// Before: multiple reallocations
ob.bids = newLevelMap(true)
for _, level := range snapshot.Bids {
    ob.bids.set(price, qty) // Grows slice each time
}

// After: pre-allocate with known capacity
bidCount := len(snapshot.Bids)
ob.bids = &levelMap{
    isBid:  true,
    prices: make([]float64, 0, bidCount),     // pre-allocate capacity
    qtyAt:  make(map[float64]float64, bidCount),
}
```

#### 2. Bulk Processing for Snapshots

```go
// Collect all prices first
bidPrices := make([]float64, 0, bidCount)
for _, lvl := range snap.Bids {
    price, _ := strconv.ParseFloat(lvl[0], 64)
    qty, _ := strconv.ParseFloat(lvl[1], 64)
    if qty > 0 {
        ob.bids.qtyAt[price] = qty
        bidPrices = append(bidPrices, price)
    }
}
// Sort once at the end - much more efficient
sort.Sort(sort.Reverse(sort.Float64Slice(bidPrices)))
ob.bids.prices = bidPrices
```

### Concurrency Design

#### RWMutex Strategy

I chose a single RWMutex for the entire order book rather than per-side locking:

```go
type OrderBook struct {
    mu     sync.RWMutex  // One lock for entire book
    lastID int64
    bids   *levelMap
    asks   *levelMap
}
```

**Why single mutex:**

- **Simplicity:** Easier to reason about, no deadlock risk
- **Read optimization:** Multiple goroutines can read simultaneously
- **Atomic updates:** Ensures bid/ask consistency during updates
- **Performance:** Read-heavy workloads benefit from RWMutex

**Alternative considered:** Per-side mutexes

- **Pros:** Better write concurrency
- **Cons:** Complexity, potential deadlocks, doesn't really match my understanding of trading patterns

Overall, just made more sense to me to use 1 lock (mutex) for operations that are related to each other.

## Development Journey & Technical Challenges

### Challenge 1: WebSocket URL Format

**What I tried:** Initially connected using uppercase symbol format

```go
// This didn't work - no messages received
url := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s@depth@100ms", "BTCUSDT")
```

**What I observed:**

- WebSocket connection succeeded (no error) and initial snapshot loaded correctly
- But no real-time updates were received and order book prices remained static


**Debugging process:**

1. Added raw message logging to verify connection
2. Checked Binance API documentation more carefully
3. Found out that there is a case sensitivity requirement

**Solution:**

```go
// Binance requires lowercase symbols
url := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s@depth@100ms", strings.ToLower(symbol))
```

Easy solution but definitely made me stress about why all parts work separately, but don't want to work together. 

### Challenge 2: WebSocket Message Loop Architecture

**Initial approach:** Used `select` with `default` case

```go
// This was wrong - caused busy spinning 
for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        _, msg, err := c.ws.ReadMessage() // Non-blocking
    }
}
```

**Problem identified:** The `default` case made ReadMessage non-blocking, causing a tight loop that interfered with message reception.

**Solution:** Simplified to blocking read with context checking

```go
for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }
  
    c.ws.SetReadDeadline(time.Now().Add(10 * time.Second))
    _, msg, err := c.ws.ReadMessage() // Blocking read
    if err != nil {
        return err
    }
    // Continue with Process message
}
```

### Challenge 3: Binance Sequence Synchronization

**The Problem:** Binance WebSocket and REST API weren't perfectly synchronized

**What I observed:**

(example run)

Snapshot lastUpdateId: 72665906280
First WebSocket update: U=72665906317 u=72665906336
I was surprised by the big gap - 37 sequence numbers!


**Binance's Official Sync Logic (Complex):**

1. Subscribe to WebSocket stream
2. Buffer all messages
3. Fetch snapshot
4. Drop buffered events where u <= snapshot.lastUpdateId
5. Apply buffered events where U <= lastUpdateId+1 <= u

**My Practical Solution:**
For this demo, I implemented a simplified but robust sync:

```go
if c.syncing {
    if upd.FinalID <= c.lastUpdateId {
        continue // Skip old updates
    }
    // Accept first valid update after snapshot
    c.syncing = false
    log.Printf("Sync complete, applying first update after snapshot")
}
```

**Trade-offs:**

- **Pro:** Simple, works reliably for demo purposes
- **Con:** Might miss some updates during snapshot fetch

Great for the challenge and my demo program, but if made for production:
- **Production note:** it would need full buffer implementation, so there are 0% chances of missing an update

### Challenge 4: Performance Bottleneck Analysis

**Initial Performance (so terrible):** 

```
BenchmarkApplyUpdate_10k-8    15,081    68,382 ns/op    203 B/op    9 allocs/op
```

**Profiling revealed:** Surprisingly or not, 99% of time spent was in `sort.Float64s()`. Dind't think about that initially, it just looked like a simple and quick solution.

**Root cause:**
Every single price level update triggered a full array sort. Very inefficient. Poor initial algorithmic choice on my part. 

```go
// This was what was killing us - O(n log n) every single time!
func (lm *levelMap) set(price, qty float64) {
    lm.qtyAt[price] = qty
    lm.prices = append(lm.prices, price)
    sort.Float64s(lm.prices) // SO EXPENSIVE!
}
```

**Optimization strategy:**

Key steps: 
1. **Maintain sorted invariant** instead of re-sorting
2. **Use binary search** to find insertion point
3. **Shift elements** rather than sort entire array

**Result:** 67x performance improvement! 
This conclusion is made from the result comparisons, there is a results section towards the end of the README.md. Check PERFORMANCE.md for more precise performance data. 

### Challenge 5: Memory Allocation Patterns

**Snapshot loading was allocating excessively:**

```
BenchmarkSnapshot_1k-8    544    2,162,831 ns/op    239,665 B/op    2,075 allocs/op
```

**Investigation:** Each `set()` call during snapshot loading was:

1. Calling `append()` on prices slice (potential reallocation)
2. Calling `sort.Float64s()` (temporary allocations)
3. Called 1000+ times per snapshot

**Solution:** Bulk processing approach

```go
// Pre-allocate everything upfront
bidPrices := make([]float64, 0, len(snap.Bids))
qtyMap := make(map[float64]float64, len(snap.Bids))

// Collect all data first
for _, level := range snap.Bids {
    // Parse and collect
}

// Sort once at the end
sort.Sort(sort.Reverse(sort.Float64Slice(bidPrices)))
```

**Result:** 98% reduction in allocations (2,075 → 25). This was such a big deal!

## Architecture Components

Information about each file, folder, why it exists and how it has been used. 

### Core Order Book (`orderbook/`)

The optimized data structure with three specialized files:

**`orderbook.go`** - Core data structures and thread safety

- levelMap hybrid design (map + sorted slice)
- RWMutex protection for concurrent access
- Best bid/ask accessors (30ns, zero allocations)

**`snapshot.go`** - Bulk loading optimization

- Pre-allocated data structures
- Bulk processing to minimize allocations
- Single sort operation per side

**`update.go`** - Incremental updates

- Binary search insertion for O(log n) performance
- Sequence gap detection
- Zero-quantity level removal

### WebSocket Client (`wsclient/`)

**`binance_websocket.go`** - Network layer isolation

- Snapshot fetching from REST API
- WebSocket stream management
- Sync logic between snapshot and stream
- Auto-recovery on connection issues
- Error handling with specific logging

### Demo Application (`main.go`)

Simple demonstration showing:

- Real-time price updates
- Spread calculation
- Graceful shutdown handling

## Performance Results

The optimizations made a huge difference:

| Operation            | Before    | After    | Improvement          |
| -------------------- | --------- | -------- | -------------------- |
| Update (10k levels)  | 68,382 ns | 1,024 ns | **67x faster** |
| Snapshot (1k levels) | 2.16ms    | 117μs   | **18x faster** |
| Memory allocations   | 2,075/op  | 25/op    | **98% less**   |
| Best bid/ask access  | ~100ns    | 30ns     | **3x faster**  |

Current performance:

- **~1μs** per update (sub-microsecond latency)
- **>1M updates/second** throughput
- **7M+ concurrent operations/second**

## Comprehensive Testing & Benchmarking

### Test Suite for Correctness

**Unit Tests (`orderbook/orderbook_test.go`):**

```bash
go test ./orderbook/ -v
```

Tests cover:

- Snapshot loading correctness
- Incremental update application
- Best bid/ask accuracy
- Sequence gap detection
- Edge cases (empty book, single levels)

**Happy Path Testing:**
Following challenge requirements, tests focus on:

1. **Snapshot followed by updates** without sequence gaps
2. **Normal trading scenarios** with realistic price movements
3. **Concurrent access patterns** typical in trading systems

### Performance Benchmarking Suite

**Complete benchmark execution:**

```bash
# All benchmarks with memory tracking
go test -bench=. -benchmem ./benchmarks/

# Extended runs for stable results
go test -bench=. -benchmem -benchtime=10s ./benchmarks/

# Individual categories
go test -bench=BenchmarkApplyUpdate ./benchmarks/      # Update throughput
go test -bench=BenchmarkBestBidAsk ./benchmarks/       # Read performance  
go test -bench=BenchmarkSnapshot ./benchmarks/         # Snapshot loading
go test -bench=BenchmarkConcurrentAccess ./benchmarks/ # Concurrent performance
go test -bench=BenchmarkRealisticWorkload ./benchmarks/ # Mixed workload
```

### Benchmark Categories & Metrics

#### 1. Update Throughput (Different Book Sizes)

```bash
go test -bench=BenchmarkApplyUpdate -benchmem ./benchmarks/
```

**Measures:**

- Nanoseconds per update operation
- Memory allocations per update
- Bytes allocated per update
- Scalability across book sizes (100, 1k, 10k levels)

**Key Results:**

```
BenchmarkApplyUpdate_100-8     1,132,707    987 ns/op     65 B/op    8 allocs/op
BenchmarkApplyUpdate_1k-8      1,208,479    995 ns/op     65 B/op    8 allocs/op  
BenchmarkApplyUpdate_10k-8     1,033,407  1,024 ns/op     66 B/op    8 allocs/op
```

#### 2. Best Bid/Ask Access Performance

```bash
go test -bench=BenchmarkBestBidAsk -benchmem ./benchmarks/
```

**Measures:** Critical read path performance
**Result:** `35,869,435 ops    29.92 ns/op    0 B/op    0 allocs/op`

#### 3. Snapshot Loading Performance

```bash
go test -bench=BenchmarkSnapshot -benchmem ./benchmarks/
```

**Measures:** Bulk loading efficiency
**Result:** `9,766 ops    116,959 ns/op    107,129 B/op    25 allocs/op`

#### 4. Concurrent Access Performance

```bash
go test -bench=BenchmarkConcurrentAccess -benchmem ./benchmarks/
```

**Measures:** Multi-threaded read/write performance
**Result:** `7,014,172 ops    172.9 ns/op    51 B/op    1 allocs/op`

#### 5. Realistic Trading Workload

```bash
go test -bench=BenchmarkRealisticWorkload -benchmem ./benchmarks/
```

**Simulates:** Mixed operations (updates + reads + level queries)
**Result:** `497,822 ops    2,416 ns/op    32 B/op    0 allocs/op`

### Memory Consumption Analysis

**Pre-optimization snapshot loading:**

- **239,665 bytes/operation** - Excessive allocations
- **2,075 allocations/operation** - GC pressure nightmare

**Post-optimization results:**

- **107,129 bytes/operation** - 55% reduction
- **25 allocations/operation** - 98% reduction

### Profiling Commands for Deep Analysis

**CPU Profiling:**

```bash
go test -bench=BenchmarkRealisticWorkload -cpuprofile=cpu.prof ./benchmarks/
go tool pprof cpu.prof
# In pprof choose what you want to see: top10, list function_name, web
```

**Memory Profiling:**

```bash
go test -bench=BenchmarkRealisticWorkload -memprofile=mem.prof ./benchmarks/
go tool pprof mem.prof
# In pprof: top10, list function_name, web
```

**Allocation Tracking:**

```bash
go test -bench=BenchmarkApplyUpdate -benchmem -memprofilerate=1 ./benchmarks/
```

### Build Instructions & Dependencies

**Prerequisites:**

- Go 1.19+ (for generics support in benchmarks)

**Build process:**

```bash
# Download dependencies
go mod tidy

# Build binary
go build -o orderbook-demo .

# Run with optimizations
go build -ldflags="-s -w" -o orderbook-demo .

# Cross-compilation example
GOOS=linux GOARCH=amd64 go build -o orderbook-linux-amd64 .
```

**Running benchmarks:**

```bash
# Basic benchmark run
go test -bench=. ./benchmarks/

# With memory allocation tracking  
go test -bench=. -benchmem ./benchmarks/

# Longer runs for statistical significance
go test -bench=. -benchmem -benchtime=30s -count=5 ./benchmarks/

# Save results for comparison
go test -bench=. -benchmem ./benchmarks/ > bench_results.txt
```

## What I Learned

1. **Data structure choice matters** - Using both a map and sorted slice gave me O(1) reads and O(log n) updates
2. **Binary search beats sorting** - Don't re-sort entire arrays when you can insert at the right position
3. **Pre-allocation helps** - Knowing capacity upfront reduces memory allocations significantly
4. **Binance sync is tricky** - The gap detection between snapshot and stream requires careful handling

## Files Structure

```
raven-dev-challenge/
├── main.go                    # Demo program
├── orderbook/
│   ├── orderbook.go          # Core data structures and thread safety
│   ├── snapshot.go           # Bulk loading from REST API
│   └── update.go             # Incremental updates from WebSocket
├── wsclient/
│   └── binance_websocket.go  # Network layer for Binance API
└── benchmarks/
    └── benchmark_test.go     # Performance tests
```

---

**Built with Go for simplicity and performance. No external frameworks needed beyond WebSocket support.**
