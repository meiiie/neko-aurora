package tools

import (
	"context"
	"os"
	"testing"
)

// TestLiveGold hits the real endpoints; run with AURORA_LIVE_TOOLS=1.
func TestLiveGold(t *testing.T) {
	if os.Getenv("AURORA_LIVE_TOOLS") == "" {
		t.Skip("live")
	}
	r, err := New().Gold(context.Background())
	t.Log(r.Text, r.Sources, err)
	s, err := New().WebSearch(context.Background(), "giá vàng SJC hôm nay")
	t.Log(s.Text, err)
}
