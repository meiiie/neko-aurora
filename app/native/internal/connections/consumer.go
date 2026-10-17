package connections

import (
	"encoding/json"
	"nekobot.local/native/internal/protocol"
	"nekobot.local/native/internal/providerexec"
	"path/filepath"
	"regexp"
)

type ConsumerSelection struct {
	pin       Pin
	reference string
}

func (selection ConsumerSelection) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Account Pin `json:"account"`
	}{selection.pin})
}
func (selection ConsumerSelection) AccountPin() Pin { return selection.pin }
func (selection ConsumerSelection) PrivateFrame(operationID string, attempt uint64, inputSHA256 string) ([]byte, error) {
	if !ValidPin(selection.pin) || !regexp.MustCompile(`^secret-[a-f0-9]{32}$`).MatchString(selection.reference) || !regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`).MatchString(operationID) || attempt == 0 || attempt > 9007199254740991 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(inputSHA256) {
		return nil, ErrInvalid
	}
	return json.Marshal(struct {
		Version     int    `json:"version"`
		OperationID string `json:"operationId"`
		Attempt     uint64 `json:"attempt"`
		InputSHA256 string `json:"inputSHA256"`
		Binding     string `json:"binding"`
		Account     Pin    `json:"account"`
		SecretRef   string `json:"secretRef"`
	}{1, operationID, attempt, inputSHA256, "local-owner:local-space:" + selection.pin.AccountID, selection.pin, selection.reference})
}
func (s *Store) SelectConsumer(pin Pin) (ConsumerSelection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ValidPin(pin) {
		return ConsumerSelection{}, ErrInvalid
	}
	for _, a := range s.data.Accounts {
		if a.ID == pin.AccountID && a.HasSecret && pinAccount(a.Account) == pin && !sensitiveAccountMetadata(a.Account) && regexp.MustCompile(`^secret-[a-f0-9]{32}$`).MatchString(a.SecretRef) {
			return ConsumerSelection{pin, a.SecretRef}, nil
		}
	}
	return ConsumerSelection{}, ErrConflict
}
func (s *Store) ConsumerCurrent(selection ConsumerSelection) bool {
	current, e := s.SelectConsumer(selection.pin)
	return e == nil && current.reference == selection.reference
}

// PrivateSavedTaskFrame is an explicit private transport encoder. Normal selection
// marshaling remains public-safe; no provider material is read from the vault.
func (s *Store) PrivateSavedTaskFrame(selection ConsumerSelection, binding providerexec.SavedTaskBinding, connectionsRoot, capsulePath, stateHome, revisionKey string) ([]byte, error) {
	if !s.ConsumerCurrent(selection) {
		return nil, ErrConflict
	}
	pin := selection.AccountPin()
	if binding.FixtureOnly || binding.AccountID != pin.AccountID || binding.AccountRevision != int64(pin.Revision) || binding.CredentialGeneration != int64(pin.CredentialGeneration) || binding.Provider != pin.Provider || binding.Endpoint != pin.Endpoint || binding.Model != pin.Model {
		return nil, ErrInvalid
	}
	if _, e := providerexec.SavedTaskBindingHash(binding); e != nil {
		return nil, ErrInvalid
	}
	actual, e := filepath.Abs(s.dir)
	if e != nil {
		return nil, ErrInvalid
	}
	if connectionsRoot != actual || !privateConsumerPath(connectionsRoot) || !privateConsumerPath(capsulePath) || !privateConsumerPath(stateHome) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(revisionKey) {
		return nil, ErrInvalid
	}
	raw, e := json.Marshal(struct {
		Version         int                           `json:"version"`
		Purpose         string                        `json:"purpose"`
		Binding         providerexec.SavedTaskBinding `json:"binding"`
		SecretRef       string                        `json:"secretRef"`
		ConnectionsRoot string                        `json:"connectionsRoot"`
		CapsulePath     string                        `json:"capsulePath"`
		StateHome       string                        `json:"stateHome"`
		RevisionKey     string                        `json:"revisionKey"`
	}{1, "saved_account_task_v1", binding, selection.reference, connectionsRoot, capsulePath, stateHome, revisionKey})
	if e != nil {
		return nil, ErrInvalid
	}
	defer clear(raw)
	canonical, e := protocol.Canonical(raw)
	if e != nil || len(raw) > 16384 || len(canonical) > 16384 {
		clear(canonical)
		return nil, ErrInvalid
	}
	if !s.ConsumerCurrent(selection) {
		clear(canonical)
		return nil, ErrConflict
	}
	return canonical, nil
}
func privateConsumerPath(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 4096 {
		return false
	}
	for _, c := range path {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}
