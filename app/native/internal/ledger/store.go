package ledger

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	ErrConflict    = errors.New("nonce_conflict")
	ErrInvalid     = errors.New("invalid_message")
	ErrNotFound    = errors.New("not_found")
	ErrCorrupt     = errors.New("journal_corrupt")
	ErrUnavailable = errors.New("store_unavailable")
)

const maxFrame = 1 << 20

type Bot struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Revision    int    `json:"revision"`
	Alias       string `json:"alias,omitempty"`
	Description string `json:"description,omitempty"`
}
type Input struct {
	BotID       string `json:"botId"`
	ClientNonce string `json:"clientNonce"`
	Text        string `json:"text"`
	Lane        string `json:"lane"`
}
type Work struct {
	RequestID             string           `json:"requestId"`
	SourceMessageID       string           `json:"sourceMessageId"`
	OwnerUserID           string           `json:"ownerUserId"`
	SpaceID               string           `json:"spaceId"`
	BotID                 string           `json:"botId"`
	ConversationID        string           `json:"conversationId"`
	ClientNonce           string           `json:"clientNonce"`
	Text                  string           `json:"text"`
	Lane                  string           `json:"lane"`
	Status                string           `json:"status"`
	DesiredState          string           `json:"desiredState"`
	Epoch                 uint64           `json:"epoch"`
	BotConfigRevision     int              `json:"botConfigRevision"`
	CreatedAt             string           `json:"createdAt"`
	CreatedSequence       uint64           `json:"createdSequence"`
	AttemptID             string           `json:"attemptId,omitempty"`
	Reply                 string           `json:"reply,omitempty"`
	Failure               string           `json:"failure,omitempty"`
	Core                  *CoreBinding     `json:"core,omitempty"`
	Artifacts             []Artifact       `json:"artifacts,omitempty"`
	Operations            []Operation      `json:"operations,omitempty"`
	Context               *ContextPin      `json:"contextPin,omitempty"`
	SourceBotID           string           `json:"sourceBotId,omitempty"`
	RoutedAlias           string           `json:"routedAlias,omitempty"`
	ContextAccess         *SourceAccess    `json:"contextAccess,omitempty"`
	RuntimeStopped        bool             `json:"runtimeStopped,omitempty"`
	RuntimeStopReceipt    *StoppedReceipt  `json:"runtimeStopReceipt,omitempty"`
	RuntimeAttemptBinding *CoreBinding     `json:"runtimeAttemptBinding,omitempty"`
	ContextRevision       uint64           `json:"contextRevision,omitempty"`
	ContextAttempts       []ContextAttempt `json:"contextAttempts,omitempty"`
}
type Event struct {
	Sequence              uint64 `json:"sequence"`
	Type                  string `json:"type"`
	Work                  Work   `json:"work"`
	Bot                   *Bot   `json:"bot,omitempty"`
	Nonce                 string `json:"nonce,omitempty"`
	CompletionVersion     int    `json:"completionVersion,omitempty"`
	ContextRefreshVersion int    `json:"contextRefreshVersion,omitempty"`
}
type Snapshot struct {
	Sequence     uint64   `json:"sequence"`
	Bots         []Bot    `json:"bots"`
	Work         []Work   `json:"work"`
	Runtime      string   `json:"runtime"`
	Capabilities []string `json:"capabilities"`
}
type record struct {
	Version  int    `json:"version"`
	Event    Event  `json:"event"`
	Previous string `json:"previous"`
	Digest   string `json:"digest"`
}
type Store struct {
	mu        sync.Mutex
	file      *os.File
	lock      *os.File
	work      map[string]Work
	nonces    map[string]string
	events    []Event
	digest    string
	closed    bool
	poisoned  bool
	dir       string
	runtime   string
	bots      []Bot
	botNonces map[string]string
}

var bots = []Bot{
	{ID: "bot-meiiie", Name: "Meiiie", Color: "#b66a55", Revision: 1, Alias: "meiiie"},
	{ID: "bot-davin", Name: "Davin", Color: "#55765f", Revision: 1, Alias: "davin"},
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := lockWriter(filepath.Join(dir, "writer.lock"))
	if err != nil {
		return nil, fmt.Errorf("writer_lock: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(dir, "transactions.jsonl"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		lock.Close()
		return nil, err
	}
	s := &Store{file: file, lock: lock, work: map[string]Work{}, nonces: map[string]string{}, dir: dir, runtime: "core_unavailable", bots: append([]Bot{}, bots...), botNonces: map[string]string{}}
	if err = s.replay(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	err := s.file.Close()
	lockErr := s.lock.Close()
	if err != nil {
		return err
	}
	return lockErr
}
func digestRecord(r record) (string, error) {
	r.Digest = ""
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func (s *Store) replay() error {
	reader := bufio.NewReaderSize(s.file, maxFrame+1)
	var offset int64
	for {
		line, err := reader.ReadSlice('\n')
		if len(line) > maxFrame {
			return ErrCorrupt
		}
		if err == io.EOF {
			if len(line) > 0 {
				if err = s.file.Truncate(offset); err != nil {
					return err
				}
				if err = s.file.Sync(); err != nil {
					return err
				}
			}
			_, err = s.file.Seek(0, io.SeekEnd)
			return err
		}
		if err != nil {
			return err
		}
		var r record
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err = dec.Decode(&r); err != nil {
			return ErrCorrupt
		}
		var extra any
		if dec.Decode(&extra) != io.EOF {
			return ErrCorrupt
		}
		d, err := digestRecord(r)
		if err != nil || r.Version != 1 || r.Previous != s.digest || d != r.Digest || r.Event.Sequence != uint64(len(s.events)+1) {
			return ErrCorrupt
		}
		if !validReplay(s, r.Event) {
			return ErrCorrupt
		}
		s.apply(r.Event)
		s.digest = r.Digest
		offset += int64(len(line))
	}
}
func validReplay(s *Store, e Event) bool {
	if e.ContextRefreshVersion != 0 && (e.ContextRefreshVersion != 1 || e.Type != "work.context_refreshed") {
		return false
	}
	if e.CompletionVersion != 0 && (e.CompletionVersion != 1 || e.Type != "work.completed") {
		return false
	}
	if e.Type == "bot.created" {
		return s.validBotEvent(e)
	}
	if e.Bot != nil || e.Nonce != "" {
		return false
	}
	w := e.Work
	if w.RequestID == "" || w.SourceMessageID == "" || w.OwnerUserID != "local-owner" || w.SpaceID != "local-space" {
		return false
	}
	switch e.Type {
	case "work.accepted":
		if w.Context != nil {
			if (w.Context.Version != 1 && w.Context.Version != 2) || !reflect.DeepEqual(w.Context, s.captureContextVersion(w, w.Context.Version)) {
				return false
			}
		}
		if w.Core != nil || len(w.Operations) > 0 || len(w.Artifacts) > 0 || w.Reply != "" || w.AttemptID != "" || w.Failure != "" || w.ContextAccess != nil || w.RuntimeStopped || w.RuntimeStopReceipt != nil || w.RuntimeAttemptBinding != nil || w.ContextRevision != 0 || len(w.ContextAttempts) != 0 {
			return false
		}
		if s.validInput(Input{w.BotID, w.ClientNonce, w.Text, w.Lane}) != nil || !s.validRoute(w) || w.Status != "waiting_host" || w.DesiredState != "running" || w.Epoch != 1 {
			return false
		}
		if _, ok := s.work[w.RequestID]; ok {
			return false
		}
		if _, ok := s.nonces[workNonceKey(w)]; ok {
			return false
		}
		return w.ConversationID == "conversation-"+w.BotID && w.BotConfigRevision == 1 && w.CreatedSequence == e.Sequence
	case "work.cancelled":
		old, ok := s.work[w.RequestID]
		if ok && old.Status != "waiting_host" {
			return validRuntimeReplay(s, e)
		}
		if !ok || old.Status != "waiting_host" || w.Status != "cancelled" || w.DesiredState != "cancelled" || w.Epoch != old.Epoch+1 {
			return false
		}
		old.Status = w.Status
		old.DesiredState = w.DesiredState
		old.Epoch = w.Epoch
		return reflect.DeepEqual(old, w)
	default:
		return validRuntimeReplay(s, e)
	}
}
func (s *Store) apply(e Event) {
	if e.Bot != nil {
		bot := *e.Bot
		s.bots = append(s.bots, bot)
		s.botNonces[e.Nonce] = bot.ID
		e.Bot = &bot
		s.events = append(s.events, e)
		return
	}
	s.work[e.Work.RequestID] = copyWork(e.Work)
	s.nonces[workNonceKey(e.Work)] = e.Work.RequestID
	e.Work = copyWork(e.Work)
	s.events = append(s.events, e)
}
func (s *Store) commit(e Event) error {
	if s.closed || s.poisoned {
		return ErrUnavailable
	}
	e.Sequence = uint64(len(s.events) + 1)
	r := record{Version: 1, Event: e, Previous: s.digest}
	d, err := digestRecord(r)
	if err != nil {
		return err
	}
	r.Digest = d
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if len(line) > maxFrame {
		return ErrInvalid
	}
	n, err := s.file.Write(line)
	if err != nil || n != len(line) {
		s.poisoned = true
		return ErrUnavailable
	}
	if err = s.file.Sync(); err != nil {
		s.poisoned = true
		return ErrUnavailable
	}
	s.apply(e)
	s.digest = d
	return nil
}
func nonceKey(bot, nonce string) string { return bot + "\x00" + nonce }
func (s *Store) validInput(in Input) error {
	found := false
	for _, bot := range s.bots {
		if bot.ID == in.BotID {
			found = true
		}
	}
	if !found || !utf8.ValidString(in.Text) || strings.TrimSpace(in.Text) == "" || len(in.Text) > 32768 || len(in.ClientNonce) < 8 || len(in.ClientNonce) > 128 {
		return ErrInvalid
	}
	for _, c := range in.ClientNonce {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return ErrInvalid
		}
	}
	if in.Lane != "foreground" && in.Lane != "background" {
		return ErrInvalid
	}
	return nil
}
func newID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b[:]), nil
}
func (s *Store) Accept(in Input) (Work, bool, error) {
	if in.Lane == "" {
		in.Lane = "foreground"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validInput(in); err != nil {
		return Work{}, false, err
	}
	if s.closed || s.poisoned {
		return Work{}, false, ErrUnavailable
	}
	if id, ok := s.nonces[nonceKey(in.BotID, in.ClientNonce)]; ok {
		old := s.work[id]
		if old.Text != in.Text || old.Lane != in.Lane {
			return Work{}, false, ErrConflict
		}
		return copyWork(old), true, nil
	}
	req, err := newID("req-")
	if err != nil {
		return Work{}, false, err
	}
	msg, err := newID("msg-")
	if err != nil {
		return Work{}, false, err
	}
	w := Work{RequestID: req, SourceMessageID: msg, OwnerUserID: "local-owner", SpaceID: "local-space", BotID: in.BotID, ConversationID: "conversation-" + in.BotID, ClientNonce: in.ClientNonce, Text: in.Text, Lane: in.Lane, Status: "waiting_host", DesiredState: "running", Epoch: 1, BotConfigRevision: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), CreatedSequence: uint64(len(s.events) + 1)}
	if target, alias, err := s.resolveRoute(in.Text); err != nil {
		return Work{}, false, err
	} else if target != "" && target != in.BotID {
		w.SourceBotID = in.BotID
		w.BotID = target
		w.RoutedAlias = alias
		w.ConversationID = "conversation-" + target
	}
	w.Context = s.captureContext(w)
	if !boundedRuntimePrompt(w) {
		return Work{}, false, ErrInvalid
	}
	if err = s.commit(Event{Type: "work.accepted", Work: w}); err != nil {
		return Work{}, false, err
	}
	return copyWork(w), false, nil
}
func (s *Store) Cancel(id string) (Work, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[id]
	if !ok {
		return Work{}, ErrNotFound
	}
	if s.closed || s.poisoned {
		return Work{}, ErrUnavailable
	}
	if w.Status == "cancelled" {
		return copyWork(w), nil
	}
	if w.Status != "waiting_host" && w.Status != "starting" && w.Status != "running" && w.Status != "recovery_required" && w.Status != "suspending" && w.Status != "suspended" {
		return Work{}, ErrConflict
	}
	active := w.Status == "starting" || w.Status == "running" || (w.Status == "suspending" && !w.RuntimeStopped)
	w.Status = "cancelled"
	if active {
		w.Status = "cancelling"
	}
	w.DesiredState = "cancelled"
	w.Epoch++
	typ := "work.cancelled"
	if active {
		typ = "work.cancelling"
	}
	if err := s.commit(Event{Type: typ, Work: w}); err != nil {
		return Work{}, err
	}
	return copyWork(w), nil
}
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Snapshot{Sequence: uint64(len(s.events)), Bots: append([]Bot{}, s.bots...), Work: []Work{}, Runtime: s.runtime, Capabilities: []string{"context_access_v1", "context_refresh_v1"}}
	for _, w := range s.work {
		out.Work = append(out.Work, copyWork(w))
	}
	sort.Slice(out.Work, func(i, j int) bool { return out.Work[i].CreatedSequence < out.Work[j].CreatedSequence })
	return out
}
func (s *Store) Events(after uint64) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if after > uint64(len(s.events)) {
		return nil, ErrConflict
	}
	out := append([]Event{}, s.events[int(after):]...)
	for i := range out {
		out[i].Work = copyWork(out[i].Work)
		if out[i].Bot != nil {
			bot := *out[i].Bot
			out[i].Bot = &bot
		}
	}
	return out, nil
}
