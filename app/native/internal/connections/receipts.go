package connections

import (
	"encoding/json"
)

type AccountSaveReceipt struct {
	Version          int      `json:"version"`
	ClientNonce      string   `json:"clientNonce"`
	Found            bool     `json:"found"`
	Used             bool     `json:"used"`
	InputFingerprint string   `json:"inputFingerprint,omitempty"`
	Account          *Account `json:"account,omitempty"`
}

func (s *Store) AccountReceipt(nonce string) (AccountSaveReceipt, error) {
	if !text(nonce, 128) {
		return AccountSaveReceipt{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := AccountSaveReceipt{Version: 1, ClientNonce: nonce, Used: containsNonce(s.data.UsedNonces, nonce)}
	for _, a := range s.data.Accounts {
		if a.Nonce == nonce {
			if sensitiveAccountMetadata(a.Account) {
				return AccountSaveReceipt{}, ErrInvalid
			}
			current := a.Account
			r.Found, r.Used, r.InputFingerprint, r.Account = true, true, a.Fingerprint, &current
			return r, nil
		}
	}
	return r, nil
}

func accountInputMetadata(in AccountInput) ([]byte, error) {
	return json.Marshal(map[string]any{"id": in.ID, "revision": in.ExpectedRevision, "name": in.Name, "provider": in.Provider, "endpoint": in.Endpoint, "model": in.Model, "hasSubmittedSecret": in.Secret != ""})
}

func sensitiveAccountMetadata(a Account) bool {
	return accountSecretPattern.MatchString(a.Name) || accountSecretPattern.MatchString(a.Model) || accountSecretPattern.MatchString(a.Endpoint)
}
