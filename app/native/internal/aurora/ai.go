package aurora

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://api.z.ai/api/coding/paas/v4"
	DefaultModel   = "glm-5.3"
)

// AI is an OpenAI-compatible chat-completions client (z.ai GLM Coding Plan).
// The key is read from the environment on the server only and never leaves it.
type AI struct {
	BaseURL string
	Model   string
	key     string
	http    *http.Client
}

func AIFromEnv() *AI {
	a := &AI{BaseURL: DefaultBaseURL, Model: DefaultModel, key: os.Getenv("ZAI_API_KEY"), http: &http.Client{Timeout: 3 * time.Minute}}
	if v := Env("AI_BASE_URL"); v != "" {
		a.BaseURL = strings.TrimRight(v, "/")
	}
	if v := Env("AI_MODEL"); v != "" {
		a.Model = v
	}
	return a
}

func (a *AI) Configured() bool { return a != nil && a.key != "" }

type ChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

var ErrNoAI = errors.New("ai_not_configured")

// Stream sends messages and calls onDelta for each content token. It returns
// the full content.
// Stream runs one streamed completion. A transient failure that happens before
// any text (unreachable, 429, 5xx, broken stream) is retried up to twice with
// exponential back-off; time-to-first-token is logged for latency tracking.
func (a *AI) Stream(ctx context.Context, msgs []ChatMsg, onDelta func(string)) (string, error) {
	if !a.Configured() {
		return "", ErrNoAI
	}
	t0 := time.Now()
	var first time.Duration
	wrap := func(d string) {
		if first == 0 {
			first = time.Since(t0)
		}
		onDelta(d)
	}
	var out string
	var err error
	for try := 0; try < maxAttempts+2; try++ {
		out, err = a.streamOnce(ctx, msgs, wrap)
		limit := maxAttempts
		if err != nil && err.Error() == "ai_unreachable" {
			// a fresh container's egress can take ~10-20s (live restart
			// test: resumed run failed ai_unreachable right after boot)
			limit = maxAttempts + 2
		}
		if err == nil || first > 0 || ctx.Err() != nil || !retryable(err) || try >= limit-1 {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(backoff(try)):
		}
	}
	log.Printf("aurora ai model=%s ttft_ms=%d total_ms=%d chars=%d err=%v", a.Model, first.Milliseconds(), time.Since(t0).Milliseconds(), len(out), err != nil)
	return out, err
}

// Exponential back-off with jitter for transient provider errors before the
// first token (429/5xx/unreachable): ~1.5s, ~3s. A stream that already
// produced text is never replayed, so the owner never sees duplicated words.
var retryDelay = 1500 * time.Millisecond

const maxAttempts = 3

func backoff(try int) time.Duration {
	d := retryDelay << try
	return d + time.Duration(rand.Int64N(int64(d)/4+1))
}

func retryable(err error) bool {
	e := err.Error()
	return e == "ai_unreachable" || e == "ai_stream_broken" || strings.HasPrefix(e, "ai_http_429") || strings.HasPrefix(e, "ai_http_5")
}

func (a *AI) streamOnce(ctx context.Context, msgs []ChatMsg, onDelta func(string)) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": a.Model, "messages": msgs, "stream": true, "temperature": 0.6,
		"thinking": map[string]string{"type": "disabled"},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", a.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+a.key)
	res, err := a.http.Do(req)
	if err != nil {
		return "", errors.New("ai_unreachable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return "", fmt.Errorf("ai_http_%d: %s", res.StatusCode, scrub(string(b), a.key))
	}
	var full strings.Builder
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		p := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if p == "[DONE]" {
			break
		}
		var ev struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(p), &ev) != nil || len(ev.Choices) == 0 {
			continue
		}
		if d := ev.Choices[0].Delta.Content; d != "" {
			full.WriteString(d)
			onDelta(d)
		}
	}
	if ctx.Err() != nil {
		return full.String(), ctx.Err()
	}
	if err := sc.Err(); err != nil && full.Len() == 0 {
		return "", errors.New("ai_stream_broken")
	}
	return full.String(), nil
}

func scrub(s, key string) string {
	if key != "" {
		s = strings.ReplaceAll(s, key, "***")
	}
	return strings.TrimSpace(s)
}

// Models lists model ids exposed by the endpoint (for diagnostics).
func (a *AI) Models(ctx context.Context) ([]string, error) {
	if !a.Configured() {
		return nil, ErrNoAI
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", a.BaseURL+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+a.key)
	res, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("models_http_%d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	ids := []string{}
	for _, d := range out.Data {
		ids = append(ids, d.ID)
	}
	return ids, nil
}
