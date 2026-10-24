package vault

// Typed Vault items (Grok-style Vault): payment cards, logins and personal
// details. Each item is one sealed entry (name VI_<id>, kind "vault:<type>")
// whose plaintext is the JSON field map. Only Item metadata (label, last4,
// site, a username hint) ever leaves the package through Items; Fields is
// for the owner's own reveal and for server-side injection after approval.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrType = errors.New("invalid_vault_item")

// ItemFields lists the accepted fields per type, in display order.
var ItemFields = map[string][]string{
	"card":     {"number", "name", "exp", "cvc", "zip"},
	"login":    {"site", "username", "password", "totp", "notes"},
	"identity": {"name", "email", "phone", "address", "city", "postal", "country"},
}

// plainField values are metadata, never masked (the site of a login).
var plainField = map[string]bool{"site": true}

const itemPrefix = "VI_"

// Item is the public description of a Vault item: no secret values.
type Item struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Label   string            `json:"label"`
	Hint    map[string]string `json:"hint"`
	Filled  []string          `json:"filled"`
	Updated string            `json:"updated"`
}

var idRe = regexp.MustCompile(`^[a-f0-9]{8,32}$`)

func ValidItemID(id string) bool { return idRe.MatchString(id) }

func itemName(id string) string { return itemPrefix + strings.ToUpper(id) }

func isItemKind(k string) bool { return strings.HasPrefix(k, "vault:") }

// SaveItem creates (id == "") or updates an item. On update an empty field
// keeps its stored value, so the owner never retypes a secret to edit a label.
func (v *Vault) SaveItem(id, typ, label string, fields map[string]string) (Item, error) {
	allowed, ok := ItemFields[typ]
	if v == nil || !ok {
		return Item{}, ErrType
	}
	label = strings.TrimSpace(label)
	if utf8.RuneCountInString(label) > 80 {
		return Item{}, ErrType
	}
	old := map[string]string{}
	if id == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		id = hex.EncodeToString(b)
	} else {
		if !ValidItemID(id) {
			return Item{}, ErrMissing
		}
		m, ok := v.Meta(itemName(id))
		if !ok || m.Kind != "vault:"+typ {
			return Item{}, ErrMissing
		}
		old, _ = v.Fields(id)
	}
	out := map[string]string{}
	for _, k := range allowed {
		val := strings.TrimSpace(fields[k])
		if k == "number" {
			val = strings.Join(strings.FieldsFunc(val, func(r rune) bool { return r == ' ' || r == '-' }), "")
		}
		if k == "totp" {
			val = strings.ToUpper(strings.ReplaceAll(val, " ", ""))
		}
		if val == "" {
			val = old[k]
		}
		if len(val) > 512 || strings.ContainsAny(val, "\x00") {
			return Item{}, ErrValue
		}
		if val != "" {
			out[k] = val
		}
	}
	for k := range fields {
		if !contains(allowed, k) {
			return Item{}, ErrType
		}
	}
	switch {
	case len(out) == 0:
		return Item{}, ErrValue
	case typ == "card" && !luhn(out["number"]):
		return Item{}, ErrValue
	case typ == "login" && out["password"] == "":
		return Item{}, ErrValue
	case typ == "login" && out["totp"] != "":
		if _, err := TOTP(out["totp"], time.Now()); err != nil {
			return Item{}, ErrValue
		}
	}
	if label == "" {
		label = defaultLabel(typ, out)
	}
	hint, _ := json.Marshal(hintOf(typ, out))
	b, _ := json.Marshal(out)
	m, err := v.Set(Meta{Name: itemName(id), Label: label, Kind: "vault:" + typ, Hint: string(hint)}, string(b))
	if err != nil {
		return Item{}, err
	}
	return itemOf(m), nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func defaultLabel(typ string, f map[string]string) string {
	switch typ {
	case "card":
		return Brand(f["number"]) + " •••• " + last4(f["number"])
	case "login":
		return f["site"]
	}
	return f["name"]
}

func last4(n string) string {
	if len(n) < 4 {
		return n
	}
	return n[len(n)-4:]
}

// Brand names a card network from its IIN (display only).
func Brand(n string) string {
	switch {
	case strings.HasPrefix(n, "4"):
		return "Visa"
	case len(n) >= 2 && n[0] == '5' && n[1] >= '1' && n[1] <= '5', strings.HasPrefix(n, "2"):
		return "Mastercard"
	case strings.HasPrefix(n, "34"), strings.HasPrefix(n, "37"):
		return "Amex"
	case strings.HasPrefix(n, "35"):
		return "JCB"
	}
	return "Thẻ"
}

func luhn(n string) bool {
	if len(n) < 12 || len(n) > 19 {
		return false
	}
	sum, dbl := 0, false
	for i := len(n) - 1; i >= 0; i-- {
		c := n[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if dbl {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum, dbl = sum+d, !dbl
	}
	return sum%10 == 0
}

// MaskUser shows a username hint: first 2 characters, then the domain.
func MaskUser(u string) string {
	at := strings.IndexByte(u, '@')
	head, tail := u, ""
	if at >= 0 {
		head, tail = u[:at], u[at:]
	}
	r := []rune(head)
	if len(r) > 2 {
		r = r[:2]
	}
	return string(r) + "•••" + tail
}

func hintOf(typ string, f map[string]string) map[string]string {
	h := map[string]string{}
	switch typ {
	case "card":
		h["brand"], h["last4"] = Brand(f["number"]), last4(f["number"])
	case "login":
		h["site"] = f["site"]
		if f["username"] != "" {
			h["user"] = MaskUser(f["username"])
		}
		if f["totp"] != "" {
			h["totp"] = "1"
		}
	case "identity":
		if f["name"] != "" {
			h["name"] = MaskUser(f["name"])
		}
	}
	return h
}

func itemOf(m Meta) Item {
	it := Item{ID: strings.ToLower(strings.TrimPrefix(m.Name, itemPrefix)), Type: strings.TrimPrefix(m.Kind, "vault:"), Label: m.Label, Hint: map[string]string{}, Updated: m.Updated}
	var h map[string]string
	if json.Unmarshal([]byte(m.Hint), &h) == nil && h != nil {
		it.Hint = h
	}
	return it
}

// Items lists every Vault item's metadata (newest first).
func (v *Vault) Items() []Item {
	out := []Item{}
	if v == nil {
		return out
	}
	v.mu.RLock()
	for _, e := range v.items {
		if isItemKind(e.Kind) {
			it := itemOf(e.Meta)
			var f map[string]string
			_ = json.Unmarshal([]byte(v.cache[e.Name]), &f)
			for _, k := range ItemFields[it.Type] {
				if f[k] != "" {
					it.Filled = append(it.Filled, k)
				}
			}
			out = append(out, it)
		}
	}
	v.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out
}

func (v *Vault) Item(id string) (Item, bool) {
	for _, it := range v.Items() {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

// Fields returns an item's plaintext values for in-process use only.
func (v *Vault) Fields(id string) (map[string]string, bool) {
	if !ValidItemID(id) {
		return nil, false
	}
	m, ok := v.Meta(itemName(id))
	if !ok || !isItemKind(m.Kind) {
		return nil, false
	}
	s, _ := v.Get(itemName(id))
	var f map[string]string
	if json.Unmarshal([]byte(s), &f) != nil {
		return nil, false
	}
	return f, true
}

func (v *Vault) DeleteItem(id string) error {
	if _, ok := v.Fields(id); !ok {
		return ErrMissing
	}
	return v.Delete(itemName(id))
}

// TOTP is RFC 6238 (SHA-1, 30 s, 6 digits) from a base32 secret.
func TOTP(secret string, t time.Time) (string, error) {
	s := strings.TrimRight(strings.ToUpper(strings.ReplaceAll(secret, " ", "")), "=")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil || len(key) < 10 {
		return "", ErrValue
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(t.Unix()/30))
	h := hmac.New(sha1.New, key)
	h.Write(msg[:])
	sum := h.Sum(nil)
	o := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[o:o+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1000000), nil
}

// Use is one audit line: which item a bot used, how and with what result.
// Metadata only, never a value.
type Use struct {
	At     string `json:"at"`
	Item   string `json:"item"`
	Label  string `json:"label"`
	Bot    string `json:"bot,omitempty"`
	Action string `json:"action"` // fill|env|reveal|save|delete
	Target string `json:"target,omitempty"`
	Result string `json:"result,omitempty"`
}

func (v *Vault) logPath() string { return filepath.Join(filepath.Dir(v.path), "vault_uses.jsonl") }

// LogUse appends an audit line (redacted again as a safety net).
func (v *Vault) LogUse(u Use) {
	if v == nil {
		return
	}
	u.At = time.Now().UTC().Format(time.RFC3339)
	u.Target, u.Result = v.Redact(u.Target), v.Redact(u.Result)
	b, _ := json.Marshal(u)
	v.mu.Lock()
	defer v.mu.Unlock()
	f, err := os.OpenFile(v.logPath(), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

// Uses returns the newest n audit lines, newest first.
func (v *Vault) Uses(n int) []Use {
	out := []Use{}
	if v == nil {
		return out
	}
	v.mu.RLock()
	b, _ := os.ReadFile(v.logPath())
	v.mu.RUnlock()
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		var u Use
		if json.Unmarshal([]byte(lines[i]), &u) == nil && u.At != "" {
			out = append(out, u)
		}
	}
	return out
}
