package retentionplan

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxItems       = 500
	MaxTextBytes   = 1 << 20
	MaxReasons     = 32
	MaxReasonBytes = 128
)

type Item struct {
	ID               string
	CreatedAt        string
	Status           string
	ProtectedReasons []string
	TextBytes        int
}

type Candidate struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	Bytes     int    `json:"bytes"`
}

type Protection struct {
	ID      string   `json:"id"`
	Reasons []string `json:"reasons"`
}

type Preview struct {
	Version    int          `json:"version"`
	Days       int          `json:"days"`
	AsOf       string       `json:"asOf"`
	Enabled    bool         `json:"enabled"`
	Candidates []Candidate  `json:"candidates"`
	Protected  []Protection `json:"protected"`
	TotalBytes int          `json:"totalBytes"`
}

type ErrorCode string

const (
	InvalidDays      ErrorCode = "invalid_days"
	InvalidClock     ErrorCode = "invalid_clock"
	ItemLimit        ErrorCode = "item_limit"
	InvalidID        ErrorCode = "invalid_id"
	DuplicateID      ErrorCode = "duplicate_id"
	InvalidStatus    ErrorCode = "invalid_status"
	InvalidTimestamp ErrorCode = "invalid_created_at"
	InvalidBytes     ErrorCode = "invalid_text_bytes"
	BytesLimit       ErrorCode = "text_bytes_limit"
	InvalidReason    ErrorCode = "invalid_protection_reason"
	ReasonLimit      ErrorCode = "protection_reason_limit"
)

var (
	ErrInvalidInput = errors.New("retention_invalid_input")
	rfc3339         = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,9})?(?:Z|[+-][0-9]{2}:[0-9]{2})$`)
)

type ValidationError struct {
	Code ErrorCode
}

func (e *ValidationError) Error() string { return "retention_" + string(e.Code) }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

func Plan(items []Item, days int, now time.Time) (Preview, error) {
	fail := func(code ErrorCode) (Preview, error) {
		return Preview{}, &ValidationError{Code: code}
	}
	if days < 0 || days > 3650 {
		return fail(InvalidDays)
	}
	asOf := now.UTC()
	if now.IsZero() || asOf.Year() < 1 || asOf.Year() > 9999 {
		return fail(InvalidClock)
	}
	if len(items) > MaxItems {
		return fail(ItemLimit)
	}
	seen := make(map[string]bool, len(items))
	created := make([]time.Time, len(items))
	totalInputBytes := 0
	for i, item := range items {
		if !token(item.ID, 128) {
			return fail(InvalidID)
		}
		if seen[item.ID] {
			return fail(DuplicateID)
		}
		seen[item.ID] = true
		if !token(item.Status, 64) {
			return fail(InvalidStatus)
		}
		at, err := parseDate(item.CreatedAt)
		if err != nil {
			return fail(InvalidTimestamp)
		}
		created[i] = at
		if item.TextBytes < 0 {
			return fail(InvalidBytes)
		}
		if item.TextBytes > MaxTextBytes-totalInputBytes {
			return fail(BytesLimit)
		}
		totalInputBytes += item.TextBytes
		if len(item.ProtectedReasons) > MaxReasons {
			return fail(ReasonLimit)
		}
		for _, reason := range item.ProtectedReasons {
			if !validReason(reason) {
				return fail(InvalidReason)
			}
		}
	}
	preview := Preview{Version: 1, Days: days, AsOf: asOf.Format(time.RFC3339Nano), Enabled: false, Candidates: []Candidate{}, Protected: []Protection{}}
	cutoff := asOf.Add(-time.Duration(days) * 24 * time.Hour)
	for i, item := range items {
		reasons := make(map[string]bool, len(item.ProtectedReasons)+3)
		for _, reason := range item.ProtectedReasons {
			reasons[reason] = true
		}
		if item.Status != "completed" && item.Status != "failed" && item.Status != "cancelled" {
			reasons["nonterminal_status"] = true
		}
		if days == 0 {
			reasons["retention_disabled"] = true
		}
		if created[i].After(asOf) {
			reasons["future_created_at"] = true
		} else if days != 0 && !created[i].Before(cutoff) {
			reasons["within_retention_window"] = true
		}
		if len(reasons) == 0 {
			preview.Candidates = append(preview.Candidates, Candidate{ID: item.ID, CreatedAt: created[i].UTC().Format(time.RFC3339Nano), Bytes: item.TextBytes})
			preview.TotalBytes += item.TextBytes
			continue
		}
		protection := Protection{ID: item.ID, Reasons: make([]string, 0, len(reasons))}
		for reason := range reasons {
			protection.Reasons = append(protection.Reasons, reason)
		}
		sort.Strings(protection.Reasons)
		preview.Protected = append(preview.Protected, protection)
	}
	sort.Slice(preview.Candidates, func(i, j int) bool { return preview.Candidates[i].ID < preview.Candidates[j].ID })
	sort.Slice(preview.Protected, func(i, j int) bool { return preview.Protected[i].ID < preview.Protected[j].ID })
	return preview, nil
}

func token(s string, max int) bool {
	if len(s) == 0 || len(s) > max {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == ':') {
			return false
		}
	}
	return true
}

func validReason(s string) bool {
	if len(s) == 0 || len(s) > MaxReasonBytes || !utf8.ValidString(s) || strings.TrimSpace(s) != s {
		return false
	}
	for _, c := range s {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}

func parseDate(s string) (time.Time, error) {
	if len(s) > 64 || !rfc3339.MatchString(s) {
		return time.Time{}, &ValidationError{Code: InvalidTimestamp}
	}
	if s[len(s)-1] != 'Z' {
		offset := s[len(s)-6:]
		hour := (offset[1]-'0')*10 + offset[2] - '0'
		minute := (offset[4]-'0')*10 + offset[5] - '0'
		if hour > 23 || minute > 59 {
			return time.Time{}, &ValidationError{Code: InvalidTimestamp}
		}
	}
	at, err := time.Parse(time.RFC3339Nano, s)
	if err != nil || at.Year() < 1 || at.Year() > 9999 || at.UTC().Year() < 1 || at.UTC().Year() > 9999 {
		return time.Time{}, &ValidationError{Code: InvalidTimestamp}
	}
	return at, nil
}
