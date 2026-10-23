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
	"strconv"
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
	BaseURL   string
	Model     string
	MaxTokens int // output cap per call; a big file write must fit
	key       string
	http      *http.Client
}

func AIFromEnv() *AI {
	a := &AI{BaseURL: DefaultBaseURL, Model: DefaultModel, key: os.Getenv("ZAI_API_KEY"), http: &http.Client{Timeout: 10 * time.Minute}, MaxTokens: 32768} // the turn context bounds a call; 3 min cut long file writes mid-stream
	if v, err := strconv.Atoi(Env("AI_MAX_TOKENS")); err == nil && v > 0 {
		a.MaxTokens = v
	}
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
	return a.StreamWith(ctx, callCfg{}, msgs, onDelta)
}

// callCfg overrides the endpoint for one call: a provider the owner added
// (OpenAI-compatible, key from the vault), a model from the menu, and the
// thinking toggle. Zero values mean the server default (GLM on z.ai).
type callCfg struct {
	Base, Model, Key string
	Thinking         bool
	OnThink          func(string)
	Images           []string // data: URLs for a vision call
}

func (a *AI) StreamWith(ctx context.Context, cc callCfg, msgs []ChatMsg, onDelta func(string)) (string, error) {
	if cc.Base == "" {
		cc.Base, cc.Key = a.BaseURL, a.key
	}
	if cc.Model == "" {
		cc.Model = a.Model
	}
	if cc.Key == "" || a == nil {
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
		out, err = a.streamOnce(ctx, cc, msgs, wrap)
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
	log.Printf("aurora ai model=%s ttft_ms=%d total_ms=%d chars=%d err=%v", cc.Model, first.Milliseconds(), time.Since(t0).Milliseconds(), len(out), err != nil)
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

func (a *AI) streamOnce(ctx context.Context, cc callCfg, msgs []ChatMsg, onDelta func(string)) (string, error) {
	req0 := map[string]any{"model": cc.Model, "messages": msgs, "stream": true, "temperature": 0.6, "max_tokens": a.MaxTokens}
	if len(cc.Images) > 0 && len(msgs) > 0 { // OpenAI vision content parts on the last message
		parts := []map[string]any{{"type": "text", "text": msgs[len(msgs)-1].Content}}
		for _, u := range cc.Images {
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": u}})
		}
		raw := make([]any, 0, len(msgs))
		for _, m := range msgs[:len(msgs)-1] {
			raw = append(raw, m)
		}
		req0["messages"] = append(raw, map[string]any{"role": msgs[len(msgs)-1].Role, "content": parts})
	}
	if strings.Contains(cc.Base, "z.ai") || strings.Contains(cc.Base, "bigmodel") {
		t := "disabled"
		if cc.Thinking {
			t = "enabled"
		}
		req0["thinking"] = map[string]string{"type": t}
	}
	body, _ := json.Marshal(req0)
	req, err := http.NewRequestWithContext(ctx, "POST", cc.Base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+cc.Key)
	res, err := a.http.Do(req)
	if err != nil {
		if ctx.Err() == nil { // the real cause, without the key
			log.Printf("aurora ai unreachable: %s", scrub(err.Error(), cc.Key))
		}
		return "", errors.New("ai_unreachable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return "", fmt.Errorf("ai_http_%d: %s", res.StatusCode, scrub(string(b), cc.Key))
	}
	var full strings.Builder
	cut := false
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
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
				} `json:"delta"`
				Finish string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(p), &ev) != nil || len(ev.Choices) == 0 {
			continue
		}
		if r := ev.Choices[0].Delta.Reasoning; r != "" && cc.OnThink != nil {
			cc.OnThink(r)
		}
		if d := ev.Choices[0].Delta.Content; d != "" {
			full.WriteString(d)
			onDelta(d)
		}
		if ev.Choices[0].Finish == "length" {
			cut = true
		}
	}
	if ctx.Err() != nil {
		return full.String(), ctx.Err()
	}
	if err := sc.Err(); err != nil {
		if full.Len() == 0 {
			return "", errors.New("ai_stream_broken")
		}
		cut = true // broken mid-answer: the tail (often a file body) is missing
	}
	if cut {
		log.Printf("aurora ai reply cut at %d chars (max_tokens=%d)", full.Len(), a.MaxTokens)
		return full.String() + truncMark, nil
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
