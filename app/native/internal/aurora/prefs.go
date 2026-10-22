package aurora

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Prefs holds mobile presentation preferences only (avatar, pin, tag) and
// this device's Web Push subscriptions. Bots, messages, tasks, plans and
// approvals live in the control-plane ledger; nothing here is a second model.
type Avatar struct {
	Shell int    `json:"shell"`
	Hat   string `json:"hat"`
	Eyes  string `json:"eyes"`
	Extra string `json:"extra"`
}

type PushSub struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type botPrefs struct {
	Avatar *Avatar `json:"av,omitempty"`
	Pinned bool    `json:"pin,omitempty"`
	Tag    string  `json:"tag,omitempty"`
}

type prefsData struct {
	Bots map[string]botPrefs `json:"bots"`
	Push []PushSub           `json:"push,omitempty"`
}

type Prefs struct {
	mu   sync.Mutex
	path string
	d    prefsData
}

func OpenPrefs(dir string) (*Prefs, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	p := &Prefs{path: filepath.Join(dir, "prefs.json"), d: prefsData{Bots: map[string]botPrefs{}}}
	b, err := os.ReadFile(p.path)
	if err == nil {
		if err := json.Unmarshal(b, &p.d); err != nil {
			return nil, err
		}
		if p.d.Bots == nil {
			p.d.Bots = map[string]botPrefs{}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return p, nil
}

func (p *Prefs) saveLocked() error {
	b, err := json.Marshal(p.d)
	if err != nil {
		return err
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}

func (p *Prefs) Bot(id string) botPrefs {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.d.Bots[id]
}

func (p *Prefs) SetBot(id string, fn func(*botPrefs)) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	b := p.d.Bots[id]
	fn(&b)
	p.d.Bots[id] = b
	return p.saveLocked()
}

func (p *Prefs) AddPush(s PushSub) error {
	if !strings.HasPrefix(s.Endpoint, "https://") || s.Keys.P256dh == "" || s.Keys.Auth == "" {
		return ErrInvalid
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.d.Push {
		if e.Endpoint == s.Endpoint {
			return nil
		}
	}
	p.d.Push = append(p.d.Push, s)
	return p.saveLocked()
}

func (p *Prefs) PushSubs() []PushSub {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]PushSub(nil), p.d.Push...)
}

func (p *Prefs) DropPush(endpoint string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.d.Push[:0]
	for _, e := range p.d.Push {
		if e.Endpoint != endpoint {
			out = append(out, e)
		}
	}
	p.d.Push = out
	_ = p.saveLocked()
}

var okHat = map[string]bool{"none": true, "beanie": true, "cap": true, "top": true, "crown": true}
var okEyes = map[string]bool{"none": true, "round": true, "shades": true, "visor": true, "mono": true}
var okExtra = map[string]bool{"none": true, "bow": true, "flower": true, "antenna": true, "scarf": true}

func validAvatar(a Avatar) bool {
	return a.Shell >= 0 && a.Shell < 10 && okHat[a.Hat] && okEyes[a.Eyes] && okExtra[a.Extra]
}

// defaultAvatar gives bots created on the desktop a stable look.
func defaultAvatar(id string) Avatar {
	h := 0
	for _, c := range id {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	hats := []string{"none", "beanie", "cap", "top", "crown"}
	eyes := []string{"round", "none", "shades", "visor", "mono"}
	return Avatar{Shell: h % 10, Hat: hats[(h/10)%5], Eyes: eyes[(h/50)%5], Extra: "none"}
}

// Env reads AURORA_<name>, falling back to the pre-rename DUET_<name>.
func Env(name string) string {
	if v := os.Getenv("AURORA_" + name); v != "" {
		return v
	}
	return os.Getenv("DUET_" + name)
}
