package syncstream

import "errors"

const (
	Version                 = 1
	MaxSafeSequence  uint64 = 1<<53 - 1
	MaxEnvelopeBytes        = 256 << 10
	MaxFrames               = 128
	MaxObjects              = 512
	MaxObjectBytes          = 16 << 10
	MaxStateBytes           = 2 << 20
	ReplayWindow            = 256
	MaxCursorBytes          = 512
)

var (
	ErrInvalid   = errors.New("sync_invalid")
	ErrLimit     = errors.New("sync_limit")
	ErrIntegrity = errors.New("sync_integrity")
	ErrInstance  = errors.New("sync_instance_reset_required")
	ErrScope     = errors.New("sync_scope_mismatch")
	ErrAhead     = errors.New("sync_cursor_ahead")
	ErrGap       = errors.New("sync_sequence_gap")
	ErrConflict  = errors.New("sync_conflict")
	ErrResync    = errors.New("sync_snapshot_required")
)

type Mode string

const (
	Snapshot  Mode = "snapshot"
	Increment Mode = "increment"
)

type Cursor struct {
	Version    int    `json:"version"`
	InstanceID string `json:"instanceId"`
	ScopeID    string `json:"scopeId"`
	Sequence   uint64 `json:"sequence"`
}

type Envelope struct {
	Version     int      `json:"version"`
	InstanceID  string   `json:"instanceId"`
	ScopeID     string   `json:"scopeId"`
	Mode        Mode     `json:"mode"`
	After       uint64   `json:"after"`
	To          uint64   `json:"to"`
	HighWater   uint64   `json:"highWater"`
	ScopeDigest string   `json:"scopeDigest"`
	DataDigest  string   `json:"dataDigest"`
	Snapshot    []Object `json:"snapshot"`
	Frames      []Frame  `json:"frames"`
}

type Frame struct {
	Sequence uint64   `json:"sequence"`
	Updates  []Object `json:"updates"`
}

type Object struct {
	Kind     string   `json:"kind"`
	ID       string   `json:"id"`
	Revision uint64   `json:"revision"`
	Deleted  bool     `json:"deleted"`
	Bot      *Bot     `json:"bot,omitempty"`
	Message  *Message `json:"message,omitempty"`
	File     *File    `json:"file,omitempty"`
}

type Bot struct {
	Name        string `json:"name"`
	Alias       string `json:"alias,omitempty"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

type Message struct {
	BotID     string `json:"botId"`
	ChannelID string `json:"channelId"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	Status    string `json:"status"`
}

type File struct {
	Name           string `json:"name"`
	MIME           string `json:"mime"`
	Size           uint64 `json:"size"`
	SHA256         string `json:"sha256"`
	BotID          string `json:"botId"`
	ChannelID      string `json:"channelId"`
	ProjectID      string `json:"projectId"`
	Status         string `json:"status"`
	Version        uint64 `json:"version"`
	AccessRevision uint64 `json:"accessRevision"`
}
