package ledger

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func networkSourceRun(t *testing.T, s *Store, c Channel, nonce string) (Work, NetworkReadScope) {
	t.Helper()
	w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", Text: "Shared owner task", ClientNonce: "owner-" + nonce, Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http", Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	return w, NetworkReadScope{SourceWorkID: w.RequestID, SourceEpoch: w.Epoch, SourceAttempt: NetworkAttempt(w)}
}

func TestNetworkAcceptedEscapedMaximumCanDispatch(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := sharedChannel(t, s, "Network escaped", "network-escaped-channel")
	_, source := networkSourceRun(t, s, c, "network-escaped-source")
	to, _, err := s.RegisterNetworkSession(NetworkSessionInput{BotID: "bot-davin", ConversationID: c.ID, DeviceID: "server", Tier: "turn", ExpectedChannelRevision: c.Revision, ClientNonce: "network-escaped-recipient", Scope: source})
	if err != nil {
		t.Fatal("active shared member could not register actual server recipient", err)
	}
	in := NetworkMessageInput{ToSessionID: to.ID, Text: strings.Repeat("<", NetworkMaxTextBytes), Wake: true, ClientNonce: "network-escaped-message", Scope: source}
	for i := 0; i < NetworkMaxFiles; i++ {
		in.Attachments = append(in.Attachments, NetworkAttachment{Name: fmt.Sprintf("part-%d.txt", i), Content: strings.Repeat("<", NetworkMaxAttachmentBytes/NetworkMaxFiles)})
	}
	m, _, err := s.SendNetworkMessage(in)
	if err != nil {
		t.Fatal("bounded escaped message was not accepted", err)
	}
	w, _, err := s.DispatchNetworkMessage(m.ID)
	if err != nil {
		t.Fatal("accepted message cannot be dispatched", err)
	}
	if len(w.Text) > 2048 || strings.Contains(w.Text, in.Text) || w.PeerMessageID != m.ID {
		t.Fatal("agent payload masqueraded as owner task text")
	}
	context, err := s.NetworkContext(w.RequestID)
	if err != nil || len(context) != 1 || context[0].Text != in.Text || len(context[0].Attachments) != NetworkMaxFiles {
		t.Fatal("typed original body/files lost", err)
	}
	in.ClientNonce = "network-escaped-over-total"
	in.Attachments[0].Content += "x"
	if _, _, err = s.SendNetworkMessage(in); !errors.Is(err, ErrInvalid) {
		t.Fatal("attachment aggregate overflow accepted", err)
	}
	in.ClientNonce = "network-escaped-nine-files"
	in.Attachments[0].Content = "x"
	in.Attachments = append(in.Attachments, NetworkAttachment{Name: "ninth.txt", Content: "x"})
	if _, _, err = s.SendNetworkMessage(in); !errors.Is(err, ErrInvalid) {
		t.Fatal("ninth attachment accepted", err)
	}
}

func TestNetworkFixtureScopeAndRevocationFailClosed(t *testing.T) {
	s, a, p, _ := fixtureLedger(t)
	c := fixtureChannel(t, s, p["owner"])
	owner := p["owner"]
	to, _, err := s.RegisterNetworkSession(NetworkSessionInput{BotID: "bot-davin", ConversationID: c.ID, ExpectedChannelRevision: c.Revision, ClientNonce: "network-fixture-recipient", Scope: NetworkReadScope{Principal: &owner}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.NetworkSnapshot(NetworkReadScope{}); !errors.Is(err, ErrForbidden) {
		t.Fatal("missing fixture identity admitted", err)
	}
	for _, role := range []string{"reader", "eve"} {
		pin := p[role]
		if _, _, err = s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "Forbidden send", ClientNonce: "network-fixture-" + role, Scope: NetworkReadScope{Principal: &pin}}); !errors.Is(err, ErrForbidden) {
			t.Fatal("reader/outsider sent", role, err)
		}
	}
	reader := p["reader"]
	if directory, err := s.NetworkDirectory(NetworkReadScope{Principal: &reader}); err != nil || len(directory) != 1 {
		t.Fatal("reader cannot see authorized shared directory", err)
	}
	bob := p["bob"]
	m, _, err := s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "Writer wake", Wake: true, ClientNonce: "network-fixture-wake", Scope: NetworkReadScope{Principal: &bob}})
	if err != nil {
		t.Fatal(err)
	}
	fyi, _, err := s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "Writer FYI", ClientNonce: "network-fixture-fyi", Scope: NetworkReadScope{Principal: &bob}})
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.AcceptFor(owner, Input{BotID: "bot-davin", Text: "Owner next turn", ClientNonce: "owner-network-fixture-turn", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeliverNetworkContext(w.RequestID, []string{fyi.ID}); err != nil {
		t.Fatal("valid multiple-principal hold failed", err)
	}
	if context, err := s.NetworkContext(w.RequestID); err != nil || len(context) != 1 {
		t.Fatal(err)
	}
	if err = a.RevokeSession(bob.SessionID); err != nil {
		t.Fatal(err)
	}
	if ids := s.NetworkPendingWakeIDs(); len(ids) != 0 {
		t.Fatal("revoked wake remains schedulable")
	}
	if _, _, err = s.DispatchNetworkMessage(m.ID); err == nil {
		t.Fatal("revoked writer woke Work")
	}
	if _, err = s.NetworkContext(w.RequestID); !errors.Is(err, ErrForbidden) {
		t.Fatal("revoked FYI disclosed to a later execution", err)
	}
	if _, err = s.NetworkSnapshot(NetworkReadScope{Principal: &bob}); err == nil {
		t.Fatal("revoked principal read mailbox")
	}
	if _, err = s.NetworkSnapshot(NetworkReadScope{Principal: &owner}); err != nil {
		t.Fatal("revoked sender locked unrelated owner out", err)
	}
}

func TestNetworkRevisionArchiveAndExpiryBlockFreshDispatch(t *testing.T) {
	for _, kind := range []string{"revision", "archive", "expiry"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			s, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Unix(1800000000, 0)
			s.now = func() time.Time { return now }
			c := sharedChannel(t, s, "Network invalidation", "network-invalidation-channel")
			to := networkSession(t, s, "bot-davin", c.ID, "network-invalidation-recipient")
			m, _, err := s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "Queued wake", Wake: true, ClientNonce: "network-invalidation-message"})
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "revision":
				_, _, err = s.SaveChannel(ChannelInput{ID: c.ID, Name: c.Name, BotIDs: []string{"bot-meiiie"}, ExpectedRevision: c.Revision, ClientNonce: "network-invalidation-revoke"})
			case "archive":
				_, _, err = s.ControlBot(BotControlInput{BotID: "bot-davin", Action: "archive", ClientNonce: "network-invalidation-archive"})
			case "expiry":
				now = now.Add(8 * 24 * time.Hour)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err = s.DispatchNetworkMessage(m.ID); !errors.Is(err, ErrConflict) {
				t.Fatal("invalidated wake dispatched", err)
			}
			if len(s.NetworkPendingWakeIDs()) != 0 {
				t.Fatal("invalidated wake remained schedulable")
			}
			s.Close()
			s, err = Open(dir)
			if err != nil {
				t.Fatal("historical mailbox invalid after current revocation/expiry", err)
			}
			defer s.Close()
		})
	}
}

func TestNetworkAgentCausalAndPairBudgetsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := sharedChannel(t, s, "Network budgets", "network-budget-channel")
	to := networkSession(t, s, "bot-davin", c.ID, "network-budget-recipient")
	_, source := networkSourceRun(t, s, c, "network-budget-source-a")
	for i := 0; i < 4; i++ {
		if _, _, err = s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "bounded", ClientNonce: fmt.Sprintf("network-budget-root-%d", i), Scope: source}); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, err = s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "fifth", ClientNonce: "network-budget-root-fifth", Scope: source}); !errors.Is(err, ErrConflict) {
		t.Fatal("restart reset root message budget", err)
	}
	for root := 0; root < 2; root++ {
		_, scope := networkSourceRun(t, s, c, fmt.Sprintf("network-budget-source-%d", root))
		for i := 0; i < 4; i++ {
			if _, _, err = s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "bounded", ClientNonce: fmt.Sprintf("network-budget-pair-%d-%d", root, i), Scope: scope}); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, source = networkSourceRun(t, s, c, "network-budget-source-final")
	if _, _, err = s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "thirteenth", ClientNonce: "network-budget-pair-final", Scope: source}); !errors.Is(err, ErrConflict) {
		t.Fatal("new root bypassed pair rate limit", err)
	}
}

func TestNetworkHumanDecisionProvenanceIsExistingDecisionOnly(t *testing.T) {
	for _, decision := range []string{"approved", "declined", "superseded", "imported"} {
		t.Run(decision, func(t *testing.T) {
			dir := t.TempDir()
			s, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			to := networkSession(t, s, "bot-davin", "conversation-bot-davin", "network-decision-recipient")
			m, _, err := s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "Please draft", Wake: true, ClientNonce: "network-decision-message"})
			if err != nil {
				t.Fatal(err)
			}
			w, _, err := s.DispatchNetworkMessage(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			runtime := "direct-http"
			if decision == "imported" {
				runtime = "imported"
			}
			w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: runtime, Model: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			w, err = s.DirectFinish(w.RequestID, w.Epoch, "completed", "", func(d *DirectRun) {
				d.Approval = &DirectApproval{Title: "Draft", Kind: "email", To: "fixture@example.test", Body: "Model draft", State: "pending"}
			})
			if err != nil {
				t.Fatal(err)
			}
			if s.networkApproval(w) != nil {
				t.Fatal("pending model output fabricated human decision")
			}
			state := decision
			if state == "imported" {
				state = "approved"
			}
			w, err = s.DirectDecide(w.RequestID, func(a *DirectApproval, d *DirectRun) error {
				a.Body = "Owner reviewed draft"
				a.State = state
				a.DecidedAt = "2026-10-25T12:00:00Z"
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			proof := s.networkApproval(w)
			if state == "superseded" || decision == "imported" {
				if proof != nil {
					t.Fatal("supersede/import fabricated human provenance")
				}
			} else if proof == nil || proof.State != state || proof.DecidedAt == "" || len(proof.Digest) != 64 {
				t.Fatal("actual human decision missing")
			}
			if decision == "approved" {
				w, err = s.DirectDecide(w.RequestID, func(a *DirectApproval, d *DirectRun) error {
					a.Delivery = &DirectDelivery{Status: "sent", At: "2026-10-25T12:01:00Z"}
					a.Outcome = "Fixture delivery"
					return nil
				})
				if err != nil || s.networkApproval(w).Digest != proof.Digest {
					t.Fatal("delivery progress changed human decision digest", err)
				}
			}
			s.Close()
			s, err = Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			views, err := s.NetworkSnapshot(NetworkReadScope{})
			if err != nil || len(views.Messages) != 1 {
				t.Fatal(err)
			}
			if proof == nil && views.Messages[0].Approval != nil || proof != nil && (views.Messages[0].Approval == nil || views.Messages[0].Approval.Digest != proof.Digest) {
				t.Fatal("restart changed human decision provenance")
			}
		})
	}
}

func TestNetworkResumedAttemptRecoversOriginalSendReceipt(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	c := sharedChannel(t, s, "Network resumed", "network-resumed-channel")
	to := networkSession(t, s, "bot-davin", c.ID, "network-resumed-recipient")
	w, firstScope := networkSourceRun(t, s, c, "network-resumed-source")
	in := NetworkMessageInput{ToSessionID: to.ID, Text: "Immutable handoff", Wake: true, ClientNonce: "network-resumed-stable-send", Scope: firstScope}
	original, _, err := s.SendNetworkMessage(in)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DirectResume(w.RequestID); err != nil {
		t.Fatal(err)
	}
	w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http", Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SendNetworkMessage(in); !errors.Is(err, ErrForbidden) {
		t.Fatal("stale tool call survived a resumed attempt", err)
	}
	in.Scope = NetworkReadScope{SourceWorkID: w.RequestID, SourceEpoch: w.Epoch, SourceAttempt: NetworkAttempt(w)}
	recovered, dup, err := s.SendNetworkMessage(in)
	if err != nil || !dup || recovered.ID != original.ID || recovered.SourceAttempt != original.SourceAttempt {
		t.Fatal("new live attempt failed to recover immutable receipt", err)
	}
	in.Text = "Changed handoff"
	if _, _, err = s.SendNetworkMessage(in); !errors.Is(err, ErrConflict) {
		t.Fatal("resumed nonce accepted changed payload", err)
	}
	if len(s.networkMessages) != 1 {
		t.Fatal("send-before-crash duplicated mail")
	}
}

func TestNetworkAgentReadCannotRecoverPriorAudience(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := sharedChannel(t, s, "Network old audience", "network-audience-channel")
	to := networkSession(t, s, "bot-meiiie", c.ID, "network-audience-recipient")
	old, _, err := s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "Old audience canary", ClientNonce: "network-audience-old-message"})
	if err != nil {
		t.Fatal(err)
	}
	c, _, err = s.SaveChannel(ChannelInput{ID: c.ID, Name: c.Name, BotIDs: c.BotIDs, ExpectedRevision: c.Revision, ClientNonce: "network-audience-revision"})
	if err != nil {
		t.Fatal(err)
	}
	_, source := networkSourceRun(t, s, c, "network-audience-new-source")
	view, err := s.NetworkSnapshot(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range view.Messages {
		if m.ID == old.ID {
			t.Fatal("current agent learned prior frozen audience mail")
		}
	}
	owner, err := s.NetworkSnapshot(NetworkReadScope{})
	if err != nil || len(owner.Messages) != 1 || owner.Messages[0].ID != old.ID {
		t.Fatal("owner mailbox lost historical record", err)
	}
}

func TestNetworkDescendantCannotResetCausalBudget(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := sharedChannel(t, s, "Network causal", "network-causal-channel")
	to := networkSession(t, s, "bot-davin", c.ID, "network-causal-recipient")
	back := networkSession(t, s, "bot-meiiie", c.ID, "network-causal-origin")
	root, source := networkSourceRun(t, s, c, "network-causal-root")
	m, _, err := s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "First handoff", Wake: true, ClientNonce: "network-causal-first", Scope: source})
	if err != nil {
		t.Fatal(err)
	}
	child, _, err := s.DispatchNetworkMessage(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	child, err = s.DirectStart(child.RequestID, child.Epoch, DirectRun{Runtime: "direct-http", Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	childScope := NetworkReadScope{SourceWorkID: child.RequestID, SourceEpoch: child.Epoch, SourceAttempt: NetworkAttempt(child)}
	for i := 0; i < 3; i++ {
		m, _, err = s.SendNetworkMessage(NetworkMessageInput{ToSessionID: back.ID, Text: "Descendant", ClientNonce: fmt.Sprintf("network-causal-child-%d", i), Scope: childScope})
		if err != nil || m.RootWorkID != root.RequestID {
			t.Fatal("child did not inherit causal root", err)
		}
	}
	if _, _, err = s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "Fifth across root and child", ClientNonce: "network-causal-fifth", Scope: source}); !errors.Is(err, ErrConflict) {
		t.Fatal("descendant reset global causal budget", err)
	}
}

func TestNetworkSchedulerOnlyConsumesActualServerSessions(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	active := networkSession(t, s, "bot-davin", "conversation-bot-davin", "network-scheduler-active")
	pull, _, err := s.RegisterNetworkSession(NetworkSessionInput{BotID: "bot-meiiie", Tier: "pull", ClientNonce: "network-scheduler-pull"})
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := s.RegisterNetworkSession(NetworkSessionInput{BotID: "bot-davin", DeviceID: "another-device", Tier: "turn", ClientNonce: "network-scheduler-other"})
	if err != nil || other.Tier != "pull" {
		t.Fatal("other device falsely advertised live turn support", err)
	}
	var expected string
	for i, target := range []NetworkSession{active, pull, other} {
		m, _, err := s.SendNetworkMessage(NetworkMessageInput{ToSessionID: target.ID, Text: "Wake addressed endpoint", Wake: true, ClientNonce: fmt.Sprintf("network-scheduler-wake-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			expected = m.ID
		}
	}
	ids := s.NetworkPendingWakeIDs()
	if len(ids) != 1 || ids[0] != expected {
		t.Fatal("scheduler charged unsupported endpoint", ids)
	}
	fyi, _, err := s.SendNetworkMessage(NetworkMessageInput{ToSessionID: other.ID, Text: "Other device FYI", ClientNonce: "network-scheduler-other-fyi"})
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.Accept(Input{BotID: "bot-davin", Text: "Server owner turn", ClientNonce: "owner-network-scheduler", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	if ids, err := s.NetworkPendingContextIDs(w.RequestID); err != nil || len(ids) != 0 {
		t.Fatal("other device mail exposed to server turn", err)
	}
	if err := s.DeliverNetworkContext(w.RequestID, []string{fyi.ID}); !errors.Is(err, ErrInvalid) {
		t.Fatal("server bound another device's mailbox", err)
	}
}
