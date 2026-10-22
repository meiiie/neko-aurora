package aurora

import (
	"testing"
	"time"
)

func TestSendRateLimit(t *testing.T) {
	r := rate{max: 3}
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !r.allow(now) {
			t.Fatal("early block")
		}
	}
	if r.allow(now) {
		t.Fatal("4th allowed")
	}
	if !r.allow(now.Add(61 * time.Second)) {
		t.Fatal("window did not slide")
	}
}
