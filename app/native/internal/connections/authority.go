package connections

import (
	"net/url"
	"regexp"
)

// Pin is an immutable public descriptor, never a credential or bearer grant.
type Pin struct {
	AccountID            string `json:"accountId"`
	Revision             uint64 `json:"revision"`
	CredentialGeneration uint64 `json:"credentialGeneration"`
	Provider             string `json:"provider"`
	Endpoint             string `json:"endpoint"`
	Model                string `json:"model"`
}

func pinAccount(a Account) Pin {
	return Pin{a.ID, a.Revision, a.CredentialGeneration, a.Provider, a.Endpoint, a.Model}
}

func ValidPin(p Pin) bool {
	u, err := url.Parse(p.Endpoint)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && regexp.MustCompile(`^account-[a-f0-9]{32}$`).MatchString(p.AccountID) && p.Revision > 0 && p.CredentialGeneration > 0 && (p.Provider == "openai-compatible" || p.Provider == "anthropic") && text(p.Endpoint, 512) && text(p.Model, 120)
}

// The caller holds the ledger lock; account mutations never call into the ledger
// under this mutex. The callback linearizes publication/effects before revocation.
func (s *Store) WithPin(p Pin, commit func() error) error {
	release, err := s.AcquirePin(p)
	if err != nil {
		return err
	}
	defer release()
	return commit()
}

func (s *Store) AcquirePin(p Pin) (func(), error) {
	s.mu.Lock()
	if !ValidPin(p) {
		s.mu.Unlock()
		return nil, ErrInvalid
	}
	for _, a := range s.data.Accounts {
		if a.ID == p.AccountID && a.HasSecret && pinAccount(a.Account) == p {
			return s.mu.Unlock, nil
		}
	}
	s.mu.Unlock()
	return nil, ErrConflict
}

func (s *Store) WithSelection(id string, revision uint64, commit func(Pin) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.data.Accounts {
		if a.ID == id && a.Revision == revision && a.HasSecret && a.CredentialGeneration > 0 {
			return commit(pinAccount(a.Account))
		}
	}
	return ErrConflict
}
