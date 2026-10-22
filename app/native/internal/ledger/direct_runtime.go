package ledger

// Direct runs: a Work item executed in-process by the owner's own control
// plane (the mobile/Aurora runner) through Neko Core's one-shot CLI
// (`neko run --profile zai --no-tools`) or, when Core is not installed, the
// same provider over HTTP. A direct run never grants tools, never binds an
// executor lease and never sets Work.Core, so it cannot be confused with the
// attested host/executor pipeline: those events stay exactly as before.
//
// Every state change is a journaled, replay-validated event on the existing
// Work record, so the desktop and the mobile UI read the same task, reply,
// plan and approval.

import (
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type DirectStep struct {
	Title  string `json:"title"`
	Status string `json:"status"` // pending|running|done|waiting|failed|cancelled
}

type DirectDelivery struct {
	Status string `json:"status"` // queued|sending|sent|failed
	Detail string `json:"detail,omitempty"`
	From   string `json:"from,omitempty"`
	At     string `json:"at"`
}

// DirectApproval is a human-in-the-loop gate: nothing leaves the machine
// until the owner decides, and only the owner's recorded decision moves it.
type DirectApproval struct {
	Title     string          `json:"title"`
	Kind      string          `json:"kind"`
	Body      string          `json:"body"`
	To        string          `json:"to,omitempty"`
	Subject   string          `json:"subject,omitempty"`
	State     string          `json:"state"` // pending|approved|declined|superseded
	DecidedAt string          `json:"decidedAt,omitempty"`
	Delivery  *DirectDelivery `json:"delivery,omitempty"`
	Outcome   string          `json:"outcome,omitempty"`
}

type DirectRun struct {
	Version    int             `json:"version"`
	Runtime    string          `json:"runtime"` // neko-core-cli|direct-http|imported
	Model      string          `json:"model,omitempty"`
	Kind       string          `json:"kind,omitempty"` // ""|brief|note
	StartedAt  string          `json:"startedAt"`
	FinishedAt string          `json:"finishedAt,omitempty"`
	ImportedAt string          `json:"importedAt,omitempty"`
	PlanIntro  string          `json:"planIntro,omitempty"`
	Ack        string          `json:"ack,omitempty"` // short acknowledgement shown before the result
	Plan       []DirectStep    `json:"plan,omitempty"`
	Choices    []string        `json:"choices,omitempty"`
	Items      []string        `json:"items,omitempty"`
	Approval   *DirectApproval `json:"approval,omitempty"`
	Secret     *DirectSecret   `json:"secret,omitempty"`
	Tools      []DirectTool    `json:"tools,omitempty"`
	Sources    []DirectSource  `json:"sources,omitempty"`
}

// DirectTool is one read-only data tool call made during the turn.
type DirectTool struct {
	Name    string `json:"name"`
	Arg     string `json:"arg,omitempty"`
	Status  string `json:"status"` // running|done|failed
	Summary string `json:"summary,omitempty"`
	Ms      int    `json:"ms,omitempty"`
}

// DirectSource is a cited source for the reply.
type DirectSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	At    string `json:"at,omitempty"`
}

var directToolStatus = map[string]bool{"running": true, "done": true, "failed": true}

func validDirectTools(d *DirectRun) bool {
	if len(d.Tools) > 6 || len(d.Sources) > 16 {
		return false
	}
	for _, t := range d.Tools {
		if !directToolStatus[t.Status] || t.Name == "" || !directText(t.Name, 32) || !directText(t.Arg, 400) || !directText(t.Summary, 200) || t.Ms < 0 {
			return false
		}
	}
	for _, x := range d.Sources {
		if !directText(x.Title, 200) || !directText(x.At, 40) || !directText(x.URL, 600) || !strings.HasPrefix(x.URL, "https://") {
			return false
		}
	}
	return true
}

// DirectSecret is a bot's request for a secret. It carries only the vault
// name and copy for the masked input card; the value never enters the ledger.
type DirectSecret struct {
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
	Desc  string `json:"desc,omitempty"`
}

var directSecretName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)

func validDirectSecret(x *DirectSecret) bool {
	return x == nil || directSecretName.MatchString(x.Name) && directText(x.Label, 160) && directText(x.Desc, 640)
}

func copyDirectRun(d DirectRun) DirectRun {
	d.Plan = append([]DirectStep(nil), d.Plan...)
	d.Choices = append([]string(nil), d.Choices...)
	d.Items = append([]string(nil), d.Items...)
	if d.Approval != nil {
		a := *d.Approval
		if a.Delivery != nil {
			x := *a.Delivery
			a.Delivery = &x
		}
		d.Approval = &a
	}
	if d.Secret != nil {
		x := *d.Secret
		d.Secret = &x
	}
	d.Tools = append([]DirectTool(nil), d.Tools...)
	d.Sources = append([]DirectSource(nil), d.Sources...)
	return d
}

func directText(v string, n int) bool { return utf8.ValidString(v) && len(v) <= n }

var directStepStatus = map[string]bool{"pending": true, "running": true, "done": true, "waiting": true, "failed": true, "cancelled": true}
var directApprovalState = map[string]bool{"pending": true, "approved": true, "declined": true, "superseded": true}
var directDeliveryStatus = map[string]bool{"queued": true, "sending": true, "sent": true, "failed": true}

func validDirectApproval(a *DirectApproval) bool {
	if a == nil {
		return true
	}
	if !directApprovalState[a.State] || strings.TrimSpace(a.Body) == "" || !directText(a.Title, 400) || !directText(a.Kind, 48) || !directText(a.Body, 16384) || !directText(a.To, 320) || !directText(a.Subject, 640) || !directText(a.Outcome, 2000) {
		return false
	}
	if a.Delivery != nil && (a.State != "approved" || !directDeliveryStatus[a.Delivery.Status] || !directText(a.Delivery.Detail, 640) || !directText(a.Delivery.From, 320)) {
		return false
	}
	return true
}

func validDirectRun(d *DirectRun) bool {
	if d == nil || d.Version != 1 || (d.Runtime != "neko-core-cli" && d.Runtime != "direct-http" && d.Runtime != "imported") || (d.Kind != "" && d.Kind != "brief" && d.Kind != "note") {
		return false
	}
	if !directText(d.Model, 128) || d.StartedAt == "" || !directText(d.PlanIntro, 2000) || !directText(d.Ack, 600) || len(d.Plan) > 8 || len(d.Choices) > 4 || len(d.Items) > 8 {
		return false
	}
	for _, st := range d.Plan {
		if !directStepStatus[st.Status] || !directText(st.Title, 480) {
			return false
		}
	}
	for _, c := range append(append([]string(nil), d.Choices...), d.Items...) {
		if !directText(c, 640) {
			return false
		}
	}
	return validDirectApproval(d.Approval) && validDirectSecret(d.Secret) && validDirectTools(d)
}

func sameDirectIdentity(a, b *DirectRun) bool {
	return a != nil && b != nil && a.Version == b.Version && a.Runtime == b.Runtime && a.Model == b.Model && a.Kind == b.Kind && a.StartedAt == b.StartedAt && a.ImportedAt == b.ImportedAt
}

// validDirectDecision allows only owner-driven approval transitions.
func validDirectDecision(old, next *DirectApproval) bool {
	if old == nil || next == nil || old.Title != next.Title || old.Kind != next.Kind || old.To != next.To {
		return false
	}
	switch old.State {
	case "pending":
		return next.State == "approved" || next.State == "declined" || next.State == "superseded"
	case "approved":
		// delivery progress after approval; the drafted content is frozen
		return next.State == "approved" && next.Body == old.Body && next.Subject == old.Subject && next.DecidedAt == old.DecidedAt && next.Delivery != nil
	}
	return false
}

func validDirectReplay(old, w Work, typ string) bool {
	if !validDirectRun(w.Direct) || w.Core != nil || old.Core != nil || w.Epoch != old.Epoch || w.DesiredState != old.DesiredState {
		return false
	}
	switch typ {
	case "work.direct_started":
		if old.Direct != nil || old.Status != "waiting_host" || old.DesiredState != "running" || w.Status != "running" || !strings.HasPrefix(w.AttemptID, "direct-") || w.Reply != old.Reply || w.Direct.FinishedAt != "" {
			return false
		}
		return w.Direct.Approval == nil || w.Direct.Runtime == "imported"
	case "work.direct_updated", "work.direct_finished":
		if old.Status != "running" || !sameDirectIdentity(old.Direct, w.Direct) || w.Reply != old.Reply {
			return false
		}
		if old.Direct.Approval != nil && !reflect.DeepEqual(old.Direct.Approval, w.Direct.Approval) {
			return false
		}
		if old.Direct.Approval == nil && w.Direct.Approval != nil && w.Direct.Approval.State != "pending" && w.Direct.Runtime != "imported" {
			return false
		}
		if typ == "work.direct_updated" {
			return w.Status == "running" && w.Direct.FinishedAt == "" && w.Failure == old.Failure
		}
		return (w.Status == "completed" || w.Status == "failed") && w.Direct.FinishedAt != "" && directText(w.Failure, 160)
	case "work.direct_decided":
		if old.Direct == nil || old.Status != w.Status || w.Reply != old.Reply || w.Failure != old.Failure || !sameDirectIdentity(old.Direct, w.Direct) {
			return false
		}
		a, b := copyDirectRun(*old.Direct), copyDirectRun(*w.Direct)
		if !validDirectDecision(a.Approval, b.Approval) {
			return false
		}
		a.Approval, b.Approval = nil, nil
		// superseding may also close a plan that was paused waiting on the owner
		a.Plan, b.Plan = nil, nil
		return reflect.DeepEqual(a, b)
	}
	return false
}

func directNow() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// DirectStart moves accepted work into an in-process direct run.
func (s *Store) DirectStart(id string, epoch uint64, run DirectRun) (Work, error) {
	attempt, err := newID("direct-")
	if err != nil {
		return Work{}, err
	}
	run.Version = 1
	if run.StartedAt == "" {
		run.StartedAt = directNow()
	}
	return s.update(id, epoch, "work.direct_started", func(w *Work) error {
		if w.Status != "waiting_host" || w.DesiredState != "running" || w.Direct != nil || w.Core != nil {
			return ErrConflict
		}
		d := copyDirectRun(run)
		w.Direct, w.Status, w.AttemptID = &d, "running", attempt
		return nil
	})
}

// DirectUpdate changes plan/choices/approval presentation of a running direct turn.
func (s *Store) DirectUpdate(id string, epoch uint64, change func(*DirectRun)) (Work, error) {
	return s.update(id, epoch, "work.direct_updated", func(w *Work) error {
		if w.Status != "running" || w.Direct == nil {
			return ErrConflict
		}
		d := copyDirectRun(*w.Direct)
		change(&d)
		w.Direct = &d
		return nil
	})
}

// DirectFinish closes a running direct turn as completed or failed.
func (s *Store) DirectFinish(id string, epoch uint64, status, failure string, change func(*DirectRun)) (Work, error) {
	if status != "completed" && status != "failed" {
		return Work{}, ErrInvalid
	}
	return s.update(id, epoch, "work.direct_finished", func(w *Work) error {
		if w.Status != "running" || w.Direct == nil {
			return ErrConflict
		}
		d := copyDirectRun(*w.Direct)
		if change != nil {
			change(&d)
		}
		d.FinishedAt = directNow()
		w.Direct, w.Status, w.Failure = &d, status, failure
		return nil
	})
}

// DirectStopped finalises a direct run the owner cancelled (status cancelling).
func (s *Store) DirectStopped(id string) (Work, error) {
	cur, err := s.Get(id)
	if err != nil {
		return Work{}, err
	}
	return s.update(id, cur.Epoch, "work.cancelled", func(w *Work) error {
		if w.Status != "cancelling" || w.Direct == nil {
			return ErrConflict
		}
		w.Status, w.Failure = "cancelled", "stopped_by_owner"
		return nil
	})
}

// DirectDecide records an owner decision (or delivery progress) on the
// approval gate of a finished direct turn.
func (s *Store) DirectDecide(id string, change func(*DirectApproval, *DirectRun) error) (Work, error) {
	cur, err := s.Get(id)
	if err != nil {
		return Work{}, err
	}
	return s.update(id, cur.Epoch, "work.direct_decided", func(w *Work) error {
		if w.Direct == nil || w.Direct.Approval == nil {
			return ErrNotFound
		}
		d := copyDirectRun(*w.Direct)
		if err := change(d.Approval, &d); err != nil {
			return err
		}
		if !validDirectDecision(w.Direct.Approval, d.Approval) {
			return ErrConflict
		}
		w.Direct = &d
		return nil
	})
}

func ValidDirectSecret(x *DirectSecret) bool { return x != nil && validDirectSecret(x) }
