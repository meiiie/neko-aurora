// Package duet is the server side of the /duet/ mobile surface: agents,
// messages, background tasks, an activity log, approvals, the morning brief
// and notifications. State is one JSON file inside the control-plane state dir.
package duet

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Avatar struct {
	Shell int    `json:"shell"`
	Hat   string `json:"hat"`
	Eyes  string `json:"eyes"`
	Extra string `json:"extra"`
}

type Agent struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Desc     string    `json:"desc"`
	Tag      string    `json:"tag,omitempty"`
	Avatar   Avatar    `json:"av"`
	Status   string    `json:"status"` // idle|working|waiting|done|error
	Snip     string    `json:"snip"`
	Pinned   bool      `json:"pin"`
	Archived bool      `json:"archived,omitempty"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"t"`
}

type Approval struct {
	Title     string     `json:"title"`
	Kind      string     `json:"kind"`
	Body      string     `json:"body"`
	State     string     `json:"state"` // pending|approved|declined
	DecidedAt *time.Time `json:"decidedAt,omitempty"`
}

type Message struct {
	ID       string    `json:"id"`
	AgentID  string    `json:"agentId"`
	Role     string    `json:"role"` // user|agent|system
	Text     string    `json:"text"`
	Approval *Approval `json:"approval,omitempty"`
	Created  time.Time `json:"t"`
}

type Activity struct {
	ID      string    `json:"id"`
	AgentID string    `json:"agentId,omitempty"`
	Text    string    `json:"text"`
	T       time.Time `json:"t"`
}

type Brief struct {
	Text      string    `json:"text"`
	Items     []string  `json:"items"`
	Generated time.Time `json:"generated"`
	Source    string    `json:"source"` // ai|fallback
}

type PushSub struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type data struct {
	Agents   []*Agent   `json:"agents"`
	Messages []*Message `json:"messages"`
	Activity []Activity `json:"activity"`
	Brief    *Brief     `json:"brief,omitempty"`
	Push     []PushSub  `json:"push,omitempty"`
}

var (
	ErrNotFound = errors.New("not_found")
	ErrInvalid  = errors.New("invalid")
	ErrConflict = errors.New("conflict")
)

type Store struct {
	mu   sync.Mutex
	path string
	d    data
}

func newID(p string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return p + hex.EncodeToString(b[:])
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "duet.json")}
	b, err := os.ReadFile(s.path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &s.d); err != nil {
			return nil, err
		}
	case os.IsNotExist(err):
		s.seed()
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	// a crash mid-task leaves "working"; surface it honestly
	for _, a := range s.d.Agents {
		if a.Status == "working" {
			a.Status, a.Snip = "error", "Tác vụ bị gián đoạn khi máy chủ khởi động lại. Nhắn lại để tiếp tục."
		}
	}
	return s, nil
}

func (s *Store) seed() {
	now := time.Now()
	mk := func(name, desc, tag string, av Avatar) *Agent {
		return &Agent{ID: newID("ag_"), Name: name, Desc: desc, Tag: tag, Avatar: av, Status: "idle", Snip: "Sẵn sàng nhận việc.", Created: now, Updated: now}
	}
	s.d.Agents = []*Agent{
		mk("Morning Brief", "Tóm tắt mỗi sáng những gì cần tôi.", "Mỗi sáng", Avatar{0, "none", "round", "none"}),
		mk("Posting Buddy", "Soạn bài theo giọng tôi. Không bao giờ tự đăng.", "", Avatar{6, "cap", "shades", "none"}),
		mk("Inbox Zero", "Phân loại thư, soạn trả lời để tôi duyệt.", "", Avatar{3, "beanie", "none", "antenna"}),
	}
	s.d.Activity = []Activity{{ID: newID("ac_"), Text: "Khởi tạo Neko Duet với 3 agent mẫu", T: now}}
}

func (s *Store) saveLocked() error {
	if len(s.d.Activity) > 500 {
		s.d.Activity = s.d.Activity[len(s.d.Activity)-500:]
	}
	b, err := json.Marshal(s.d)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func cp[T any](v *T) *T { c := *v; return &c }

func (s *Store) Agents() []*Agent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []*Agent{}
	for _, a := range s.d.Agents {
		if !a.Archived {
			out = append(out, cp(a))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out
}

func (s *Store) agentLocked(id string) *Agent {
	for _, a := range s.d.Agents {
		if a.ID == id && !a.Archived {
			return a
		}
	}
	return nil
}

func (s *Store) Agent(id string) (*Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.agentLocked(id)
	if a == nil {
		return nil, ErrNotFound
	}
	return cp(a), nil
}

func clean(v string, n int) string {
	v = strings.TrimSpace(strings.ToValidUTF8(v, ""))
	if r := []rune(v); len(r) > n {
		v = string(r[:n])
	}
	return v
}

var okHat = map[string]bool{"none": true, "beanie": true, "cap": true, "top": true, "crown": true}
var okEyes = map[string]bool{"none": true, "round": true, "shades": true, "visor": true, "mono": true}
var okExtra = map[string]bool{"none": true, "bow": true, "flower": true, "antenna": true, "scarf": true}

func validAvatar(a Avatar) bool {
	return a.Shell >= 0 && a.Shell < 10 && okHat[a.Hat] && okEyes[a.Eyes] && okExtra[a.Extra]
}

func (s *Store) CreateAgent(name, desc string, av Avatar) (*Agent, error) {
	name, desc = clean(name, 28), clean(desc, 160)
	if name == "" || !validAvatar(av) {
		return nil, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	a := &Agent{ID: newID("ag_"), Name: name, Desc: desc, Avatar: av, Status: "idle", Snip: "Sẵn sàng nhận việc.", Created: now, Updated: now}
	s.d.Agents = append(s.d.Agents, a)
	s.d.Messages = append(s.d.Messages, &Message{ID: newID("m_"), AgentID: a.ID, Role: "agent", Text: "Chào, mình là " + name + ". " + desc + " Nhắn mình việc cần làm nhé.", Created: now})
	s.d.Activity = append(s.d.Activity, Activity{ID: newID("ac_"), AgentID: a.ID, Text: "Đã tạo agent " + name, T: now})
	return cp(a), s.saveLocked()
}

type AgentPatch struct {
	Name     *string `json:"name"`
	Desc     *string `json:"desc"`
	Avatar   *Avatar `json:"av"`
	Pinned   *bool   `json:"pin"`
	Archived *bool   `json:"archived"`
}

func (s *Store) PatchAgent(id string, p AgentPatch) (*Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.agentLocked(id)
	if a == nil {
		return nil, ErrNotFound
	}
	if p.Name != nil {
		if n := clean(*p.Name, 28); n != "" {
			a.Name = n
		} else {
			return nil, ErrInvalid
		}
	}
	if p.Desc != nil {
		a.Desc = clean(*p.Desc, 160)
	}
	if p.Avatar != nil {
		if !validAvatar(*p.Avatar) {
			return nil, ErrInvalid
		}
		a.Avatar = *p.Avatar
	}
	if p.Pinned != nil {
		a.Pinned = *p.Pinned
	}
	if p.Archived != nil && *p.Archived {
		a.Archived = true
		s.d.Activity = append(s.d.Activity, Activity{ID: newID("ac_"), AgentID: a.ID, Text: "Đã lưu trữ " + a.Name, T: time.Now()})
	}
	return cp(a), s.saveLocked()
}

func (s *Store) Messages(agentID string, limit int) ([]*Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agentLocked(agentID) == nil {
		return nil, ErrNotFound
	}
	out := []*Message{}
	for _, m := range s.d.Messages {
		if m.AgentID == agentID {
			c := cp(m)
			if m.Approval != nil {
				c.Approval = cp(m.Approval)
			}
			out = append(out, c)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (s *Store) AddMessage(m *Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agentLocked(m.AgentID) == nil {
		return ErrNotFound
	}
	if m.ID == "" {
		m.ID = newID("m_")
	}
	if m.Created.IsZero() {
		m.Created = time.Now()
	}
	s.d.Messages = append(s.d.Messages, m)
	return s.saveLocked()
}

func (s *Store) SetStatus(id, status, snip string) (*Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.agentLocked(id)
	if a == nil {
		return nil, ErrNotFound
	}
	a.Status, a.Snip, a.Updated = status, clean(snip, 140), time.Now()
	return cp(a), s.saveLocked()
}

func (s *Store) Log(agentID, text string) Activity {
	s.mu.Lock()
	defer s.mu.Unlock()
	ac := Activity{ID: newID("ac_"), AgentID: agentID, Text: clean(text, 200), T: time.Now()}
	s.d.Activity = append(s.d.Activity, ac)
	_ = s.saveLocked()
	return ac
}

func (s *Store) ActivityLog(limit int) []Activity {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.d.Activity)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]Activity, limit)
	for i := 0; i < limit; i++ {
		out[i] = s.d.Activity[n-1-i]
	}
	return out
}

// Decide records an approval decision exactly once. Duplicate or racing
// decisions return ErrConflict so double taps cannot flip a decision.
func (s *Store) Decide(msgID, decision string) (*Message, error) {
	if decision != "approved" && decision != "declined" {
		return nil, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.d.Messages {
		if m.ID == msgID && m.Approval != nil {
			if s.agentLocked(m.AgentID) == nil {
				return nil, ErrNotFound
			}
			if m.Approval.State != "pending" {
				return nil, ErrConflict
			}
			now := time.Now()
			m.Approval.State, m.Approval.DecidedAt = decision, &now
			c := cp(m)
			c.Approval = cp(m.Approval)
			return c, s.saveLocked()
		}
	}
	return nil, ErrNotFound
}

func (s *Store) Pending() []*Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []*Message{}
	for _, m := range s.d.Messages {
		if m.Approval != nil && m.Approval.State == "pending" && s.agentLocked(m.AgentID) != nil {
			c := cp(m)
			c.Approval = cp(m.Approval)
			out = append(out, c)
		}
	}
	return out
}

func (s *Store) Brief() *Brief {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.d.Brief == nil {
		return nil
	}
	return cp(s.d.Brief)
}

func (s *Store) SetBrief(b *Brief) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d.Brief = b
	return s.saveLocked()
}

func (s *Store) AddPush(p PushSub) error {
	if !strings.HasPrefix(p.Endpoint, "https://") || p.Keys.P256dh == "" || p.Keys.Auth == "" {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.d.Push {
		if e.Endpoint == p.Endpoint {
			return nil
		}
	}
	s.d.Push = append(s.d.Push, p)
	return s.saveLocked()
}

func (s *Store) PushSubs() []PushSub {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]PushSub(nil), s.d.Push...)
}

func (s *Store) DropPush(endpoint string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.d.Push[:0]
	for _, e := range s.d.Push {
		if e.Endpoint != endpoint {
			out = append(out, e)
		}
	}
	s.d.Push = out
	_ = s.saveLocked()
}
