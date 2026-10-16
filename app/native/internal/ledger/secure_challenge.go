package ledger

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"time"
)

type ChallengeBinding struct {
	OwnerUserID          string `json:"ownerUserId"`
	RequestID            string `json:"requestId"`
	Epoch                uint64 `json:"epoch"`
	TaskRevision         uint64 `json:"taskRevision"`
	ContextRevision      uint64 `json:"contextRevision"`
	AttemptID            string `json:"attemptId"`
	AccountID            string `json:"accountId"`
	AccountRevision      uint64 `json:"accountRevision"`
	CredentialGeneration uint64 `json:"credentialGeneration"`
	Destination          string `json:"destination"`
	Purpose              string `json:"purpose"`
	ContextSHA256        string `json:"contextSha256"`
}
type SecureChallenge struct {
	Version         int              `json:"version"`
	ID              string           `json:"id"`
	Binding         ChallengeBinding `json:"binding"`
	Fingerprint     string           `json:"fingerprint"`
	CreatedAt       string           `json:"createdAt"`
	ExpiresAt       string           `json:"expiresAt"`
	Status          string           `json:"status"`
	ResolvedAt      string           `json:"resolvedAt,omitempty"`
	SimulatorOnly   bool             `json:"simulatorOnly"`
	GrantsExecution bool             `json:"grantsExecution"`
}
type ChallengeInput struct {
	Action          string `json:"action"`
	RequestID       string `json:"requestId,omitempty"`
	ExpectedEpoch   uint64 `json:"expectedEpoch,omitempty"`
	AttemptID       string `json:"attemptId,omitempty"`
	AccountID       string `json:"accountId,omitempty"`
	AccountRevision uint64 `json:"accountRevision,omitempty"`
	Destination     string `json:"destination,omitempty"`
	Purpose         string `json:"purpose,omitempty"`
	ChallengeID     string `json:"challengeId,omitempty"`
	Fingerprint     string `json:"fingerprint,omitempty"`
	ClientNonce     string `json:"clientNonce"`
}
type ChallengeResult struct {
	Challenge SecureChallenge `json:"challenge"`
	Code      string          `json:"simulationCode,omitempty"`
}

type challengeProof struct {
	Digest [32]byte
	Active bool
}

var simulationHex = regexp.MustCompile(`[0-9a-fA-F]{64,}`)

func (s *Store) privateSimulationValue(raw []byte) bool {
	if len(s.challengeProofs) == 0 {
		return false
	}
	for _, run := range simulationHex.FindAll(raw, -1) {
		for offset := 0; offset+64 <= len(run); offset++ {
			digest := sha256.Sum256([]byte(strings.ToLower(string(run[offset : offset+64]))))
			for _, proof := range s.challengeProofs {
				if subtle.ConstantTimeCompare(digest[:], proof.Digest[:]) == 1 {
					return true
				}
			}
		}
	}
	return false
}

func (s *Store) challengeOtherNonceUsed(nonce string) bool {
	_, product := s.productNonces[nonce]
	_, conversation := s.conversationNonces[nonce]
	_, retention := s.retentionNonces[nonce]
	_, channel := s.channelNonces[nonce]
	_, plugin := s.pluginNonces[nonce]
	_, adapter := s.adapterNonces[nonce]
	for _, w := range s.work {
		if w.ClientNonce == nonce {
			return true
		}
	}
	return product || conversation || retention || channel || plugin || adapter || s.botMetadataNonceUsed(nonce) || s.lifecycleNonceUsed(nonce) || s.previousCommandNonceUsed(nonce)
}

func (s *Store) challengeEventNonceUsed(e Event) bool {
	_, command := s.challengeNonces[e.Nonce]
	_, work := s.challengeNonces[e.Work.ClientNonce]
	return command || work
}

func (s *Store) PublicInputAllowed(raw []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.privateSimulationValue(raw)
}

func challengeFingerprint(b ChallengeBinding) string {
	raw, _ := json.Marshal(b)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func challengeBinding(w Work) ChallengeBinding {
	b := ChallengeBinding{OwnerUserID: w.OwnerUserID, RequestID: w.RequestID, Epoch: w.Epoch, TaskRevision: w.TaskRevision, ContextRevision: w.ContextRevision, AttemptID: w.AttemptID, Destination: w.ConversationID, Purpose: "secure_input_simulation", ContextSHA256: contextHash(w)}
	if w.Account != nil {
		b.AccountID, b.AccountRevision, b.CredentialGeneration = w.Account.AccountID, w.Account.Revision, w.Account.CredentialGeneration
	}
	return b
}
func (s *Store) ConfigureChallengeSimulator(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.challengeSimulator = enabled && !s.readOnly
}
func challengeTimes(c SecureChallenge) bool {
	a, e := time.Parse(time.RFC3339Nano, c.CreatedAt)
	b, f := time.Parse(time.RFC3339Nano, c.ExpiresAt)
	return e == nil && f == nil && b.Sub(a) == 5*time.Minute && a.UTC().Format(time.RFC3339Nano) == c.CreatedAt && b.UTC().Format(time.RFC3339Nano) == c.ExpiresAt
}
func (s *Store) validChallengeEvent(e Event) bool {
	if e.Challenge == nil || e.ChallengeInput == nil || !reflect.DeepEqual(e, Event{Sequence: e.Sequence, Type: e.Type, Challenge: e.Challenge, ChallengeInput: e.ChallengeInput, Nonce: e.Nonce}) || e.Nonce != e.ChallengeInput.ClientNonce || !validAccessNonce(e.Nonce) || s.challengeOtherNonceUsed(e.Nonce) || len(s.challengeNonces) >= 512 {
		return false
	}
	c, in := *e.Challenge, *e.ChallengeInput
	if c.Version != 1 || !resourceID(c.ID, "challenge-") || !c.SimulatorOnly || c.GrantsExecution || c.Fingerprint != challengeFingerprint(c.Binding) || !challengeTimes(c) {
		return false
	}
	if e.Type == "challenge.created" {
		w, ok := s.work[in.RequestID]
		expected := ChallengeInput{Action: "create", RequestID: w.RequestID, ExpectedEpoch: w.Epoch, AttemptID: w.AttemptID, AccountID: c.Binding.AccountID, AccountRevision: c.Binding.AccountRevision, Destination: w.ConversationID, Purpose: "secure_input_simulation", ClientNonce: in.ClientNonce}
		_, exists := s.challenges[c.ID]
		return ok && !exists && len(s.challenges) < 128 && w.DesiredState == "running" && controlActive(w) && s.contextAllowed(w) && c.Status == "pending" && c.ResolvedAt == "" && c.Binding == challengeBinding(w) && in == expected
	}
	old, ok := s.challenges[c.ID]
	if !ok || old.Status != "pending" || e.Type != "challenge.resolved" || (in.Action != "approve_simulation" && in.Action != "decline") || in != (ChallengeInput{Action: in.Action, ChallengeID: old.ID, Fingerprint: old.Fingerprint, ClientNonce: in.ClientNonce}) {
		return false
	}
	w, found := s.work[old.Binding.RequestID]
	if !found || challengeBinding(w) != old.Binding || w.DesiredState != "running" || !controlActive(w) || !s.contextAllowed(w) {
		return false
	}
	at, err := time.Parse(time.RFC3339Nano, c.ResolvedAt)
	created, _ := time.Parse(time.RFC3339Nano, c.CreatedAt)
	expires, _ := time.Parse(time.RFC3339Nano, c.ExpiresAt)
	if err != nil || at.Before(created) || !at.Before(expires) {
		return false
	}
	old.Status, old.ResolvedAt = in.Action, c.ResolvedAt
	return old == c
}
func (s *Store) CreateSecureChallenge(in ChallengeInput) (ChallengeResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.challengeSimulator || s.readOnly {
		return ChallengeResult{}, false, ErrUnsupported
	}
	if s.closed || s.poisoned {
		return ChallengeResult{}, false, ErrUnavailable
	}
	if e, ok := s.challengeNonces[in.ClientNonce]; ok {
		if e.Type != "challenge.created" || *e.ChallengeInput != in {
			return ChallengeResult{}, false, ErrConflict
		}
		return ChallengeResult{Challenge: s.challenges[e.Challenge.ID]}, true, nil
	}
	if !validAccessNonce(in.ClientNonce) || in.Action != "create" || len(s.challenges) >= 128 || len(s.challengeNonces) >= 512 {
		return ChallengeResult{}, false, ErrInvalid
	}
	if s.challengeOtherNonceUsed(in.ClientNonce) || s.privateSimulationValue([]byte(in.ClientNonce)) {
		return ChallengeResult{}, false, ErrConflict
	}
	w, ok := s.work[in.RequestID]
	if !ok {
		return ChallengeResult{}, false, ErrNotFound
	}
	b := challengeBinding(w)
	expected := ChallengeInput{Action: "create", RequestID: w.RequestID, ExpectedEpoch: w.Epoch, AttemptID: w.AttemptID, AccountID: b.AccountID, AccountRevision: b.AccountRevision, Destination: w.ConversationID, Purpose: b.Purpose, ClientNonce: in.ClientNonce}
	if in != expected || w.DesiredState != "running" || !controlActive(w) || !s.contextAllowed(w) || !s.accountAllowed(w) || !s.collaborationBudgetAllows(w) {
		return ChallengeResult{}, false, ErrConflict
	}
	id, err := newID("challenge-")
	if err != nil {
		return ChallengeResult{}, false, err
	}
	now := s.clock()
	c := SecureChallenge{Version: 1, ID: id, Binding: b, Fingerprint: challengeFingerprint(b), CreatedAt: now.UTC().Format(time.RFC3339Nano), ExpiresAt: now.Add(5 * time.Minute).UTC().Format(time.RFC3339Nano), Status: "pending", SimulatorOnly: true}
	code := make([]byte, 32)
	if _, err = rand.Read(code); err != nil {
		return ChallengeResult{}, false, err
	}
	encoded := hex.EncodeToString(code)
	hash := sha256.Sum256([]byte(encoded))
	for i := range code {
		code[i] = 0
	}
	e := Event{Type: "challenge.created", Challenge: &c, ChallengeInput: &in, Nonce: in.ClientNonce}
	if !s.validChallengeEvent(e) {
		return ChallengeResult{}, false, ErrInvalid
	}
	if err = s.commit(e); err != nil {
		return ChallengeResult{}, false, err
	}
	s.challengeProofs[c.ID] = challengeProof{Digest: hash, Active: true}
	return ChallengeResult{Challenge: c, Code: encoded}, false, nil
}
func (s *Store) ResolveSecureChallenge(in ChallengeInput, code string) (SecureChallenge, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.challengeSimulator || s.readOnly {
		return SecureChallenge{}, false, ErrUnsupported
	}
	if s.closed || s.poisoned {
		return SecureChallenge{}, false, ErrUnavailable
	}
	if !validAccessNonce(in.ClientNonce) || !lowerHex(code, 64) {
		return SecureChallenge{}, false, ErrInvalid
	}
	if e, ok := s.challengeNonces[in.ClientNonce]; ok {
		if e.Type != "challenge.resolved" || *e.ChallengeInput != in {
			return SecureChallenge{}, false, ErrConflict
		}
		return s.challenges[e.Challenge.ID], true, nil
	}
	if s.challengeOtherNonceUsed(in.ClientNonce) || s.privateSimulationValue([]byte(in.ClientNonce)) || len(s.challengeNonces) >= 512 {
		return SecureChallenge{}, false, ErrConflict
	}
	c, ok := s.challenges[in.ChallengeID]
	created, _ := time.Parse(time.RFC3339Nano, c.CreatedAt)
	expires, _ := time.Parse(time.RFC3339Nano, c.ExpiresAt)
	now := s.clock()
	if ok && (now.Before(created) || !now.Before(expires)) {
		proof := s.challengeProofs[in.ChallengeID]
		proof.Active = false
		s.challengeProofs[in.ChallengeID] = proof
		return SecureChallenge{}, false, ErrConflict
	}
	hash, proof := s.challengeProofs[in.ChallengeID]
	supplied := sha256.Sum256([]byte(code))
	if !ok || !proof || !hash.Active || subtle.ConstantTimeCompare(hash.Digest[:], supplied[:]) != 1 || c.Status != "pending" || in.Fingerprint != c.Fingerprint {
		return SecureChallenge{}, false, ErrConflict
	}
	w, ok := s.work[c.Binding.RequestID]
	if !ok || challengeBinding(w) != c.Binding || w.DesiredState != "running" || !controlActive(w) || !s.contextAllowed(w) || !s.accountAllowed(w) || !s.collaborationBudgetAllows(w) {
		return SecureChallenge{}, false, ErrConflict
	}
	c.Status, c.ResolvedAt = in.Action, now.UTC().Format(time.RFC3339Nano)
	e := Event{Type: "challenge.resolved", Challenge: &c, ChallengeInput: &in, Nonce: in.ClientNonce}
	if !s.validChallengeEvent(e) {
		return SecureChallenge{}, false, ErrConflict
	}
	if err := s.commit(e); err != nil {
		return SecureChallenge{}, false, err
	}
	hash.Active = false
	s.challengeProofs[c.ID] = hash
	return c, false, nil
}
