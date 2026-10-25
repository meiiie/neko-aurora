package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sync"
	"time"
)

var ErrInvalid = errors.New("fixture_identity_invalid")
var ErrUnauthenticated = errors.New("fixture_session_invalid")
var ErrConflict = errors.New("fixture_identity_conflict")

const SafeInteger uint64 = 1<<53 - 1

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

type ActorDevice struct {
	ActorID        string `json:"actorId"`
	DeviceID       string `json:"deviceId"`
	DeviceRevision uint64 `json:"deviceRevision"`
}
type Config struct {
	Version           int              `json:"version"`
	FixtureOnly       bool             `json:"fixtureOnly"`
	AuthorityID       string           `json:"authorityId"`
	SpaceID           string           `json:"spaceId"`
	SessionTTLSeconds int              `json:"sessionTTLSeconds"`
	Actors            []ActorDevice    `json:"actors"`
	Now               func() time.Time `json:"-"`
}
type VerifiedPrincipal struct {
	Version         int    `json:"version"`
	ActorID         string `json:"actorId"`
	SpaceID         string `json:"spaceId"`
	AuthorityID     string `json:"authorityId"`
	DeviceID        string `json:"deviceId"`
	DeviceRevision  uint64 `json:"deviceRevision"`
	SessionID       string `json:"sessionId"`
	SessionRevision uint64 `json:"sessionRevision"`
	IssuerEpoch     uint64 `json:"issuerEpoch"`
	ExpiresAtUnixMs int64  `json:"expiresAtUnixMs"`
}
type SessionStatus struct {
	Version         int    `json:"version"`
	Mode            string `json:"mode"`
	Authenticated   bool   `json:"authenticated"`
	ActorID         string `json:"actorId"`
	SpaceID         string `json:"spaceId"`
	AuthorityID     string `json:"authorityId,omitempty"`
	DeviceID        string `json:"deviceId,omitempty"`
	DeviceRevision  uint64 `json:"deviceRevision,omitempty"`
	SessionRevision uint64 `json:"sessionRevision,omitempty"`
	IssuerEpoch     uint64 `json:"issuerEpoch,omitempty"`
	ExpiresAtUnixMs int64  `json:"expiresAtUnixMs,omitempty"`
	CacheNamespace  string `json:"cacheNamespace"`
	SessionBinding  string `json:"sessionBinding"`
}
type device struct {
	ActorDevice
	active bool
}
type session struct {
	pin      VerifiedPrincipal
	active   bool
	issuedAt int64
}
type bootstrap struct {
	deviceID            string
	revision, epoch     uint64
	issuedAt, expiresAt int64
}
type Authority struct {
	mu        sync.Mutex
	id, space string
	epoch     uint64
	ttl       time.Duration
	now       func() time.Time
	devices   map[string]device
	tickets   map[string]bootstrap
	sessions  map[string]session
	ids       map[string]string
}

func ValidPrincipal(p VerifiedPrincipal) bool {
	return p.Version == 1 && identifier.MatchString(p.ActorID) && identifier.MatchString(p.SpaceID) && identifier.MatchString(p.AuthorityID) && identifier.MatchString(p.DeviceID) && identifier.MatchString(p.SessionID) && p.DeviceRevision > 0 && p.DeviceRevision <= SafeInteger && p.SessionRevision > 0 && p.SessionRevision <= SafeInteger && p.IssuerEpoch > 0 && p.IssuerEpoch <= SafeInteger && p.ExpiresAtUnixMs > 0 && uint64(p.ExpiresAtUnixMs) <= SafeInteger
}
func opaque() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func NewFixture(c Config) (*Authority, error) {
	if c.Version != 1 || !c.FixtureOnly || !identifier.MatchString(c.AuthorityID) || c.SpaceID != "local-space" || c.SessionTTLSeconds < 30 || c.SessionTTLSeconds > 3600 || len(c.Actors) < 1 || len(c.Actors) > 32 {
		return nil, ErrInvalid
	}
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	epoch := binary.LittleEndian.Uint64(entropy[:]) & SafeInteger
	if epoch == 0 {
		epoch = 1
	}
	a := &Authority{id: c.AuthorityID, space: c.SpaceID, epoch: epoch, ttl: time.Duration(c.SessionTTLSeconds) * time.Second, now: c.Now, devices: map[string]device{}, tickets: map[string]bootstrap{}, sessions: map[string]session{}, ids: map[string]string{}}
	if a.now == nil {
		a.now = time.Now
	}
	for _, d := range c.Actors {
		if !identifier.MatchString(d.ActorID) || !identifier.MatchString(d.DeviceID) || d.DeviceRevision == 0 || d.DeviceRevision > SafeInteger {
			return nil, ErrInvalid
		}
		if _, exists := a.devices[d.DeviceID]; exists {
			return nil, ErrInvalid
		}
		a.devices[d.DeviceID] = device{d, true}
	}
	return a, nil
}
func (a *Authority) AuthorityID() string { return a.id }
func (a *Authority) SpaceID() string     { return a.space }
func (a *Authority) IssuerEpoch() uint64 { a.mu.Lock(); defer a.mu.Unlock(); return a.epoch }
func (a *Authority) BootstrapTicket(actorID, deviceID string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.retireLocked()
	d, ok := a.devices[deviceID]
	if !ok || !d.active || d.ActorID != actorID || len(a.tickets) >= 128 {
		return "", ErrInvalid
	}
	return a.ticketLocked(d)
}
func (a *Authority) ticketLocked(d device) (string, error) {
	ticket, err := opaque()
	if err != nil {
		return "", err
	}
	now := a.now().UnixMilli()
	ttl := a.ttl
	if ttl > 5*time.Minute {
		ttl = 5 * time.Minute
	}
	a.tickets[ticket] = bootstrap{d.DeviceID, d.DeviceRevision, a.epoch, now, now + ttl.Milliseconds()}
	return ticket, nil
}
func (a *Authority) retireLocked() {
	now := a.now().UnixMilli()
	for ticket, t := range a.tickets {
		if now >= t.expiresAt {
			delete(a.tickets, ticket)
		}
	}
	for token, s := range a.sessions {
		if !s.active || now >= s.pin.ExpiresAtUnixMs {
			delete(a.ids, s.pin.SessionID)
			delete(a.sessions, token)
		}
	}
}
func (a *Authority) Exchange(ticket string) (string, VerifiedPrincipal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.retireLocked()
	t, ok := a.tickets[ticket]
	d, found := a.devices[t.deviceID]
	now := a.now().UnixMilli()
	if !ok || !found || !d.active || d.DeviceRevision != t.revision || a.epoch != t.epoch || now < t.issuedAt || now >= t.expiresAt || len(a.sessions) >= 512 {
		return "", VerifiedPrincipal{}, ErrUnauthenticated
	}
	token, err := opaque()
	if err != nil {
		return "", VerifiedPrincipal{}, err
	}
	sid, err := opaque()
	if err != nil {
		return "", VerifiedPrincipal{}, err
	}
	p := VerifiedPrincipal{1, d.ActorID, a.space, a.id, d.DeviceID, d.DeviceRevision, "fixture-session-" + sid[:32], 1, a.epoch, a.now().Add(a.ttl).UnixMilli()}
	if !ValidPrincipal(p) {
		return "", VerifiedPrincipal{}, ErrInvalid
	}
	delete(a.tickets, ticket)
	a.sessions[token] = session{p, true, now}
	a.ids[p.SessionID] = token
	return token, p, nil
}
func (a *Authority) validLocked(p VerifiedPrincipal) bool {
	token, ok := a.ids[p.SessionID]
	s, found := a.sessions[token]
	d, deviceFound := a.devices[p.DeviceID]
	now := a.now().UnixMilli()
	return ok && found && deviceFound && s.active && d.active && s.pin == p && p.AuthorityID == a.id && p.SpaceID == a.space && p.IssuerEpoch == a.epoch && p.DeviceRevision == d.DeviceRevision && p.ActorID == d.ActorID && now >= s.issuedAt && now < p.ExpiresAtUnixMs
}
func (a *Authority) Resolve(token string) (VerifiedPrincipal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[token]
	if !ok || !a.validLocked(s.pin) {
		return VerifiedPrincipal{}, ErrUnauthenticated
	}
	return s.pin, nil
}
func (a *Authority) Valid(p VerifiedPrincipal) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.validLocked(p)
}
func (a *Authority) Hold(p VerifiedPrincipal) (func(), error) {
	if a == nil {
		return nil, ErrUnauthenticated
	}
	a.mu.Lock()
	if !a.validLocked(p) {
		a.mu.Unlock()
		return nil, ErrUnauthenticated
	}
	return a.mu.Unlock, nil
}

func (a *Authority) HoldAll(principals []VerifiedPrincipal) (func(), error) {
	if a == nil || len(principals) == 0 {
		return nil, ErrUnauthenticated
	}
	a.mu.Lock()
	for _, p := range principals {
		if !a.validLocked(p) {
			a.mu.Unlock()
			return nil, ErrUnauthenticated
		}
	}
	return a.mu.Unlock, nil
}
func Binding(p VerifiedPrincipal) string {
	raw, _ := json.Marshal(p)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func ActorNamespace(p VerifiedPrincipal) string {
	raw, _ := json.Marshal([]string{p.AuthorityID, p.SpaceID, p.ActorID})
	h := sha256.Sum256(raw)
	return "fixture-" + hex.EncodeToString(h[:])
}
func Status(p VerifiedPrincipal) SessionStatus {
	binding := Binding(p)
	return SessionStatus{Version: 1, Mode: "fixture", Authenticated: true, ActorID: p.ActorID, SpaceID: p.SpaceID, AuthorityID: p.AuthorityID, DeviceID: p.DeviceID, DeviceRevision: p.DeviceRevision, SessionRevision: p.SessionRevision, IssuerEpoch: p.IssuerEpoch, ExpiresAtUnixMs: p.ExpiresAtUnixMs, CacheNamespace: ActorNamespace(p), SessionBinding: binding}
}
func (a *Authority) RevokeSession(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	token, ok := a.ids[id]
	if !ok {
		return ErrInvalid
	}
	s := a.sessions[token]
	s.active = false
	a.sessions[token] = s
	return nil
}

type DeviceControlResult struct {
	DeviceID       string `json:"deviceId"`
	DeviceRevision uint64 `json:"deviceRevision"`
	Active         bool   `json:"active"`
	Ticket         string `json:"ticket,omitempty"`
}

func (a *Authority) ControlDevice(id, action string, expected uint64) (DeviceControlResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.controlLocked(id, action, expected)
}
func (a *Authority) ControlDeviceFor(p VerifiedPrincipal, id, action string, expected uint64) (DeviceControlResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.validLocked(p) || p.ActorID != "local-owner" {
		return DeviceControlResult{}, ErrUnauthenticated
	}
	return a.controlLocked(id, action, expected)
}
func (a *Authority) KnownActor(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, d := range a.devices {
		if d.ActorID == id {
			return true
		}
	}
	return false
}
func (a *Authority) controlLocked(id, action string, expected uint64) (DeviceControlResult, error) {
	d, ok := a.devices[id]
	if !ok || d.DeviceRevision != expected || expected >= SafeInteger {
		return DeviceControlResult{}, ErrConflict
	}
	if action != "revoke" && action != "rotate" {
		return DeviceControlResult{}, ErrInvalid
	}
	d.DeviceRevision++
	d.active = action == "rotate"
	a.devices[id] = d
	for token, s := range a.sessions {
		if s.pin.DeviceID == id {
			s.active = false
			a.sessions[token] = s
		}
	}
	for ticket, t := range a.tickets {
		if t.deviceID == id {
			delete(a.tickets, ticket)
		}
	}
	out := DeviceControlResult{id, d.DeviceRevision, d.active, ""}
	if d.active {
		ticket, err := a.ticketLocked(d)
		if err != nil {
			return DeviceControlResult{}, err
		}
		out.Ticket = ticket
	}
	return out, nil
}
