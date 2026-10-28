package hub

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type messageReservation struct {
	Digest string    `json:"digest"`
	At     time.Time `json:"at"`
}

// Reservations outlive the seven-day automatic retry window. Failed ledger
// admission keeps its reservation, so a restart cannot spend it a second time.
func (s *userStore) reserveMessage(id, key, digest string, now time.Time, limit int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.m[id]
	if !ok {
		return errors.New("signin_required")
	}
	if old, ok := u.MessageReservations[key]; ok {
		if old.Digest != digest {
			return errors.New("nonce_conflict")
		}
		return nil
	}
	reservations := map[string]messageReservation{}
	for k, v := range u.MessageReservations {
		if now.Sub(v.At) <= 8*24*time.Hour {
			reservations[k] = v
		}
	}
	if len(reservations) >= 16384 {
		return errors.New("reservation_limit")
	}
	used := u.Used
	if u.UsageDay != day(now) {
		used = 0
	}
	if used >= limit {
		return errors.New("quota_exceeded")
	}
	previousDay, previousUsed, previousReservations := u.UsageDay, u.Used, u.MessageReservations
	reservations[key] = messageReservation{Digest: digest, At: now.UTC()}
	u.UsageDay, u.Used, u.MessageReservations = day(now), used+1, reservations
	if err := s.saveLocked(); err != nil {
		u.UsageDay, u.Used, u.MessageReservations = previousDay, previousUsed, previousReservations
		return errors.New("quota_storage_unavailable")
	}
	return nil
}

func (h *Hub) admitMessage(w http.ResponseWriter, r *http.Request, u User) bool {
	limit := h.cfg.Plans[u.Plan]
	if !strings.HasSuffix(r.URL.Path, "/messages") {
		if h.users.spend(u.ID, h.cfg.Now(), limit) {
			return true
		}
		writeJSON(w, 429, map[string]any{"error": "quota_exceeded", "daily": limit})
		return false
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
	if err != nil || len(b) > 64<<10 {
		writeJSON(w, 413, map[string]string{"error": "too_long"})
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	var payload map[string]json.RawMessage
	var in struct {
		Text      string `json:"text"`
		Nonce     string `json:"clientNonce"`
		Workspace string `json:"workspaceId"`
	}
	if json.Unmarshal(b, &payload) != nil || json.Unmarshal(b, &in) != nil || strings.TrimSpace(in.Text) == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid"})
		return false
	}
	if in.Workspace != "" && in.Workspace != "account-"+u.ID {
		writeJSON(w, 409, map[string]string{"error": "workspace_changed"})
		return false
	}
	if len(in.Nonce) < 8 || len(in.Nonce) > 100 {
		if h.users.spend(u.ID, h.cfg.Now(), limit) {
			return true
		}
		err = errors.New("quota_exceeded")
	} else {
		canonical, _ := json.Marshal(payload)
		digest := sha256.Sum256(canonical)
		key := sha256.Sum256([]byte(r.URL.Path + "\x00" + in.Nonce))
		err = h.users.reserveMessage(u.ID, hex.EncodeToString(key[:]), hex.EncodeToString(digest[:]), h.cfg.Now(), limit)
	}
	if err == nil {
		return true
	}
	code := 503
	if err.Error() == "quota_exceeded" {
		code = 429
	}
	if err.Error() == "nonce_conflict" {
		code = 409
	}
	writeJSON(w, code, map[string]any{"error": err.Error(), "daily": limit})
	return false
}
