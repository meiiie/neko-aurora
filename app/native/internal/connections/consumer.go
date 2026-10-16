package connections

import (
	"encoding/json"
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
