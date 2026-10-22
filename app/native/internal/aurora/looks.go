package aurora

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"nekoaurora.local/native/internal/ledger"
)

// Looks is the Aurora view of per-bot presentation (avatar, pin, tag) and
// the owner's Web Push subscriptions. Both are journaled ledger records
// (aurora.look / aurora.push); there is no side file.
type Avatar = ledger.AuroraAvatar

type PushSub struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type botPrefs struct {
	Avatar *Avatar
	Pinned bool
	Tag    string
}

type Prefs struct{ L *ledger.Store }

func (p *Prefs) Bot(id string) botPrefs {
	l, _ := p.L.AuroraLookOf(id)
	return botPrefs{Avatar: l.Avatar, Pinned: l.Pinned, Tag: l.Tag}
}

func (p *Prefs) SetBot(id string, fn func(*botPrefs)) error {
	b := p.Bot(id)
	fn(&b)
	return p.L.SetAuroraLook(ledger.AuroraLook{BotID: id, Avatar: b.Avatar, Pinned: b.Pinned, Tag: b.Tag})
}

func (p *Prefs) AddPush(s PushSub) error {
	if !strings.HasPrefix(s.Endpoint, "https://") || s.Keys.P256dh == "" || s.Keys.Auth == "" {
		return ErrInvalid
	}
	if err := p.L.AddAuroraPush(ledger.AuroraPush{Endpoint: s.Endpoint, P256dh: s.Keys.P256dh, Auth: s.Keys.Auth}); err != nil {
		if err == ledger.ErrInvalid {
			return ErrInvalid
		}
		return err
	}
	return nil
}

func (p *Prefs) PushSubs() []PushSub {
	out := []PushSub{}
	for _, x := range p.L.AuroraPushSubs() {
		var s PushSub
		s.Endpoint, s.Keys.P256dh, s.Keys.Auth = x.Endpoint, x.P256dh, x.Auth
		out = append(out, s)
	}
	return out
}

func (p *Prefs) DropPush(endpoint string) { _ = p.L.RemoveAuroraPush(endpoint) }

func validAvatar(a Avatar) bool { return ledger.ValidAuroraAvatar(a) }

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

// Env reads AURORA_<name>.
func Env(name string) string { return os.Getenv("AURORA_" + name) }

// importPrefsFile moves a pre-ledger prefs.json (avatar/pin/tag, push) into
// the journal once, then renames it so it is never read again.
func importPrefsFile(p *Prefs, dir string) error {
	path := filepath.Join(dir, "prefs.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	var d struct {
		Bots map[string]struct {
			Avatar *Avatar `json:"av"`
			Pinned bool    `json:"pin"`
			Tag    string  `json:"tag"`
		} `json:"bots"`
		Push []PushSub `json:"push"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	for id, x := range d.Bots {
		if x.Avatar != nil && !validAvatar(*x.Avatar) {
			x.Avatar = nil
		}
		_ = p.L.SetAuroraLook(ledger.AuroraLook{BotID: id, Avatar: x.Avatar, Pinned: x.Pinned, Tag: clean(x.Tag, 40)})
	}
	for _, s := range d.Push {
		_ = p.AddPush(s)
	}
	return os.Rename(path, path+".imported")
}
