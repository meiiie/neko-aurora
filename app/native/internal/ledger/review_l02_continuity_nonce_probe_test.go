package ledger

import (
	"errors"
	"testing"
)

func TestIndependentL02ProjectContinuityStateNonceFence(t *testing.T) {
	for _, direction := range []string{"state_then_continuity", "continuity_then_state"} {
		t.Run(direction, func(t *testing.T) {
			s, c, p := productFixture(t)
			nonce := "independent-project-metadata-cross-kind"
			lifecycle := ProjectControlInput{ProjectID: p.ID, Action: "archive", ClientNonce: nonce}
			continuity := ProjectContinuityInput{ProjectID: p.ID, ChannelID: c.ID, ExpectedProjectRevision: p.Revision, ExpectedChannelRevision: c.Revision, ClientNonce: nonce}
			var before uint64
			var err error
			if direction == "state_then_continuity" {
				if _, _, err = s.ControlProject(lifecycle); err != nil {
					t.Fatal("control: project archive rejected", err)
				}
				before = s.Snapshot().Sequence
				_, _, err = s.SaveProjectContinuity(continuity)
			} else {
				if _, _, err = s.SaveProjectContinuity(continuity); err != nil {
					t.Fatal("control: valid continuity rejected", err)
				}
				before = s.Snapshot().Sequence
				_, _, err = s.ControlProject(lifecycle)
			}
			if !errors.Is(err, ErrConflict) {
				t.Error("project metadata command nonce reused across state/continuity kinds", err)
			}
			if s.Snapshot().Sequence != before {
				t.Errorf("wrong-kind project metadata command changed journal: before=%d after=%d", before, s.Snapshot().Sequence)
			}
		})
	}
}
