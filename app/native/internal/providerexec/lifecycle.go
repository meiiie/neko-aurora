package providerexec

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"nekoaurora.local/native/internal/protocol"
)

var (
	ErrInvalid     = errors.New("saved_key_probe_invalid")
	ErrConflict    = errors.New("saved_key_probe_conflict")
	ErrDenied      = errors.New("saved_key_probe_denied")
	ErrExpired     = errors.New("saved_key_probe_expired")
	ErrLimit       = errors.New("saved_key_probe_limit")
	ErrUnavailable = errors.New("saved_key_probe_unavailable")
)
var accountID = regexp.MustCompile(`^account-[a-f0-9]{32}$`)
var nonce = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
var operationID = regexp.MustCompile(`^keyprobe-[a-f0-9]{32}$`)

const safe = uint64(9007199254740991)

type AccountPin struct {
	AccountID            string `json:"accountId"`
	Revision             uint64 `json:"revision"`
	CredentialGeneration uint64 `json:"credentialGeneration"`
	Provider             string `json:"provider"`
	Endpoint             string `json:"endpoint"`
	Model                string `json:"model"`
}
type Input struct {
	Version         int        `json:"version"`
	ClientNonce     string     `json:"clientNonce"`
	Account         AccountPin `json:"account"`
	ConsentID       string     `json:"consentId"`
	ConsentRevision uint64     `json:"consentRevision"`
	ExpiresAtUnixMS int64      `json:"expiresAtUnixMS"`
}
type Attempt struct {
	OperationID string
	Number      int
	InputSHA256 string
	Input       Input
}
type Result struct {
	Version     int    `json:"version"`
	OperationID string `json:"operationId"`
	Attempt     int    `json:"attempt"`
	InputSHA256 string `json:"inputSHA256"`
	HTTPStatus  int    `json:"httpStatus"`
}
type Receipt struct {
	Version          int    `json:"version"`
	OperationID      string `json:"operationId"`
	InputSHA256      string `json:"inputSHA256"`
	Input            Input  `json:"input"`
	Status           string `json:"status"`
	Attempts         int    `json:"attempts"`
	CreatedAtUnixMS  int64  `json:"createdAtUnixMS"`
	HTTPStatus       int    `json:"httpStatus,omitempty"`
	ExecutionGranted bool   `json:"executionGranted"`
}
type Authorizer interface{ Current(Input) bool }
type Broker interface {
	Probe(context.Context, Attempt) (Result, error)
}
type disk struct {
	Version          int       `json:"version"`
	ObservedAtUnixMS int64     `json:"observedAtUnixMS"`
	Receipts         []Receipt `json:"receipts"`
}
type Controller struct {
	mu        sync.Mutex
	dir       string
	data      disk
	authority Authorizer
	now       func() time.Time
}

func validText(s string, bound int) bool {
	return s != "" && len(s) <= bound && utf8.ValidString(s) && !strings.ContainsAny(s, "\x00\r\n")
}
func ValidAccountPin(p AccountPin) bool {
	u, e := url.Parse(p.Endpoint)
	return e == nil && accountID.MatchString(p.AccountID) && p.Revision > 0 && p.Revision <= safe && p.CredentialGeneration > 0 && p.CredentialGeneration <= safe && (p.Provider == "openai-compatible" || p.Provider == "anthropic") && validText(p.Model, 120) && validText(p.Endpoint, 512) && u.Scheme == "https" && u.Host != "" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && u.Opaque == "" && u.String() == p.Endpoint
}
func validInput(in Input) bool {
	return in.Version == 1 && nonce.MatchString(in.ClientNonce) && ValidAccountPin(in.Account) && nonce.MatchString(in.ConsentID) && in.ConsentRevision > 0 && in.ConsentRevision <= safe && in.ExpiresAtUnixMS > 0 && uint64(in.ExpiresAtUnixMS) <= safe
}
func inputHash(in Input) string {
	b, _ := json.Marshal(in)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func statusValid(s string) bool {
	switch s {
	case "running", "authenticated_probe", "unauthorized", "forbidden", "transient", "refused", "unknown", "revoked", "expired":
		return true
	}
	return false
}
func validReceipt(r Receipt) bool {
	return r.Version == 1 && operationID.MatchString(r.OperationID) && validInput(r.Input) && r.InputSHA256 == inputHash(r.Input) && r.Attempts >= 1 && r.Attempts <= 3 && r.CreatedAtUnixMS > 0 && r.Input.ExpiresAtUnixMS > r.CreatedAtUnixMS && r.Input.ExpiresAtUnixMS-r.CreatedAtUnixMS <= 60000 && statusValid(r.Status) && !r.ExecutionGranted && (r.HTTPStatus == 0 || r.HTTPStatus >= 100 && r.HTTPStatus <= 599)
}
func Open(dir string, authority Authorizer) (*Controller, error) {
	if os.MkdirAll(dir, 0700) != nil {
		return nil, ErrUnavailable
	}
	c := &Controller{dir: dir, authority: authority, now: time.Now, data: disk{Version: 1, Receipts: []Receipt{}}}
	f, e := os.Open(filepath.Join(dir, "saved-key-probes-v1.json"))
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return nil, ErrUnavailable
	}
	raw, e := io.ReadAll(io.LimitReader(f, 65537))
	ce := f.Close()
	if e != nil || ce != nil || len(raw) > 65536 {
		return nil, ErrUnavailable
	}
	canonical, e := protocol.Canonical(raw)
	if e != nil {
		return nil, ErrUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(canonical))
	d.DisallowUnknownFields()
	if d.Decode(&c.data) != nil || c.data.Version != 1 || c.data.ObservedAtUnixMS < 0 || uint64(c.data.ObservedAtUnixMS) > safe || len(c.data.Receipts) > 128 {
		return nil, ErrUnavailable
	}
	seen := map[string]bool{}
	ids := map[string]bool{}
	recovered := false
	for i, r := range c.data.Receipts {
		if !validReceipt(r) || seen[r.Input.ClientNonce] || ids[r.OperationID] || r.CreatedAtUnixMS > c.data.ObservedAtUnixMS {
			return nil, ErrUnavailable
		}
		seen[r.Input.ClientNonce] = true
		ids[r.OperationID] = true
		if r.Status == "running" {
			c.data.Receipts[i].Status = "unknown"
			c.data.Receipts[i].HTTPStatus = 0
			recovered = true
		}
	}
	if recovered && c.save(c.data) != nil {
		return nil, ErrUnavailable
	}
	return c, nil
}
func (c *Controller) save(next disk) error {
	raw, e := json.Marshal(next)
	if e != nil || len(raw) > 65536 {
		return ErrUnavailable
	}
	var random [16]byte
	if _, e = rand.Read(random[:]); e != nil {
		return ErrUnavailable
	}
	p := filepath.Join(c.dir, "keyprobe-"+hex.EncodeToString(random[:])+".tmp")
	f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return ErrUnavailable
	}
	defer os.Remove(p)
	n, e := f.Write(raw)
	if e == nil && n != len(raw) {
		e = io.ErrShortWrite
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil || os.Rename(p, filepath.Join(c.dir, "saved-key-probes-v1.json")) != nil {
		return ErrUnavailable
	}
	c.data = next
	return nil
}
func (c *Controller) Receipt(n string) (Receipt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !nonce.MatchString(n) {
		return Receipt{}, ErrInvalid
	}
	for _, r := range c.data.Receipts {
		if r.Input.ClientNonce == n {
			return r, nil
		}
	}
	return Receipt{}, ErrConflict
}
func (c *Controller) current(in Input) bool { return c.authority != nil && c.authority.Current(in) }
func (c *Controller) Probe(ctx context.Context, in Input, broker Broker, explicitRetry bool) (Receipt, error) {
	c.mu.Lock()
	if !validInput(in) {
		c.mu.Unlock()
		return Receipt{}, ErrInvalid
	}
	hash := inputHash(in)
	index := -1
	var r Receipt
	for i, old := range c.data.Receipts {
		if old.Input.ClientNonce == in.ClientNonce {
			if old.InputSHA256 != hash {
				c.mu.Unlock()
				return Receipt{}, ErrConflict
			}
			index = i
			r = old
			break
		}
	}
	if index >= 0 && !explicitRetry {
		c.mu.Unlock()
		return r, nil
	}
	if index >= 0 && r.Status != "transient" {
		c.mu.Unlock()
		return Receipt{}, ErrConflict
	}
	if index >= 0 && r.Attempts >= 3 {
		c.mu.Unlock()
		return Receipt{}, ErrLimit
	}
	now := c.now().UnixMilli()
	if now < c.data.ObservedAtUnixMS || in.ExpiresAtUnixMS <= now {
		c.mu.Unlock()
		return Receipt{}, ErrExpired
	}
	if in.ExpiresAtUnixMS-now > 60000 {
		c.mu.Unlock()
		return Receipt{}, ErrInvalid
	}
	if ctx.Err() != nil || broker == nil || !c.current(in) {
		c.mu.Unlock()
		return Receipt{}, ErrDenied
	}
	if index < 0 {
		if len(c.data.Receipts) >= 128 {
			c.mu.Unlock()
			return Receipt{}, ErrLimit
		}
		var id [16]byte
		if _, e := rand.Read(id[:]); e != nil {
			c.mu.Unlock()
			return Receipt{}, ErrUnavailable
		}
		r = Receipt{Version: 1, OperationID: "keyprobe-" + hex.EncodeToString(id[:]), InputSHA256: hash, Input: in, CreatedAtUnixMS: now}
		index = len(c.data.Receipts)
	}
	r.Status = "running"
	r.HTTPStatus = 0
	r.Attempts++
	next := c.data
	next.Receipts = append([]Receipt{}, next.Receipts...)
	if index == len(next.Receipts) {
		next.Receipts = append(next.Receipts, r)
	} else {
		next.Receipts[index] = r
	}
	next.ObservedAtUnixMS = now
	if e := c.save(next); e != nil {
		c.mu.Unlock()
		return Receipt{}, e
	}
	c.mu.Unlock()
	deadlineCtx, cancel := context.WithDeadline(ctx, time.UnixMilli(in.ExpiresAtUnixMS))
	defer cancel()
	finished := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-finished:
				return
			case <-deadlineCtx.Done():
				return
			case <-ticker.C:
				if !c.current(in) || c.now().UnixMilli() < now {
					cancel()
					return
				}
			}
		}
	}()
	at := Attempt{OperationID: r.OperationID, Number: r.Attempts, InputSHA256: hash, Input: in}
	var result Result
	var e error
	if c.current(in) {
		result, e = broker.Probe(deadlineCtx, at)
	} else {
		e = ErrDenied
	}
	close(finished)
	<-stopped
	c.mu.Lock()
	defer c.mu.Unlock()
	observed := c.now().UnixMilli()
	status := "unknown"
	httpStatus := 0
	if !c.current(in) {
		status = "revoked"
	} else if observed < now || observed < c.data.ObservedAtUnixMS || observed >= in.ExpiresAtUnixMS {
		status = "expired"
	} else if e == nil && deadlineCtx.Err() == nil && result.Version == 1 && result.OperationID == at.OperationID && result.Attempt == at.Number && result.InputSHA256 == hash && result.HTTPStatus >= 100 && result.HTTPStatus <= 599 {
		httpStatus = result.HTTPStatus
		switch {
		case httpStatus >= 200 && httpStatus <= 299:
			status = "authenticated_probe"
		case httpStatus == 401:
			status = "unauthorized"
		case httpStatus == 403:
			status = "forbidden"
		case httpStatus == 429 || httpStatus >= 500:
			status = "transient"
		default:
			status = "refused"
		}
	}
	r.Status = status
	r.HTTPStatus = httpStatus
	next = c.data
	next.Receipts = append([]Receipt{}, next.Receipts...)
	next.Receipts[index] = r
	if observed > next.ObservedAtUnixMS {
		next.ObservedAtUnixMS = observed
	}
	if e = c.save(next); e != nil {
		return Receipt{}, e
	}
	return r, nil
}
