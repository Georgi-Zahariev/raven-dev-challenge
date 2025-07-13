package main

import (
	"context"
	"log"
	"time"

	"raven-dev-challenge/orderbook"
	"raven-dev-challenge/wsclient"
)

func main() {
	log.Println("Starting order book demo...")

	ob := orderbook.New()
	client := wsclient.New("btcusdt", ob)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run WebSocket client in background
	go func() {
		if err := client.Run(ctx); err != nil {
			log.Printf("WebSocket error: %v", err)
		}
	}()

	// Print best bid/ask every second
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		bid, bqty, hasBid := ob.BestBid()
		ask, aqty, hasAsk := ob.BestAsk()

		if hasBid && hasAsk {
			spread := ask - bid
			log.Printf("BID: $%.2f (%.4f) | ASK: $%.2f (%.4f) | Spread: $%.2f",
				bid, bqty, ask, aqty, spread)
		}
	}
}
