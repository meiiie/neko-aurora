package aurora

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestCoreStreamKeepsVietnameseAtEveryByteBoundary(t *testing.T) {
	bun := findCoreBun()
	if bun == "" {
		t.Skip("Bun unavailable")
	}
	const want = "Mực biết trời sắp mưa trước khi giọt đầu tiên rơi xuống sân. Mình đã nhận lời nhắn bổ sung của bạn. 🐈🌧️"
	text, _ := json.Marshal(want + coreStats + " fixture usage must remain hidden]\n")
	entry := filepath.Join(t.TempDir(), "stream.mjs")
	script := "const bytes=Buffer.from(" + string(text) + "); for(let i=0;i<bytes.length;i++){process.stdout.write(bytes.subarray(i,i+1));await Bun.sleep(2)}"
	if err := os.WriteFile(entry, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	c := &Core{Bun: bun, Entry: entry, Home: t.TempDir(), Profile: "fixture", Model: "fixture", key: "fixture"}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var streamed strings.Builder
	invalid := false
	reply, err := c.Stream(ctx, []ChatMsg{{Role: "user", Content: "Local fixture only"}}, func(part string) { invalid = invalid || !utf8.ValidString(part); streamed.WriteString(part) })
	if err != nil || reply != want || streamed.String() != want || invalid {
		t.Fatalf("Unicode stream mismatch: reply=%q streamed=%q invalid=%v err=%v", reply, streamed.String(), invalid, err)
	}
}
