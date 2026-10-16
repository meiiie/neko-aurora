package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func independentL07Store(t *testing.T) (*Store, Work, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.ConfigureChallengeSimulator(true)
	w, _, err := s.Accept(input("independent-l07-work", "background"))
	if err != nil {
		t.Fatal(err)
	}
	return s, w, dir
}
func independentL07Input(w Work, nonce string) ChallengeInput {
	b := challengeBinding(w)
	return ChallengeInput{Action: "create", RequestID: w.RequestID, ExpectedEpoch: w.Epoch, AttemptID: w.AttemptID, AccountID: b.AccountID, AccountRevision: b.AccountRevision, Destination: w.ConversationID, Purpose: b.Purpose, ClientNonce: nonce}
}
func independentL07Candidate(s *Store, w Work, nonce string) Event {
	b := challengeBinding(w)
	at := s.clock().UTC()
	c := SecureChallenge{Version: 1, ID: "challenge-" + strings.Repeat("a", 32), Binding: b, Fingerprint: challengeFingerprint(b), CreatedAt: at.Format(time.RFC3339Nano), ExpiresAt: at.Add(5 * time.Minute).Format(time.RFC3339Nano), Status: "pending", SimulatorOnly: true}
	in := independentL07Input(w, nonce)
	return Event{Sequence: s.Snapshot().Sequence + 1, Type: "challenge.created", Challenge: &c, ChallengeInput: &in, Nonce: nonce}
}
func independentL07Resolve(out ChallengeResult, nonce string) ChallengeInput {
	return ChallengeInput{Action: "approve_simulation", ChallengeID: out.Challenge.ID, Fingerprint: out.Challenge.Fingerprint, ClientNonce: nonce}
}
func independentL07AssertNoCode(t *testing.T, s *Store, code string) {
	t.Helper()
	events, err := s.Events(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{s.Snapshot(), events} {
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		if bytes.Contains(raw, []byte(code)) || bytes.Contains(raw, []byte("simulationCode")) {
			t.Error("generated proof persisted in public projection")
		}
	}
	raw, err := os.ReadFile(s.file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(code)) {
		t.Error("generated proof persisted in journal")
	}
}
func TestIndependentL07ProofNonceCannotPersistCode(t *testing.T) {
	for _, kind := range []string{"create_nonce", "resolve_nonce"} {
		t.Run(kind, func(t *testing.T) {
			s, w, _ := independentL07Store(t)
			out := createChallengeFor(t, s, w, "independent-l07-create-first")
			before := s.Snapshot().Sequence
			var err error
			if kind == "create_nonce" {
				_, _, err = s.CreateSecureChallenge(independentL07Input(w, out.Code))
			} else {
				_, _, err = s.ResolveSecureChallenge(independentL07Resolve(out, out.Code), out.Code)
			}
			if err == nil || s.Snapshot().Sequence != before {
				t.Error("a generated proof used as clientNonce was accepted and journaled")
			}
			independentL07AssertNoCode(t, s, out.Code)
			receipt, e := s.CommandReceipt("secure_challenge", out.Code, "")
			if e == nil && receipt.Found {
				raw, _ := json.Marshal(receipt)
				if bytes.Contains(raw, []byte(out.Code)) {
					t.Error("generated proof echoed by durable receipt")
				}
			}
			if kind == "create_nonce" {
				resolved, _, e := s.ResolveSecureChallenge(independentL07Resolve(out, "independent-l07-safe-resolve"), out.Code)
				if e != nil || resolved.GrantsExecution || resolved.Status != "approve_simulation" {
					t.Fatal("control: original proof was not still live", e)
				}
			}
		})
	}
}
func TestIndependentL07CreatedReplayRequiresCurrentAuthority(t *testing.T) {
	for _, kind := range []string{"cancelled_task", "revoked_context"} {
		t.Run(kind, func(t *testing.T) {
			s, w, dir := independentL07Store(t)
			if !validReplay(s, independentL07Candidate(s, w, "independent-l07-replay-control")) {
				t.Fatal("control: healthy candidate rejected")
			}
			var err error
			if kind == "cancelled_task" {
				_, err = s.Cancel(w.RequestID)
			} else {
				_, _, err = s.SetSourceAccess(AccessInput{RequestID: w.RequestID, Granted: false, ClientNonce: "independent-l07-revoke-source"})
			}
			if err != nil {
				t.Fatal(err)
			}
			current, err := s.Get(w.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			before := s.Snapshot().Sequence
			if _, _, err = s.CreateSecureChallenge(independentL07Input(current, "independent-l07-live-denied")); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
				t.Fatal("control: inactive task live creation was not denied", err)
			}
			e := independentL07Candidate(s, current, "independent-l07-replay-denied")
			if validReplay(s, e) {
				t.Error("strict replay accepted new challenge for currently inactive task/context")
			}
			r := record{Version: 1, Event: e, Previous: s.digest}
			r.Digest, err = digestRecord(r)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			journal := s.file.Name()
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			f, err := os.OpenFile(journal, os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.Write(append(raw, '\n'))
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				t.Fatal("private synthetic frame write failed")
			}
			reopened, err := Open(dir)
			if reopened != nil {
				defer reopened.Close()
			}
			if !errors.Is(err, ErrCorrupt) {
				t.Error("checksum-correct inactive-authority journal reopened instead of rejecting", err)
			}
		})
	}
}
func TestIndependentL07KnownMetadataNonceFence(t *testing.T) {
	for _, family := range []string{"bot_name", "project_save", "conversation_create"} {
		for _, direction := range []string{"challenge_first", "metadata_first"} {
			t.Run(family+"/"+direction, func(t *testing.T) {
				s, w, _ := independentL07Store(t)
				nonce := "independent-l07-shared-" + family
				channel := sharedChannel(t, s, "Independent challenge project", "independent-l07-project-channel")
				metadata := func() error {
					var err error
					switch family {
					case "bot_name":
						_, _, err = s.RenameBot("bot-davin", BotNameInput{Name: "Independent nonce metadata", ClientNonce: nonce})
					case "project_save":
						_, _, err = s.SaveProject(ProjectInput{Name: "Independent nonce project", Goal: "Bounded nonce check", ChannelIDs: []string{channel.ID}, ClientNonce: nonce})
					case "conversation_create":
						_, _, err = s.CreateConversation(ConversationCreateInput{BotID: "bot-davin", Title: "Independent nonce conversation", ClientNonce: nonce})
					}
					return err
				}
				if direction == "challenge_first" {
					createChallengeFor(t, s, w, nonce)
					before := s.Snapshot().Sequence
					if err := metadata(); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
						t.Error("metadata write reused committed challenge nonce", err)
					}
				} else {
					if err := metadata(); err != nil {
						t.Fatal("control: first metadata commit failed", err)
					}
					before := s.Snapshot().Sequence
					candidate := independentL07Candidate(s, w, nonce)
					if validReplay(s, candidate) {
						t.Error("strict replay reused committed metadata nonce for challenge")
					}
					if _, _, err := s.CreateSecureChallenge(independentL07Input(w, nonce)); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
						t.Error("challenge write reused committed metadata nonce", err)
					}
				}
			})
		}
	}
}
func TestIndependentL07ProofLifecycleScopeCopiesAndPrivacy(t *testing.T) {
	for _, scenario := range []string{"success_swap_and_copies", "task_revision", "expiry", "clock_rollback", "reopen", "account_rotation"} {
		t.Run(scenario, func(t *testing.T) {
			s, w, dir := independentL07Store(t)
			var rotate func()
			if scenario == "account_rotation" {
				s.Close()
				bound, c, a, accountIn, boundDir := accountFixture(t)
				s, dir = bound, boundDir
				s.ConfigureChallengeSimulator(true)
				w = acceptBound(t, s, a, "independent-l07-bound-work")
				rotate = func() {
					accountIn.ID = a.ID
					accountIn.ExpectedRevision = a.Revision
					accountIn.ClientNonce = "independent-l07-account-rotation"
					accountIn.Secret = "fixture-only-rotated-canary-not-a-key"
					if _, _, err := c.SaveAccount(accountIn); err != nil {
						t.Fatal("control: fixture account rotation", err)
					}
				}
			}
			original := copyWork(w)
			in := independentL07Input(w, "independent-l07-positive-create")
			out, dup, err := s.CreateSecureChallenge(in)
			if err != nil || dup || !lowerHex(out.Code, 64) || !out.Challenge.SimulatorOnly || out.Challenge.GrantsExecution {
				t.Fatal("control: valid creation failed", err)
			}
			resolve := independentL07Resolve(out, "independent-l07-positive-resolve")
			if scenario == "success_swap_and_copies" {
				otherInput := input("independent-l07-second-work", "background")
				otherInput.BotID = "bot-davin"
				other, _, e := s.Accept(otherInput)
				if e != nil {
					t.Fatal(e)
				}
				second := createChallengeFor(t, s, other, "independent-l07-second-create")
				before := s.Snapshot().Sequence
				if _, _, e = s.ResolveSecureChallenge(independentL07Resolve(second, "independent-l07-wrong-swap"), out.Code); !errors.Is(e, ErrConflict) || s.Snapshot().Sequence != before {
					t.Fatal("swapped proof accepted", e)
				}
				got, dup, e := s.ResolveSecureChallenge(resolve, out.Code)
				if e != nil || dup || got.Status != "approve_simulation" || got.GrantsExecution {
					t.Fatal("valid resolution", e)
				}
				before = s.Snapshot().Sequence
				if _, _, e = s.ResolveSecureChallenge(independentL07Resolve(out, "independent-l07-second-resolution"), out.Code); !errors.Is(e, ErrConflict) || s.Snapshot().Sequence != before {
					t.Fatal("proof reused", e)
				}
				retry, d, e := s.CreateSecureChallenge(in)
				if e != nil || !d || retry.Code != "" || retry.Challenge != got {
					t.Fatal("create ACK resurrected code", e)
				}
				if _, d, e = s.ResolveSecureChallenge(resolve, strings.Repeat("0", 64)); e != nil || !d || s.Snapshot().Sequence != before {
					t.Fatal("committed public ACK unavailable", e)
				}
				events, _ := s.Events(0)
				for i := range events {
					if events[i].Challenge != nil {
						events[i].Challenge.GrantsExecution = true
						events[i].ChallengeInput.Destination = "caller-mutated"
					}
				}
				snap := s.Snapshot()
				for i := range snap.SecureChallenges {
					snap.SecureChallenges[i].Binding.Destination = "caller-mutated"
				}
				receipt, e := s.CommandReceipt("secure_challenge", in.ClientNonce, "")
				if e != nil || receipt.Challenge == nil || receipt.Challenge.GrantsExecution || receipt.Challenge.Binding.Destination != w.ConversationID {
					t.Fatal("projection mutation changed descriptor", e)
				}
				receipt.Challenge.Binding.Destination = "caller-mutated"
				receipt, e = s.CommandReceipt("secure_challenge", in.ClientNonce, "")
				if e != nil || receipt.Challenge.Binding.Destination != w.ConversationID {
					t.Fatal("receipt copy aliased state", e)
				}
				current, _ := s.Get(w.RequestID)
				if !reflect.DeepEqual(current, original) {
					t.Fatal("simulation changed original task")
				}
				independentL07AssertNoCode(t, s, out.Code)
				return
			}
			created, _ := time.Parse(time.RFC3339Nano, out.Challenge.CreatedAt)
			switch scenario {
			case "task_revision":
				supplement := controlInput(w, "supplement", "independent-l07-supplement")
				supplement.Text = "Bounded additional requirement"
				if _, _, err = s.ControlTask(supplement); err != nil {
					t.Fatal("control: supplement", err)
				}
			case "expiry":
				s.now = func() time.Time { return created.Add(5 * time.Minute) }
			case "clock_rollback":
				s.now = func() time.Time { return created.Add(-time.Second) }
			case "account_rotation":
				rotate()
			case "reopen":
				if err = s.Close(); err != nil {
					t.Fatal(err)
				}
				s, err = Open(dir)
				if err != nil {
					t.Fatal("historical replay", err)
				}
				defer s.Close()
				if _, _, err = s.ResolveSecureChallenge(resolve, out.Code); !errors.Is(err, ErrUnsupported) {
					t.Fatal("restart enabled simulation", err)
				}
				s.ConfigureChallengeSimulator(true)
				retry, d, e := s.CreateSecureChallenge(in)
				if e != nil || !d || retry.Code != "" {
					t.Fatal("restart restored proof in create ACK", e)
				}
			}
			before := s.Snapshot().Sequence
			if _, _, err = s.ResolveSecureChallenge(resolve, out.Code); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
				t.Fatal("invalid current authority resolved", err)
			}
			if scenario == "expiry" || scenario == "clock_rollback" {
				s.now = func() time.Time { return created.Add(time.Minute) }
				if _, _, err = s.ResolveSecureChallenge(resolve, out.Code); !errors.Is(err, ErrConflict) {
					t.Fatal("observed expired/rollback proof revived", err)
				}
			}
			independentL07AssertNoCode(t, s, out.Code)
		})
	}
}
