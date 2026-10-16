package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAdapterHistoryRepairRequiredBeyondDeltaLimits20261013(t *testing.T) {
	for _, kind := range []string{"lag", "rows", "bytes"} {
		t.Run(kind, func(t *testing.T) {
			s, c, p := productFixture(t)
			if _, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{IncludeHistory: false, ClientNonce: "history-repair-fixture-history-off"}); err != nil {
				t.Fatal(err)
			}
			a := adapterFixture(t, s, c, "history-repair-fixture-session")
			f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, Filename: "repair.txt", Content: []byte("Owned repair fixture source"), ClientNonce: "history-repair-fixture-file"})
			if err != nil {
				t.Fatal(err)
			}
			input := AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: "history-repair-original-input", Text: "Use owned fixture", Lane: "foreground"}, SourceMessageID: "repair-original-source-message", ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}}
			original, _, err := s.AcceptAdapter(a.ID, input)
			if err != nil {
				t.Fatal(err)
			}
			original = completeChannelWork(t, s, original, "Owned original derived reply")
			page, err := s.AdapterReplies(a.ID, 0)
			if err != nil || len(page.Replies) != 1 {
				t.Fatal("missing prior consumed reply", err)
			}
			after := page.Cursor
			if kind == "lag" {
				for i := 0; i < 2048; i++ {
					w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: fmt.Sprintf("history-repair-unrelated-%04d", i), Text: "Owned unrelated fixture", Lane: "background"})
					if err != nil {
						t.Fatal(err)
					}
					if _, err = s.Cancel(w.RequestID); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				count, body := 512, "Owned additional reply"
				if kind == "bytes" {
					count, body = 14, strings.Repeat("x", 65536)
				}
				for i := 0; i < count; i++ {
					message := AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: fmt.Sprintf("history-repair-output-%04d", i), Text: "Owned repair output fixture", Lane: "foreground"}, SourceMessageID: fmt.Sprintf("repair-output-source-%04d", i)}
					if kind == "rows" {
						message.ProjectID, message.ProjectRevision, message.FileIDs = p.ID, p.Revision, []string{f.ID}
					}
					w, _, err := s.AcceptAdapter(a.ID, message)
					if err != nil {
						t.Fatal(err)
					}
					completeChannelWork(t, s, w, body)
				}
				if kind == "rows" {
					after = s.Snapshot().Sequence
				}
			}
			if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "history-repair-source-revoke"}); err != nil {
				t.Fatal(err)
			}
			before := s.Snapshot().Sequence
			if _, err = s.AdapterReplies(a.ID, after); !errors.Is(err, ErrConflict) {
				t.Fatal("fixture did not reproduce bounded delta conflict", kind, err)
			}
			ack, duplicate, err := s.AcceptAdapter(a.ID, input)
			if err != nil || !duplicate || ack.RequestID != original.RequestID || ack.Adapter.ClientNonce != input.ClientNonce || s.Snapshot().Sequence != before {
				t.Fatal("blocked delta lost original ACK or mutated history", err)
			}
			capabilities, err := s.AdapterCapabilities()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, capability := range capabilities.Capabilities {
				found = found || capability == "local_adapter_history_repair_v1"
			}
			if !found {
				t.Fatalf("explicit same-session history repair unavailable after genuine %s conflict: journal=%d oldCursor=%d; original ACK preserved", kind, before, after)
			}
			key, now := historyRepairInputs20261013()
			cursor := ""
			seen := map[string]bool{}
			pages := 0
			for {
				repair, err := s.AdapterHistoryPage(a.ID, cursor, key, now)
				if err != nil || repair.AdapterSessionID != a.ID || repair.SourceThreadID != a.SourceThreadID || repair.Offset != uint64(len(seen)) || repair.SnapshotCursor != before {
					t.Fatal("repair changed binding/head/contiguous position", kind, err)
				}
				encoded, err := json.Marshal(repair)
				if err != nil || len(encoded) > 900000 || len(repair.Rows) > 128 {
					t.Fatal("repair escaped bounded response", err)
				}
				pages++
				for _, row := range repair.Rows {
					if seen[row.RequestID] || row.ReplyID != "reply-"+row.RequestID || row.Sequence > before {
						t.Fatal("repair duplicated, rebound or invented a row sequence")
					}
					seen[row.RequestID] = true
					if kind != "bytes" || row.RequestID == original.RequestID {
						if row.Availability != "source_unavailable" || row.Text != "" || len(row.Artifacts) != 0 {
							t.Fatal("repair restored revoked fixture output")
						}
					} else if row.Availability != "available" || len(row.Text) != 65536 {
						t.Fatal("repair truncated authorized output")
					}
				}
				if repair.Complete {
					if repair.NextCursor != nil || repair.ConfirmationCursor == nil || repair.TotalRows != uint64(len(seen)) {
						t.Fatal("repair missing complete projection proof")
					}
					confirm, err := s.AdapterHistoryPage(a.ID, *repair.ConfirmationCursor, key, now)
					if err != nil || !confirm.Complete || len(confirm.Rows) != 0 || confirm.Offset != repair.TotalRows || confirm.HistoryDigest != repair.HistoryDigest || confirm.AuthorityDigest != repair.AuthorityDigest {
						t.Fatal("final repair confirmation mixed generations", err)
					}
					break
				}
				if repair.NextCursor == nil || repair.ConfirmationCursor != nil || pages > 100 {
					t.Fatal("repair did not advance bounded pages")
				}
				cursor = *repair.NextCursor
			}
			expected := map[string]int{"lag": 1, "rows": 513, "bytes": 15}[kind]
			if len(seen) != expected || !seen[original.RequestID] || s.Snapshot().Sequence != before || kind != "lag" && pages < 2 {
				t.Fatal("repair lost accepted history or mutated journal", kind, len(seen), pages)
			}
		})
	}
}

func historyRepairInputs20261013() ([32]byte, time.Time) {
	var key [32]byte
	for i := range key {
		key[i] = 1
	}
	return key, time.UnixMilli(1802400000000)
}

func TestAdapterHistoryRepairVectors20261013(t *testing.T) {
	raw, err := os.ReadFile("../../../scripts/adapter_history_repair_vectors_20261013.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name  string
			Scope struct {
				AdapterSessionID, SourceApp, SourceSessionID, SourceThreadID, ChannelID, OwnerUserID, SpaceID, ReturnRoute string
				ChannelRevision                                                                                            uint64
			}
			SnapshotCursor, AuthorityEpoch, IssuedAtUnixMS, ExpiresAtUnixMS uint64
			LivePredicates                                                  []bool
			Rows                                                            []AdapterHistoryRow
			FixtureSigningKeyHex                                            string
			Expected                                                        struct {
				ScopeBytes, TotalBytes, CursorRawBytes, CursorEncodedChars                  uint64
				ScopeSHA256, AuthorityDigest, HistoryDigest, SnapshotID, ConfirmationCursor string
				RowByteLengths                                                              []uint64
			}
		}
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, v := range fixture.Cases {
		t.Run(v.Name, func(t *testing.T) {
			scope := adapterHistoryScope(AdapterSession{ID: v.Scope.AdapterSessionID, SourceApp: v.Scope.SourceApp, SourceSessionID: v.Scope.SourceSessionID, SourceThreadID: v.Scope.SourceThreadID, ChannelID: v.Scope.ChannelID, OwnerUserID: v.Scope.OwnerUserID, SpaceID: v.Scope.SpaceID, ReturnRoute: v.Scope.ReturnRoute, ChannelRevision: v.Scope.ChannelRevision})
			scopeHash := sha256.Sum256(scope)
			if len(scope) != int(v.Expected.ScopeBytes) || hex.EncodeToString(scopeHash[:]) != v.Expected.ScopeSHA256 {
				t.Fatal("portable scope vector mismatch")
			}
			authority := sha256.New()
			authority.Write([]byte("neko-adapter-authority-v1\x00"))
			authority.Write(scope)
			adapterHistoryU64(authority, v.AuthorityEpoch)
			adapterHistoryU64(authority, uint64(len(v.Rows)))
			for i, row := range v.Rows {
				adapterHistoryString(authority, row.RequestID)
				flag := byte(0)
				if v.LivePredicates[i] {
					flag = 1
				}
				authority.Write([]byte{flag})
			}
			if hex.EncodeToString(authority.Sum(nil)) != v.Expected.AuthorityDigest {
				t.Fatal("portable authority vector mismatch")
			}
			history := sha256.New()
			history.Write([]byte("neko-adapter-history-v1\x00"))
			history.Write(scope)
			adapterHistoryU64(history, v.SnapshotCursor)
			history.Write(authority.Sum(nil))
			adapterHistoryU64(history, uint64(len(v.Rows)))
			total := uint64(0)
			for i, row := range v.Rows {
				encoded, err := adapterHistoryRowBytes(row)
				if err != nil || uint64(len(encoded)) != v.Expected.RowByteLengths[i] {
					t.Fatal("portable Unicode/artifact row vector mismatch", err)
				}
				total += uint64(len(encoded))
				history.Write(encoded)
			}
			if total != v.Expected.TotalBytes || hex.EncodeToString(history.Sum(nil)) != v.Expected.HistoryDigest {
				t.Fatal("portable history digest vector mismatch")
			}
			snapshot := sha256.New()
			snapshot.Write([]byte("neko-adapter-snapshot-v1\x00"))
			snapshot.Write(scope)
			adapterHistoryU64(snapshot, v.SnapshotCursor)
			snapshot.Write(authority.Sum(nil))
			snapshot.Write(history.Sum(nil))
			if "h1_"+hex.EncodeToString(snapshot.Sum(nil)) != v.Expected.SnapshotID {
				t.Fatal("portable snapshot vector mismatch")
			}
			var key [32]byte
			decoded, _ := hex.DecodeString(v.FixtureSigningKeyHex)
			copy(key[:], decoded)
			cursor := adapterHistoryCursor{scope: scopeHash, head: v.SnapshotCursor, offset: uint64(len(v.Rows)), issued: v.IssuedAtUnixMS, expires: v.ExpiresAtUnixMS}
			copy(cursor.authority[:], authority.Sum(nil))
			copy(cursor.history[:], history.Sum(nil))
			if token := adapterHistoryToken(cursor, key); token != v.Expected.ConfirmationCursor || len(token) != int(v.Expected.CursorEncodedChars) {
				t.Fatal("portable signed cursor vector mismatch")
			}
		})
	}
	if _, err = adapterHistoryRowBytes(AdapterHistoryRow{Text: string([]byte{255})}); !errors.Is(err, ErrAdapterHistoryBudgetExceeded) {
		t.Fatal("invalid UTF-8 accepted into canonical bytes", err)
	}
}

func TestAdapterHistoryRepairFrozenStreamingAndAuthorityABA20261013(t *testing.T) {
	s, c, _ := productFixture(t)
	if _, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{IncludeHistory: false, ClientNonce: "history-repair-stream-history-off"}); err != nil {
		t.Fatal(err)
	}
	a := adapterFixture(t, s, c, "history-repair-stream-session")
	var live Work
	for i := 0; i < 3; i++ {
		w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: fmt.Sprintf("history-repair-stream-%d", i), Text: "Owned streaming fixture", Lane: "foreground"}, SourceMessageID: fmt.Sprintf("repair-stream-source-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			live = startPermissionWork(t, s, w)
			if err = s.AppendText(live.RequestID, live.Epoch, strings.Repeat("\x00", 65535)); err != nil {
				t.Fatal(err)
			}
		} else {
			completeChannelWork(t, s, w, strings.Repeat("\x00", 65536))
		}
	}
	key, now := historyRepairInputs20261013()
	first, err := s.AdapterHistoryPage(a.ID, "", key, now)
	if err != nil || first.NextCursor == nil || first.Complete || len(first.Rows) != 2 {
		t.Fatal("escaped-text page did not split", err)
	}
	if err = s.AppendText(live.RequestID, live.Epoch, "x"); err != nil {
		t.Fatal(err)
	}
	second, err := s.AdapterHistoryPage(a.ID, *first.NextCursor, key, now)
	if err != nil || !second.Complete || len(second.Rows) != 1 || second.Rows[0].Text != strings.Repeat("\x00", 65535) || second.SnapshotCursor != first.SnapshotCursor || second.HistoryDigest != first.HistoryDigest {
		t.Fatal("later text replaced frozen projection", err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: first.Rows[0].RequestID, Granted: false, ClientNonce: "history-repair-aba-revoke"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: first.Rows[0].RequestID, ExpectedRevision: 1, Granted: true, ClientNonce: "history-repair-aba-regrant"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdapterHistoryPage(a.ID, *second.ConfirmationCursor, key, now); !errors.Is(err, ErrAdapterHistoryAuthorityChanged) {
		t.Fatal("revoke/regrant ABA confirmed stale projection", err)
	}
}

func TestAdapterHistoryRepairCursorScopeClockSignerAndReadonly20261013(t *testing.T) {
	s, _, _, a, f, _, _, _ := adapterInvalidationFixture20261013(t)
	key, now := historyRepairInputs20261013()
	page, err := s.AdapterHistoryPage(a.ID, "", key, now)
	if err != nil || page.ConfirmationCursor == nil {
		t.Fatal(err)
	}
	token := *page.ConfirmationCursor
	if _, err = s.AdapterHistoryPage(a.ID, token[:214]+"!", key, now); !errors.Is(err, ErrAdapterHistoryCursorInvalid) {
		t.Fatal("tampered cursor accepted", err)
	}
	retired := key
	retired[0]++
	if _, err = s.AdapterHistoryPage(a.ID, token, retired, now); !errors.Is(err, ErrAdapterHistoryCursorInvalid) {
		t.Fatal("retired signer accepted", err)
	}
	for _, clock := range []time.Time{now.Add(-time.Millisecond), now.Add(5*time.Minute + time.Millisecond)} {
		if _, err = s.AdapterHistoryPage(a.ID, token, key, clock); !errors.Is(err, ErrAdapterHistorySnapshotExpired) {
			t.Fatal("changed clock/expiry accepted", err)
		}
	}
	b := adapterFixture(t, s, mustAdapterChannel20261013(t, s, a.ChannelID), "history-repair-foreign-session")
	if _, err = s.AdapterHistoryPage(b.ID, token, key, now); !errors.Is(err, ErrAdapterHistoryBindingMismatch) {
		t.Fatal("cross-session cursor accepted", err)
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "history-repair-readonly-deny"}); err != nil {
		t.Fatal(err)
	}
	dir := s.dir
	before := s.Snapshot().Sequence
	s.Close()
	s, err = OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	masked, err := s.AdapterHistoryPage(a.ID, "", key, now)
	if err != nil || len(masked.Rows) != 1 || masked.Rows[0].Availability != "source_unavailable" || masked.Rows[0].Text != "" || len(masked.Rows[0].Artifacts) != 0 || s.Snapshot().Sequence != before {
		t.Fatal("read-only repair exposed or rewrote denied history", err)
	}
}

func TestAdapterHistoryRepairUnknownAuthorityMasksAndInvalidates20261013(t *testing.T) {
	s, _, _, a, f, _, w, _ := adapterInvalidationFixture20261013(t)
	key, now := historyRepairInputs20261013()
	first, err := s.AdapterHistoryPage(a.ID, "", key, now)
	if err != nil || len(first.Rows) != 1 || first.Rows[0].Availability != "available" {
		t.Fatal("missing initially authorized repair fixture", err)
	}
	before := s.Snapshot().Sequence
	f.Granted = false
	s.uploads[f.ID] = f
	if _, err = s.AdapterHistoryPage(a.ID, *first.ConfirmationCursor, key, now); !errors.Is(err, ErrAdapterHistoryAuthorityChanged) {
		t.Fatal("unknown no-event denial confirmed cached body", err)
	}
	masked, err := s.AdapterHistoryPage(a.ID, "", key, now)
	if err != nil || len(masked.Rows) != 1 || masked.Rows[0].RequestID != w.RequestID || masked.Rows[0].Availability != "source_unavailable" || masked.Rows[0].Text != "" || len(masked.Rows[0].Artifacts) != 0 || s.Snapshot().Sequence != before || s.poisoned {
		t.Fatal("unknown authority repair leaked, rebound or invented journal state", err)
	}
	if _, err = s.AdapterHistoryPage(a.ID, *masked.ConfirmationCursor, key, now); err != nil {
		t.Fatal("masked unknown-authority projection could not confirm", err)
	}
}

func TestAdapterHistoryRepairSignedOffsetAndSingleRowPolicy20261013(t *testing.T) {
	s, _, _, a, _, _, _, _ := adapterInvalidationFixture20261013(t)
	key, now := historyRepairInputs20261013()
	page, err := s.AdapterHistoryPage(a.ID, "", key, now)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := adapterHistoryParseToken(*page.ConfirmationCursor, adapterHistoryScope(a), key, uint64(now.UnixMilli()))
	if err != nil {
		t.Fatal(err)
	}
	cursor.offset++
	if _, err = s.AdapterHistoryPage(a.ID, adapterHistoryToken(cursor, key), key, now); !errors.Is(err, ErrAdapterHistoryCursorInvalid) {
		t.Fatal("signed out-of-range offset accepted", err)
	}
	if _, err = adapterHistoryRowBytes(AdapterHistoryRow{Text: strings.Repeat("x", 65537)}); !errors.Is(err, ErrAdapterHistoryBudgetExceeded) {
		t.Fatal("malformed/future oversized row silently truncated", err)
	}
	if _, err = adapterHistoryRowBytes(AdapterHistoryRow{Artifacts: make([]Artifact, 9)}); !errors.Is(err, ErrAdapterHistoryBudgetExceeded) {
		t.Fatal("malformed/future artifact fanout silently truncated", err)
	}
}

func TestAdapterHistoryRepairCumulativeCanonicalBudgets20261013(t *testing.T) {
	rows := make([]AdapterHistoryRow, 8193)
	for i := range rows {
		id := fmt.Sprintf("request-budget-%04d", i)
		rows[i] = AdapterHistoryRow{ReplyID: "reply-" + id, RequestID: id, SourceMessageID: fmt.Sprintf("source-budget-%04d", i), BotID: "bot-meiiie", Role: "assistant", Status: "completed", Sequence: uint64(i + 1), Artifacts: []Artifact{}, Availability: "available"}
	}
	if _, _, err := adapterHistoryDigest(nil, 8192, [32]byte{}, rows[:8192]); err != nil {
		t.Fatal("exact row budget rejected valid canonical metadata envelope", err)
	}
	if _, _, err := adapterHistoryDigest(nil, 8193, [32]byte{}, rows); !errors.Is(err, ErrAdapterHistoryBudgetExceeded) {
		t.Fatal("row budget silently dropped history", err)
	}
	body := strings.Repeat("x", 65536)
	for i := 0; i < 1040; i++ {
		rows[i].Text = body
	}
	if _, _, err := adapterHistoryDigest(nil, 1000, [32]byte{}, rows[:1000]); err != nil {
		t.Fatal("legitimate payload below 64MiB rejected", err)
	}
	if _, _, err := adapterHistoryDigest(nil, 1040, [32]byte{}, rows[:1040]); !errors.Is(err, ErrAdapterHistoryBudgetExceeded) {
		t.Fatal("legitimate cumulative payload beyond 64MiB silently truncated", err)
	}
	t.Log("Canonical DTO budget envelope only; genuine persisted lag/513-row/byte journeys are separate fixtures")
}

func TestAdapterHistoryRepairLargestCurrentRowFitsPage20261013(t *testing.T) {
	s, c, _ := productFixture(t)
	a := adapterFixture(t, s, c, "history-repair-largest-session")
	w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: "history-repair-largest-input", Text: "Owned sizing fixture", Lane: "foreground"}, SourceMessageID: strings.Repeat("m", 160)})
	if err != nil {
		t.Fatal(err)
	}
	w = completeChannelWork(t, s, w, strings.Repeat("\x00", 65536))
	row := AdapterReply{ReplyID: "reply-" + w.RequestID, RequestID: w.RequestID, SourceMessageID: w.Adapter.SourceMessageID, BotID: w.BotID, Role: "assistant", Text: w.Reply, Status: w.Status, Sequence: s.Snapshot().Sequence}
	for i := 0; i < 8; i++ {
		row.Artifacts = append(row.Artifacts, Artifact{ID: fmt.Sprintf("%064x", i), Filename: strings.Repeat("&", 76) + ".txt", SHA256: strings.Repeat("a", 64), Bytes: 24000})
	}
	encoded, err := json.Marshal(row)
	if err != nil || len(encoded) >= 890000 {
		t.Fatal("largest current row sizing envelope escaped page bound", len(encoded), err)
	}
	three, err := json.Marshal([]AdapterReply{row, row, row})
	if err != nil || len(three) <= 900000 {
		t.Fatal("legitimate escaped-text packing fixture did not require split pages", len(three), err)
	}
	t.Logf("real legal 65536-byte reply plus eight-artifact sizing envelope: row=%d JSON bytes; three=%d; metadata-only artifact envelope, no tool execution", len(encoded), len(three))
}
