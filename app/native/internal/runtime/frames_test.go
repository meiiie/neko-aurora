package runtime

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

type frameProbe struct {
	bytes.Buffer
	short bool
}

func (w *frameProbe) Write(b []byte) (int, error) {
	if len(b) > 1024 {
		return 0, io.ErrShortBuffer
	}
	if w.short {
		return len(b) - 1, nil
	}
	return w.Buffer.Write(b)
}
func TestHostLargeFramesRemainExactAcrossBoundedWrites(t *testing.T) {
	w := &frameProbe{}
	value := map[string]string{"text": strings.Repeat("Tiếng Việt 🐈\n", 2500)}
	if err := json.NewEncoder(hostFrameWriter{w}).Encode(value); err != nil {
		t.Fatal(err)
	}
	var actual map[string]string
	if err := json.Unmarshal(w.Bytes(), &actual); err != nil || actual["text"] != value["text"] {
		t.Fatal("transport altered frame", err)
	}
	w = &frameProbe{short: true}
	if _, err := (hostFrameWriter{w}).Write([]byte("frame")); err != io.ErrShortWrite {
		t.Fatal("silent partial frame", err)
	}
}
