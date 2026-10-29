package vault

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

const itemContacts = "__contacts"
const maxContacts = 10

var ErrContactConflict = errors.New("vault_contact_changed")

type ContactValue struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

// The first entry is the primary value used by existing fill consumers.
type ContactValues struct {
	Revision string         `json:"revision"`
	Emails   []ContactValue `json:"emails"`
	Phones   []ContactValue `json:"phones"`
}

type ContactHint struct {
	ID   string `json:"id"`
	Hint string `json:"hint"`
}

type ContactMetadata struct {
	Revision string        `json:"revision"`
	Emails   []ContactHint `json:"emails"`
	Phones   []ContactHint `json:"phones"`
}

type ContactChange struct {
	Revision string `json:"revision"`
	Action   string `json:"action"`
	Kind     string `json:"kind"`
	EntryID  string `json:"entryId"`
	Value    string `json:"value"`
}

func contactID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func contactValues(f map[string]string) (ContactValues, error) {
	c := ContactValues{Revision: "0", Emails: []ContactValue{}, Phones: []ContactValue{}}
	if f[itemContacts] == "" {
		if f["email"] != "" {
			c.Emails = append(c.Emails, ContactValue{"legacy-email", f["email"]})
		}
		if f["phone"] != "" {
			c.Phones = append(c.Phones, ContactValue{"legacy-phone", f["phone"]})
		}
		return c, nil
	}
	if json.Unmarshal([]byte(f[itemContacts]), &c) != nil || c.Revision == "" || len(c.Emails) > maxContacts || len(c.Phones) > maxContacts {
		return ContactValues{}, ErrCorrupt
	}
	seen := map[string]bool{}
	for _, list := range [][]ContactValue{c.Emails, c.Phones} {
		for _, entry := range list {
			if entry.ID == "" || len(entry.ID) > 40 || seen[entry.ID] || entry.Value == "" || len(entry.Value) > 512 || !utf8.ValidString(entry.Value) {
				return ContactValues{}, ErrCorrupt
			}
			seen[entry.ID] = true
		}
	}
	if firstContact(c.Emails) != f["email"] || firstContact(c.Phones) != f["phone"] {
		return ContactValues{}, ErrCorrupt
	}
	return c, nil
}

func firstContact(list []ContactValue) string {
	if len(list) == 0 {
		return ""
	}
	return list[0].Value
}

func contactMetadata(f map[string]string) *ContactMetadata {
	c, err := contactValues(f)
	if err != nil {
		return nil
	}
	m := &ContactMetadata{Revision: c.Revision, Emails: []ContactHint{}, Phones: []ContactHint{}}
	for _, v := range c.Emails {
		m.Emails = append(m.Emails, ContactHint{v.ID, MaskUser(v.Value)})
	}
	for _, v := range c.Phones {
		m.Phones = append(m.Phones, ContactHint{v.ID, "•••• " + tailContact(v.Value)})
	}
	return m
}

func tailContact(s string) string {
	r := []rune(s)
	if len(r) <= 4 {
		return ""
	}
	if len(r) > 4 {
		r = r[len(r)-4:]
	}
	return string(r)
}

func validContact(kind, value string) bool {
	if !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return false
	}
	if kind == "email" {
		a, err := mail.ParseAddress(value)
		return len(value) <= 254 && err == nil && a.Address == value && strings.Contains(value, "@")
	}
	if kind != "phone" || len(value) > 64 {
		return false
	}
	digits := 0
	for _, c := range value {
		if c >= '0' && c <= '9' {
			digits++
		} else if !strings.ContainsRune(" +()-.", c) {
			return false
		}
	}
	return digits >= 4 && digits <= 32
}

func writeContacts(f map[string]string, c ContactValues) error {
	var err error
	c.Revision, err = contactID()
	if err != nil {
		return err
	}
	for key, value := range map[string]string{"email": firstContact(c.Emails), "phone": firstContact(c.Phones)} {
		if value == "" {
			delete(f, key)
		} else {
			f[key] = value
		}
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	f[itemContacts] = string(b)
	return nil
}

// Legacy field edits replace only the primary entry, preserving other channels.
func mergeContacts(old, out map[string]string) error {
	if old[itemContacts] == "" && (len(old) == 0 || old["email"] == out["email"] && old["phone"] == out["phone"]) {
		return nil
	}
	c, err := contactValues(old)
	if err != nil {
		return err
	}
	changed := false
	for kind, list := range map[string]*[]ContactValue{"email": &c.Emails, "phone": &c.Phones} {
		value := out[kind]
		if value == firstContact(*list) {
			continue
		}
		if !validContact(kind, value) {
			return ErrValue
		}
		for _, v := range (*list)[min(1, len(*list)):] {
			if v.Value == value {
				return ErrValue
			}
		}
		if len(*list) == 0 {
			id, err := contactID()
			if err != nil {
				return err
			}
			*list = append(*list, ContactValue{id, value})
		} else {
			(*list)[0].Value = value
		}
		changed = true
	}
	if changed {
		return writeContacts(out, c)
	}
	out[itemContacts] = old[itemContacts]
	return nil
}

func (v *Vault) ChangeContact(id string, change ContactChange) (Item, error) {
	if v == nil || !ValidItemID(id) {
		return Item{}, ErrMissing
	}
	if change.Revision == "" || (change.Kind != "email" && change.Kind != "phone") {
		return Item{}, ErrValue
	}
	value := strings.TrimSpace(change.Value)
	switch change.Action {
	case "add":
		if change.EntryID != "" || !validContact(change.Kind, value) {
			return Item{}, ErrValue
		}
	case "replace":
		if change.EntryID == "" || !validContact(change.Kind, value) {
			return Item{}, ErrValue
		}
	case "remove", "primary":
		if change.EntryID == "" || change.Value != "" {
			return Item{}, ErrValue
		}
	default:
		return Item{}, ErrValue
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	m, ok := v.items[itemName(id)]
	if !ok {
		return Item{}, ErrMissing
	}
	if m.Kind != "vault:identity" {
		return Item{}, ErrType
	}
	f, err := v.fieldsLocked(id)
	if err != nil {
		return Item{}, err
	}
	c, err := contactValues(f)
	if err != nil {
		return Item{}, err
	}
	if c.Revision != change.Revision {
		return Item{}, ErrContactConflict
	}
	list := &c.Emails
	if change.Kind == "phone" {
		list = &c.Phones
	}
	index := -1
	for i, entry := range *list {
		if entry.ID == change.EntryID {
			index = i
		}
		if (change.Action == "add" || change.Action == "replace") && entry.Value == value && entry.ID != change.EntryID {
			return Item{}, ErrValue
		}
	}
	if change.Action != "add" && index < 0 {
		return Item{}, ErrMissing
	}
	switch change.Action {
	case "add":
		if len(*list) >= maxContacts {
			return Item{}, ErrValue
		}
		entryID, err := contactID()
		if err != nil {
			return Item{}, err
		}
		*list = append(*list, ContactValue{entryID, value})
	case "replace":
		(*list)[index].Value = value
	case "remove":
		*list = append((*list)[:index], (*list)[index+1:]...)
	case "primary":
		entry := (*list)[index]
		copy((*list)[1:index+1], (*list)[:index])
		(*list)[0] = entry
	}
	if err := writeContacts(f, c); err != nil {
		return Item{}, err
	}
	b, err := json.Marshal(f)
	if err != nil || len(b) > 8192 {
		return Item{}, ErrValue
	}
	meta, err := v.setLocked(m.Meta, string(b))
	if err != nil {
		return Item{}, err
	}
	return itemWithFields(meta, f), nil
}

// RevealItem is for the owner's explicit reveal; Fields keeps fill/env primary-only.
func (v *Vault) RevealItem(id string) (map[string]string, *ContactValues, error) {
	if v == nil || !ValidItemID(id) {
		return nil, nil, ErrMissing
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	f, err := v.fieldsLocked(id)
	if err != nil {
		return nil, nil, err
	}
	var contacts *ContactValues
	if v.items[itemName(id)].Kind == "vault:identity" {
		c, err := contactValues(f)
		if err != nil {
			return nil, nil, err
		}
		contacts = &c
	}
	delete(f, itemUseHint)
	delete(f, itemContacts)
	return f, contacts, nil
}
