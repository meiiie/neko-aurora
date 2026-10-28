package aurora

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"nekoaurora.local/native/internal/ledger"
)

type scheduleTestInput struct {
	WorkspaceID string `json:"workspaceId"`
	Revision    uint64 `json:"revision"`
	ClientNonce string `json:"clientNonce"`
}

type scheduleTestReceipt struct {
	Found     bool   `json:"found"`
	RequestID string `json:"requestId,omitempty"`
	BotID     string `json:"botId,omitempty"`
	Duplicate bool   `json:"duplicate"`
}

var routineTestNonce = regexp.MustCompile(`^[A-Za-z0-9_-]{8,100}$`)

func scheduleTestKey(id, nonce string) string {
	return fmt.Sprintf("routine-test-%x", sha256.Sum256([]byte(id+"\x00"+nonce)))
}

func scheduleTestPrefix(revision uint64) string {
	return fmt.Sprintf("[Chạy thử lịch · phiên bản %d]\n", revision)
}

func (s *Service) validateScheduleTest(id string, in scheduleTestInput) error {
	if in.WorkspaceID != s.workspaceID {
		return ledger.ErrConflict
	}
	if id == "" || len(id) > 128 || !routineTestNonce.MatchString(in.ClientNonce) || in.Revision == 0 || in.Revision > 1<<53-1 {
		return ErrInvalid
	}
	return nil
}

func (s *Service) admitRoutine(bot, nonce, text string) error {
	if s.routineAdmission == nil {
		return nil
	}
	return s.routineAdmission(fmt.Sprintf("%x", sha256.Sum256([]byte(bot+"\x00"+nonce))), fmt.Sprintf("%x", sha256.Sum256([]byte(text))))
}

// The receipt is the original atomic owner-message record, not a matching nonce alone.
func (s *Service) scheduleTestReceipt(id string, in scheduleTestInput) (scheduleTestReceipt, error) {
	key := scheduleTestKey(id, in.ClientNonce)
	for _, work := range s.L.Snapshot().Work {
		if work.ClientNonce != key {
			continue
		}
		receipt, ok := s.L.OwnerMessageForWork(work.RequestID)
		if !ok || receipt.Input.ClientNonce != key || len(receipt.WorkIDs) != 1 || receipt.Input.ReferenceID != "" || receipt.Input.ReplyTo != "" || !strings.HasPrefix(receipt.Input.Text, scheduleTestPrefix(in.Revision)) {
			return scheduleTestReceipt{}, ledger.ErrConflict
		}
		return scheduleTestReceipt{Found: true, RequestID: work.RequestID, BotID: receipt.Input.ThreadID, Duplicate: true}, nil
	}
	return scheduleTestReceipt{}, nil
}

func (s *Service) runScheduleTest(id string, in scheduleTestInput) (scheduleTestReceipt, error) {
	if err := s.validateScheduleTest(id, in); err != nil {
		return scheduleTestReceipt{}, err
	}
	s.sched.mu.Lock()
	defer s.sched.mu.Unlock()
	if receipt, err := s.scheduleTestReceipt(id, in); err != nil || receipt.Found {
		return receipt, err
	}
	if s.L.ReadOnly() {
		return scheduleTestReceipt{}, ledger.ErrReadOnly
	}
	for _, x := range s.sched.L {
		if x.ID != id {
			continue
		}
		if max(1, x.Revision) != in.Revision {
			return scheduleTestReceipt{}, ledger.ErrConflict
		}
		if !s.sendRate.allow(s.Now()) {
			return scheduleTestReceipt{}, errors.New("rate_limited")
		}
		nonce, text := scheduleTestKey(id, in.ClientNonce), scheduleTestPrefix(in.Revision)+x.Text
		if err := s.admitRoutine(x.Bot, nonce, text); err != nil {
			return scheduleTestReceipt{}, err
		}
		works, duplicate, err := s.L.AcceptOwnerMessage(ledger.OwnerMessageInput{ThreadID: x.Bot, ClientNonce: nonce, Text: text}, ledger.OwnerMessagePlan{BotIDs: []string{x.Bot}})
		if err != nil {
			return scheduleTestReceipt{}, err
		}
		return scheduleTestReceipt{Found: true, RequestID: works[0].RequestID, BotID: x.Bot, Duplicate: duplicate}, nil
	}
	return scheduleTestReceipt{}, ErrNotFound
}

func scheduleTestFail(w http.ResponseWriter, err error) {
	switch err.Error() {
	case "quota_exceeded", "rate_limited":
		w.Header().Set("Retry-After", "60")
		writeJSON(w, 429, map[string]string{"error": err.Error()})
	case "quota_storage_unavailable", "reservation_limit", "signin_required":
		writeJSON(w, 503, map[string]string{"error": "admission_unavailable"})
	case "nonce_conflict":
		fail(w, ledger.ErrConflict)
	default:
		fail(w, err)
	}
}

func (s *Service) scheduleTestRoutes() {
	s.mux.HandleFunc("POST /app/api/schedules/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		var in scheduleTestInput
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		out, err := s.runScheduleTest(r.PathValue("id"), in)
		if err != nil {
			scheduleTestFail(w, err)
			return
		}
		s.sync()
		s.poke()
		writeJSON(w, 202, out)
	})
	s.mux.HandleFunc("GET /app/api/schedules/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if len(q) != 3 || len(q["workspaceId"]) != 1 || len(q["revision"]) != 1 || len(q["clientNonce"]) != 1 {
			fail(w, ErrInvalid)
			return
		}
		rev, err := strconv.ParseUint(q.Get("revision"), 10, 64)
		in := scheduleTestInput{WorkspaceID: q.Get("workspaceId"), Revision: rev, ClientNonce: q.Get("clientNonce")}
		if err != nil {
			fail(w, ErrInvalid)
			return
		}
		if err = s.validateScheduleTest(r.PathValue("id"), in); err != nil {
			fail(w, err)
			return
		}
		s.sched.mu.Lock()
		out, err := s.scheduleTestReceipt(r.PathValue("id"), in)
		s.sched.mu.Unlock()
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
}
