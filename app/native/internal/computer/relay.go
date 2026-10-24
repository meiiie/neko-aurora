package computer

// Relay moves a bot's home tarball between our server and its Railway VM
// over plain HTTPS instead of base64 chunks through sandboxExec stdout.
// A 2 MB pptx+mp4 home took ~220 exec calls per pull and hit Railway's
// API rate limit (live pptx task 2026-10-23: "railway: http 429"); with
// the relay a sync is one exec. Each URL carries a random single-use
// token that expires after relayTTL, so the route needs no session.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const relayTTL = 10 * time.Minute

type relaySlot struct {
	get     bool   // true: the VM downloads data; false: the VM uploads
	data    []byte // download payload
	max     int64
	got     chan []byte
	expires time.Time
}

type Relay struct {
	Base string // public base URL, e.g. https://app.example.com
	mu   sync.Mutex
	m    map[string]*relaySlot
	Now  func() time.Time
}

func NewRelay(base string) *Relay {
	return &Relay{Base: strings.TrimRight(base, "/"), m: map[string]*relaySlot{}, Now: time.Now}
}

const RelayPath = "/relay/computer/"

func (r *Relay) add(s *relaySlot) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.Now()
	for k, v := range r.m {
		if now.After(v.expires) {
			delete(r.m, k)
		}
	}
	s.expires = now.Add(relayTTL)
	r.m[tok] = s
	return tok
}

func (r *Relay) take(tok string, get bool) *relaySlot {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.m[tok]
	if s == nil || s.get != get || r.Now().After(s.expires) {
		return nil
	}
	delete(r.m, tok) // single use
	return s
}

// Offer publishes data for one download and returns its URL.
func (r *Relay) Offer(data []byte) string {
	return r.Base + RelayPath + r.add(&relaySlot{get: true, data: data})
}

// Expect returns an upload URL and a wait func for the uploaded bytes.
func (r *Relay) Expect(max int64) (string, func(context.Context) ([]byte, error)) {
	s := &relaySlot{max: max, got: make(chan []byte, 1)}
	tok := r.add(s)
	return r.Base + RelayPath + tok, func(ctx context.Context) ([]byte, error) {
		select {
		case b := <-s.got:
			return b, nil
		case <-ctx.Done():
			r.take(tok, false)
			return nil, ctx.Err()
		case <-time.After(30 * time.Second): // the exec already returned: upload is late or never came
			r.take(tok, false)
			return nil, errors.New("relay upload missing")
		}
	}
}

func (r *Relay) ServeHTTP(w http.ResponseWriter, q *http.Request) {
	tok := strings.TrimPrefix(q.URL.Path, RelayPath)
	w.Header().Set("Cache-Control", "no-store")
	switch q.Method {
	case http.MethodGet:
		s := r.take(tok, true)
		if s == nil {
			http.NotFound(w, q)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(s.data)
	case http.MethodPut, http.MethodPost:
		s := r.take(tok, false)
		if s == nil {
			http.NotFound(w, q)
			return
		}
		b, err := io.ReadAll(io.LimitReader(q.Body, s.max+1))
		if err != nil || int64(len(b)) > s.max {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			s.got <- nil
			return
		}
		s.got <- b
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}
