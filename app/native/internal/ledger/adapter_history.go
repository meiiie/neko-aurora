package ledger

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"sort"
	"time"
	"unicode/utf8"
)

var (
	ErrAdapterHistoryCursorInvalid    = errors.New("adapter_history_cursor_invalid")
	ErrAdapterHistoryBindingMismatch  = errors.New("adapter_history_binding_mismatch")
	ErrAdapterHistoryScopeChanged     = errors.New("adapter_history_scope_changed")
	ErrAdapterHistoryAuthorityChanged = errors.New("adapter_history_authority_changed")
	ErrAdapterHistorySnapshotExpired  = errors.New("adapter_history_snapshot_expired")
	ErrAdapterHistoryBudgetExceeded   = errors.New("adapter_history_budget_exceeded")
)

const adapterHistoryMaxRows = 8192
const adapterHistoryMaxBytes = 64 << 20
const adapterHistoryPageRows = 128
const adapterHistoryPageBytes = 900000
const adapterHistoryTTLMS = 300000
const adapterHistoryMaxSafe = 1<<53 - 1

type AdapterHistoryRow struct {
	ReplyID         string     `json:"replyId"`
	RequestID       string     `json:"requestId"`
	SourceMessageID string     `json:"sourceMessageId"`
	BotID           string     `json:"botId"`
	Role            string     `json:"role"`
	Text            string     `json:"text"`
	Status          string     `json:"status"`
	Sequence        uint64     `json:"sequence"`
	Artifacts       []Artifact `json:"artifacts"`
	Availability    string     `json:"availability"`
}

type AdapterHistoryPage struct {
	Version            int                 `json:"version"`
	AdapterSessionID   string              `json:"adapterSessionId"`
	SourceApp          string              `json:"sourceApp"`
	SourceSessionID    string              `json:"sourceSessionId"`
	SourceThreadID     string              `json:"sourceThreadId"`
	ChannelID          string              `json:"channelId"`
	ChannelRevision    uint64              `json:"channelRevision"`
	OwnerUserID        string              `json:"ownerUserId"`
	SpaceID            string              `json:"spaceId"`
	ReturnRoute        string              `json:"returnRoute"`
	SnapshotID         string              `json:"snapshotId"`
	SnapshotCursor     uint64              `json:"snapshotCursor"`
	AuthorityDigest    string              `json:"authorityDigest"`
	HistoryDigest      string              `json:"historyDigest"`
	Offset             uint64              `json:"offset"`
	TotalRows          uint64              `json:"totalRows"`
	TotalBytes         uint64              `json:"totalBytes"`
	NextCursor         *string             `json:"nextCursor"`
	ConfirmationCursor *string             `json:"confirmationCursor"`
	Complete           bool                `json:"complete"`
	Rows               []AdapterHistoryRow `json:"rows"`
}

type adapterHistoryCursor struct {
	scope, authority, history     [32]byte
	head, offset, issued, expires uint64
}

type adapterHistoryWork struct {
	work     Work
	sequence uint64
}

func adapterHistoryU64(w hash.Hash, n uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], n)
	w.Write(encoded[:])
}

func adapterHistoryString(w hash.Hash, value string) {
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], uint32(len(value)))
	w.Write(encoded[:])
	w.Write([]byte(value))
}

func adapterHistoryScope(a AdapterSession) []byte {
	var out bytes.Buffer
	for _, value := range []string{a.ID, a.SourceApp, a.SourceSessionID, a.SourceThreadID, a.ChannelID, a.OwnerUserID, a.SpaceID, a.ReturnRoute} {
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(value)))
		out.Write(size[:])
		out.WriteString(value)
	}
	var revision [8]byte
	binary.BigEndian.PutUint64(revision[:], a.ChannelRevision)
	out.Write(revision[:])
	return out.Bytes()
}

func adapterHistoryRowBytes(row AdapterHistoryRow) ([]byte, error) {
	var out bytes.Buffer
	writeString := func(value string) bool {
		if !utf8.ValidString(value) {
			return false
		}
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(value)))
		out.Write(size[:])
		out.WriteString(value)
		return true
	}
	for _, value := range []string{row.ReplyID, row.RequestID, row.SourceMessageID, row.BotID, row.Role, row.Status, row.Availability} {
		if !writeString(value) {
			return nil, ErrAdapterHistoryBudgetExceeded
		}
	}
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], row.Sequence)
	out.Write(number[:])
	if len(row.Text) > 65536 || !writeString(row.Text) || len(row.Artifacts) > 8 {
		return nil, ErrAdapterHistoryBudgetExceeded
	}
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], uint32(len(row.Artifacts)))
	out.Write(count[:])
	for _, artifact := range row.Artifacts {
		if len(artifact.Filename) > 80 || artifact.Bytes < 0 || uint64(artifact.Bytes) > adapterHistoryMaxSafe {
			return nil, ErrAdapterHistoryBudgetExceeded
		}
		for _, value := range []string{artifact.ID, artifact.Filename, artifact.SHA256} {
			if !writeString(value) {
				return nil, ErrAdapterHistoryBudgetExceeded
			}
		}
		binary.BigEndian.PutUint64(number[:], uint64(artifact.Bytes))
		out.Write(number[:])
	}
	return out.Bytes(), nil
}

func adapterHistoryToken(c adapterHistoryCursor, key [32]byte) string {
	raw := make([]byte, 129, 161)
	raw[0] = 1
	copy(raw[1:33], c.scope[:])
	for i, value := range []uint64{c.head, c.offset, c.issued, c.expires} {
		binary.BigEndian.PutUint64(raw[33+i*8:41+i*8], value)
	}
	copy(raw[65:97], c.authority[:])
	copy(raw[97:129], c.history[:])
	mac := hmac.New(sha256.New, key[:])
	mac.Write(raw)
	raw = append(raw, mac.Sum(nil)...)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func adapterHistoryParseToken(token string, scope []byte, key [32]byte, now uint64) (adapterHistoryCursor, error) {
	c := adapterHistoryCursor{}
	if len(token) != 215 {
		return c, ErrAdapterHistoryCursorInvalid
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != 161 || raw[0] != 1 || base64.RawURLEncoding.EncodeToString(raw) != token {
		return c, ErrAdapterHistoryCursorInvalid
	}
	mac := hmac.New(sha256.New, key[:])
	mac.Write(raw[:129])
	if !hmac.Equal(mac.Sum(nil), raw[129:]) {
		return c, ErrAdapterHistoryCursorInvalid
	}
	copy(c.scope[:], raw[1:33])
	if c.scope != sha256.Sum256(scope) {
		return c, ErrAdapterHistoryBindingMismatch
	}
	c.head = binary.BigEndian.Uint64(raw[33:41])
	c.offset = binary.BigEndian.Uint64(raw[41:49])
	c.issued = binary.BigEndian.Uint64(raw[49:57])
	c.expires = binary.BigEndian.Uint64(raw[57:65])
	copy(c.authority[:], raw[65:97])
	copy(c.history[:], raw[97:129])
	if c.head > adapterHistoryMaxSafe || c.offset > adapterHistoryMaxRows || c.issued > adapterHistoryMaxSafe-adapterHistoryTTLMS || c.expires != c.issued+adapterHistoryTTLMS {
		return c, ErrAdapterHistoryCursorInvalid
	}
	if now < c.issued || now > c.expires {
		return c, ErrAdapterHistorySnapshotExpired
	}
	return c, nil
}

func adapterHistoryDigest(scope []byte, head uint64, authority [32]byte, rows []AdapterHistoryRow) ([32]byte, uint64, error) {
	if len(rows) > adapterHistoryMaxRows {
		return [32]byte{}, 0, ErrAdapterHistoryBudgetExceeded
	}
	history := sha256.New()
	history.Write([]byte("neko-adapter-history-v1\x00"))
	history.Write(scope)
	adapterHistoryU64(history, head)
	history.Write(authority[:])
	adapterHistoryU64(history, uint64(len(rows)))
	total := uint64(0)
	for _, row := range rows {
		encoded, err := adapterHistoryRowBytes(row)
		if err != nil {
			return [32]byte{}, 0, err
		}
		total += uint64(len(encoded))
		if total > adapterHistoryMaxBytes {
			return [32]byte{}, 0, ErrAdapterHistoryBudgetExceeded
		}
		history.Write(encoded)
	}
	var digest [32]byte
	copy(digest[:], history.Sum(nil))
	return digest, total, nil
}

func (s *Store) AdapterHistoryPage(id, token string, key [32]byte, now time.Time) (AdapterHistoryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	empty := AdapterHistoryPage{}
	if s.closed || s.poisoned {
		return empty, ErrUnavailable
	}
	a, found := s.adapters[id]
	if !found {
		return empty, ErrNotFound
	}
	c, found := s.channel(a.ChannelID)
	if !found || c.Revision != a.ChannelRevision {
		return empty, ErrAdapterHistoryScopeChanged
	}
	clock := now.UnixMilli()
	if clock < 0 || uint64(clock) > adapterHistoryMaxSafe-adapterHistoryTTLMS {
		return empty, ErrAdapterHistorySnapshotExpired
	}
	scope := adapterHistoryScope(a)
	cursor := adapterHistoryCursor{scope: sha256.Sum256(scope), head: uint64(len(s.events)), issued: uint64(clock), expires: uint64(clock) + adapterHistoryTTLMS}
	if token != "" {
		var err error
		cursor, err = adapterHistoryParseToken(token, scope, key, uint64(clock))
		if err != nil {
			return empty, err
		}
	}
	if cursor.head > uint64(len(s.events)) {
		return empty, ErrAdapterHistoryCursorInvalid
	}
	epoch := uint64(0)
	for _, event := range s.events {
		if event.Type != "work.text" {
			epoch = event.Sequence
		}
	}
	if token != "" && epoch > cursor.head {
		return empty, ErrAdapterHistoryAuthorityChanged
	}
	frozen := map[string]adapterHistoryWork{}
	for _, event := range s.events[:cursor.head] {
		w := event.Work
		if w.Adapter == nil || w.Adapter.AdapterSessionID != id {
			continue
		}
		frozen[w.RequestID] = adapterHistoryWork{work: w, sequence: event.Sequence}
		if len(frozen) > adapterHistoryMaxRows {
			return empty, ErrAdapterHistoryBudgetExceeded
		}
	}
	works := make([]adapterHistoryWork, 0, len(frozen))
	for _, work := range frozen {
		works = append(works, work)
	}
	sort.Slice(works, func(i, j int) bool {
		if works[i].work.CreatedSequence == works[j].work.CreatedSequence {
			return works[i].work.RequestID < works[j].work.RequestID
		}
		return works[i].work.CreatedSequence < works[j].work.CreatedSequence
	})
	authority := sha256.New()
	authority.Write([]byte("neko-adapter-authority-v1\x00"))
	authority.Write(scope)
	adapterHistoryU64(authority, epoch)
	adapterHistoryU64(authority, uint64(len(works)))
	rows := make([]AdapterHistoryRow, 0, len(works))
	for _, frozen := range works {
		w := frozen.work
		current, exists := s.work[w.RequestID]
		allowed := exists && s.contextAllowed(current) && s.contextAllowed(w)
		adapterHistoryString(authority, w.RequestID)
		flag := byte(0)
		if allowed {
			flag = 1
		}
		authority.Write([]byte{flag})
		row := AdapterHistoryRow{ReplyID: "reply-" + w.RequestID, RequestID: w.RequestID, SourceMessageID: w.Adapter.SourceMessageID, BotID: w.BotID, Role: "assistant", Text: w.Reply, Status: w.Status, Sequence: frozen.sequence, Artifacts: append([]Artifact{}, w.Artifacts...), Availability: "available"}
		if !allowed {
			row.Text, row.Artifacts, row.Availability = "", []Artifact{}, "source_unavailable"
		}
		rows = append(rows, row)
	}
	var authorityDigest [32]byte
	copy(authorityDigest[:], authority.Sum(nil))
	if token != "" && cursor.authority != authorityDigest {
		return empty, ErrAdapterHistoryAuthorityChanged
	}
	historyDigest, total, err := adapterHistoryDigest(scope, cursor.head, authorityDigest, rows)
	if err != nil {
		return empty, err
	}
	if token != "" && cursor.history != historyDigest {
		return empty, ErrAdapterHistoryAuthorityChanged
	}
	if cursor.offset > uint64(len(rows)) {
		return empty, ErrAdapterHistoryCursorInvalid
	}
	cursor.authority, cursor.history = authorityDigest, historyDigest
	snapshot := sha256.New()
	snapshot.Write([]byte("neko-adapter-snapshot-v1\x00"))
	snapshot.Write(scope)
	adapterHistoryU64(snapshot, cursor.head)
	snapshot.Write(authorityDigest[:])
	snapshot.Write(historyDigest[:])
	page := AdapterHistoryPage{Version: 1, AdapterSessionID: a.ID, SourceApp: a.SourceApp, SourceSessionID: a.SourceSessionID, SourceThreadID: a.SourceThreadID, ChannelID: a.ChannelID, ChannelRevision: a.ChannelRevision, OwnerUserID: a.OwnerUserID, SpaceID: a.SpaceID, ReturnRoute: a.ReturnRoute, SnapshotID: "h1_" + hex.EncodeToString(snapshot.Sum(nil)), SnapshotCursor: cursor.head, AuthorityDigest: hex.EncodeToString(authorityDigest[:]), HistoryDigest: hex.EncodeToString(historyDigest[:]), Offset: cursor.offset, TotalRows: uint64(len(rows)), TotalBytes: total, Rows: []AdapterHistoryRow{}}
	finish := func(p *AdapterHistoryPage) {
		end := p.Offset + uint64(len(p.Rows))
		next := cursor
		next.offset = end
		value := adapterHistoryToken(next, key)
		p.Complete = end == p.TotalRows
		p.NextCursor, p.ConfirmationCursor = nil, nil
		if p.Complete {
			p.ConfirmationCursor = &value
		} else {
			p.NextCursor = &value
		}
	}
	finish(&page)
	for index := cursor.offset; index < uint64(len(rows)) && len(page.Rows) < adapterHistoryPageRows; index++ {
		candidate := page
		candidate.Rows = append(append([]AdapterHistoryRow{}, page.Rows...), rows[index])
		finish(&candidate)
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return empty, ErrUnavailable
		}
		if len(encoded) > adapterHistoryPageBytes {
			if len(page.Rows) == 0 {
				return empty, ErrAdapterHistoryBudgetExceeded
			}
			break
		}
		page = candidate
	}
	return page, nil
}
