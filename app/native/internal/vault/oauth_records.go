package vault

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const oauthRecordPrefix = "record:mcp-oauth:"
const oauthRecordKind = "internal:mcp-oauth"
const maxOAuthRecordBytes = 32 << 10

var oauthRecordID = regexp.MustCompile(`^[a-f0-9]{64}$`)
var oauthSecretField = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)

// OAuthRecord separates protocol state/binding data from credentials. Secrets
// belong in Secrets so individual values are covered by the Vault log redactor.
// These records are never available through the public secret/item API.
type OAuthRecord struct {
	Data    json.RawMessage   `json:"data"`
	Secrets map[string]string `json:"secrets"`
}

type oauthRecordEnvelope struct {
	Version int `json:"version"`
	OAuthRecord
}

func isOAuthRecordName(name string) bool {
	return strings.HasPrefix(name, oauthRecordPrefix) && oauthRecordID.MatchString(strings.TrimPrefix(name, oauthRecordPrefix))
}

func validateOAuthRecord(record OAuthRecord) bool {
	data := bytes.TrimSpace(record.Data)
	if len(data) == 0 || len(data) > 16<<10 || data[0] != '{' || !utf8.Valid(data) || !json.Valid(data) || len(record.Secrets) > 16 {
		return false
	}
	for key, value := range record.Secrets {
		if !oauthSecretField.MatchString(key) || len(value) < 4 || len(value) > 8192 || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return false
		}
	}
	return true
}

func decodeOAuthRecord(value string) (OAuthRecord, error) {
	if len(value) > maxOAuthRecordBytes || !utf8.ValidString(value) {
		return OAuthRecord{}, ErrCorrupt
	}
	d := json.NewDecoder(strings.NewReader(value))
	d.DisallowUnknownFields()
	var envelope oauthRecordEnvelope
	if d.Decode(&envelope) != nil || envelope.Version != 1 || !validateOAuthRecord(envelope.OAuthRecord) {
		return OAuthRecord{}, ErrCorrupt
	}
	if _, err := d.Token(); err != io.EOF {
		return OAuthRecord{}, ErrCorrupt
	}
	return envelope.OAuthRecord, nil
}

func (v *Vault) SetOAuthRecord(id string, record OAuthRecord) error {
	if !oauthRecordID.MatchString(id) {
		return ErrName
	}
	if !validateOAuthRecord(record) {
		return ErrValue
	}
	b, err := json.Marshal(oauthRecordEnvelope{Version: 1, OAuthRecord: record})
	if err != nil || len(b) > maxOAuthRecordBytes {
		return ErrValue
	}
	_, err = v.set(Meta{Name: oauthRecordPrefix + id, Kind: oauthRecordKind}, string(b))
	return err
}

func (v *Vault) GetOAuthRecord(id string) (OAuthRecord, error) {
	if !oauthRecordID.MatchString(id) {
		return OAuthRecord{}, ErrName
	}
	value, ok := v.get(oauthRecordPrefix + id)
	if !ok {
		return OAuthRecord{}, ErrMissing
	}
	return decodeOAuthRecord(value)
}

func (v *Vault) DeleteOAuthRecord(id string) error {
	if !oauthRecordID.MatchString(id) {
		return ErrName
	}
	return v.delete(oauthRecordPrefix + id)
}

func redactOAuthRecord(text, value string) string {
	record, err := decodeOAuthRecord(value)
	if err != nil {
		return text
	}
	secrets := make([]string, 0, len(record.Secrets))
	for _, secret := range record.Secrets {
		secrets = append(secrets, secret)
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		text = strings.ReplaceAll(text, secret, "«OAuth credential»")
	}
	return text
}
