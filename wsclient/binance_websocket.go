package wsclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"raven-dev-challenge/orderbook"

	"github.com/gorilla/websocket"
)

type Client struct {
	symbol       string
	ob           *orderbook.OrderBook
	ws           *websocket.Conn
	lastUpdateId int64
	syncing      bool // true until we apply the first update after snapshot
}

func New(symbol string, ob *orderbook.OrderBook) *Client {
	return &Client{symbol: strings.ToUpper(symbol), ob: ob}
}

// fetchSnapshot grabs a snapshot from Binance REST API
func (c *Client) fetchSnapshot(ctx context.Context) error {
	url := fmt.Sprintf("https://api.binance.com/api/v3/depth?symbol=%s&limit=1000", c.symbol)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("snapshot HTTP %d for %s", resp.StatusCode, c.symbol)
	}

	var snap orderbook.SnapshotMsg
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return err
	}

	c.lastUpdateId = snap.LastUpdateID
	c.syncing = true
	return c.ob.ApplySnapshot(snap)
}

// connectWS connects to Binance WebSocket depth stream
func (c *Client) connectWS(ctx context.Context) error {
	url := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s@depth@100ms", strings.ToLower(c.symbol))
	var err error
	c.ws, _, err = websocket.DefaultDialer.DialContext(ctx, url, nil)
	return err
}

func (c *Client) Run(ctx context.Context) error {
	// First get a snapshot
	if err := c.fetchSnapshot(ctx); err != nil {
		log.Printf("Failed to fetch snapshot: %v", err)
		return err
	}
	log.Println("Got snapshot, connecting to WebSocket...")

	// Connect to live stream
	if err := c.connectWS(ctx); err != nil {
		log.Printf("WebSocket connection failed: %v", err)
		return err
	}
	log.Println("Connected to WebSocket")

	// Process updates
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c.ws.SetReadDeadline(time.Now().Add(10 * time.Second))
		_, msg, err := c.ws.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			return err
		}

		var upd orderbook.UpdateMsg
		if err := json.Unmarshal(msg, &upd); err != nil {
			log.Printf("JSON decode error: %v", err)
			continue
		}

		// Handle sync logic
		if c.syncing {
			if upd.FinalID <= c.lastUpdateId {
				continue // skip old updates
			}
			c.syncing = false
			log.Printf("Sync complete, applying first update after snapshot")
		}

		// Apply the update
		if err := c.ob.ApplyUpdate(upd); err == orderbook.ErrGap {
			log.Println("Gap detected, fetching new snapshot")
			_ = c.fetchSnapshot(ctx)
		} else if err == nil {
			// Occasionally log that we're still processing
			if upd.FinalID%100 == 0 {
				log.Printf("Processed update %d", upd.FinalID)
			}
		}
	}
}
