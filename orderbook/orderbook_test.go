package orderbook

import (
	"encoding/json"
	"os"
	"testing"
)

func loadSnapshot(t *testing.T, f string) SnapshotMsg {
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var snap SnapshotMsg
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatalf("json: %v", err)
	}
	return snap
}

func TestSnapshotAndUpdate(t *testing.T) {
	ob := New()
	snap := loadSnapshot(t, "testdata/snapshot.json")

	if err := ob.ApplySnapshot(snap); err != nil {
		t.Fatalf("ApplySnapshot: %v", err)
	}

	// Best bid / ask after snapshot
	if p, _, _ := ob.BestBid(); p != 10000.0 {
		t.Fatalf("best bid expected 10000, got %.2f", p)
	}

	// Synthetic “depthUpdate”
	upd := UpdateMsg{
		FinalID: snap.LastUpdateID + 1,
		Bids:    [][]string{{"10000.0", "0"}}, // remove best bid
		Asks:    [][]string{{"10010.0", "0"}}, // remove best ask
	}
	if err := ob.ApplyUpdate(upd); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if p, _, _ := ob.BestBid(); p >= 10000.0 {
		t.Fatalf("best bid not updated; got %.2f", p)
	}
}
