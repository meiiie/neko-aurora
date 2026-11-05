package ledger

// Direct runs are owned by Aurora's control plane. Engine=core projects the
// hosted Core agent loop; an empty engine retains the legacy controller. Tool
// effects require Aurora's existing capability checks. A direct run never
// creates grants, binds an executor lease or sets Work.Core; the separate
// attested host/executor pipeline retains its own events and authority.
//
// Every state change is a journaled, replay-validated event on the existing
// Work record, so the desktop and the mobile UI read the same task, reply,
// plan and approval.

import (
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type DirectStep struct {
	Title  string `json:"title"`
	Status string `json:"status"`         // pending|running|done|waiting|failed|cancelled
	Note   string `json:"note,omitempty"` // short milestone update once the step is done
}

// DirectPartial keeps visible answer text superseded by an owner interjection.
// It is conversation history, never a completed result or an executable directive.
type DirectPartial struct {
	Text            string `json:"text"`
	At              string `json:"at"`
	BeforeRequestID string `json:"beforeRequestId"`
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
	// Use is the server-side Vault action (JSON: item, url or cmd) run after
	// approval; it names an item, never a value.
	Use string `json:"use,omitempty"`
}

type DirectRun struct {
	Engine     string            `json:"engine,omitempty"` // "" (legacy controller) | core
	Segments   []DirectSegment   `json:"segments,omitempty"`
	Completion *DirectCompletion `json:"completion,omitempty"`
	Version    int               `json:"version"`
	Runtime    string            `json:"runtime"` // neko-core-cli|direct-http|imported
	Model      string            `json:"model,omitempty"`
	Kind       string            `json:"kind,omitempty"` // ""|brief|note|computer
	StartedAt  string            `json:"startedAt"`
	FinishedAt string            `json:"finishedAt,omitempty"`
	ImportedAt string            `json:"importedAt,omitempty"`
	PlanIntro  string            `json:"planIntro,omitempty"`
	Ack        string            `json:"ack,omitempty"` // short acknowledgement shown before the result
	Plan       []DirectStep      `json:"plan,omitempty"`
	Choices    []string          `json:"choices,omitempty"`
	Questions  []DirectQuestion  `json:"questions,omitempty"`
	Form       *DirectForm       `json:"form,omitempty"`
	Items      []string          `json:"items,omitempty"`
	Approval   *DirectApproval   `json:"approval,omitempty"`
	Secret     *DirectSecret     `json:"secret,omitempty"`
	Tools      []DirectTool      `json:"tools,omitempty"`
	Sources    []DirectSource    `json:"sources,omitempty"`
	// LeaseUntil is the heartbeat deadline of the process running this turn
	// (lease pattern from CopilotKit/openmuse worker.ts, MIT).
	LeaseUntil string `json:"leaseUntil,omitempty"`
	// Steered lists queued owner messages folded into this run mid-turn.
	Steered  []string        `json:"steered,omitempty"`
	Partials []DirectPartial `json:"partials,omitempty"`
}

type DirectSegment struct {
	ID           string `json:"id"`
	Text         string `json:"text"`
	At           string `json:"at"`
	AfterInputID string `json:"afterInputId,omitempty"`
}

type DirectCompletion struct {
	Status   string   `json:"status"` // ready|waiting|incomplete, independent of a completed chat turn
	Revision string   `json:"revision"`
	Summary  string   `json:"summary"`
	Evidence []string `json:"evidence"`
}

// DirectTool is the presentation record of an Aurora tool call during the turn.
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

const MaxDirectTools = 10
const MaxCoreTools = 64

func validDirectTools(d *DirectRun) bool {
	limit := MaxDirectTools
	if d.Engine == "core" {
		limit = MaxCoreTools
	}
	if len(d.Tools) > limit || len(d.Sources) > 16 {
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
	// Vault asks for a typed Vault item (card|login|identity) instead of one
	// named secret; Site pre-fills a login's site.
	Vault string `json:"vault,omitempty"`
	Site  string `json:"site,omitempty"`
}

var directSecretName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)

func validDirectSecret(x *DirectSecret) bool {
	return x == nil || directSecretName.MatchString(x.Name) && directText(x.Label, 160) && directText(x.Desc, 640) && (x.Vault == "" || x.Vault == "card" || x.Vault == "login" || x.Vault == "identity") && directText(x.Site, 200)
}

func copyDirectRun(d DirectRun) DirectRun {
	d.Segments = append([]DirectSegment(nil), d.Segments...)
	if d.Completion != nil {
		c := *d.Completion
		c.Evidence = append([]string(nil), c.Evidence...)
		d.Completion = &c
	}
	d.Partials = append([]DirectPartial(nil), d.Partials...)
	d.Plan = append([]DirectStep(nil), d.Plan...)
	d.Choices = append([]string(nil), d.Choices...)
	d.Questions = CopyDirectQuestions(d.Questions)
	d.Form = CopyDirectForm(d.Form)
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
	if !directApprovalState[a.State] || strings.TrimSpace(a.Body) == "" || !directText(a.Title, 400) || !directText(a.Kind, 48) || !directText(a.Body, 16384) || !directText(a.To, 320) || !directText(a.Subject, 640) || !directText(a.Outcome, 2000) || !directText(a.Use, 4000) {
		return false
	}
	if a.Delivery != nil && (a.State != "approved" || !directDeliveryStatus[a.Delivery.Status] || !directText(a.Delivery.Detail, 640) || !directText(a.Delivery.From, 320)) {
		return false
	}
	return true
}

func validDirectRun(d *DirectRun) bool {
	if d != nil && !validCorePresentation(d) {
		return false
	}
	if d != nil && !ValidDirectForm(d.Form) {
		return false
	}
	if d != nil && (!ValidDirectQuestions(d.Questions) || len(d.Questions) > 0 && len(d.Choices) > 0) {
		return false
	}
	if d == nil || d.Version != 1 || (d.Runtime != "neko-core-cli" && d.Runtime != "direct-http" && d.Runtime != "imported") || (d.Kind != "" && d.Kind != "brief" && d.Kind != "note" && d.Kind != "computer" && d.Kind != "handoff") {
		return false
	}
	if !directText(d.Model, 128) || d.StartedAt == "" || !directText(d.PlanIntro, 2000) || !directText(d.Ack, 600) || len(d.Plan) > 8 || len(d.Steered) > MaxSteered || len(d.Choices) > 4 || len(d.Items) > 8 {
		return false
	}
	if len(d.Partials) > MaxSteered {
		return false
	}
	for _, p := range d.Partials {
		if strings.TrimSpace(p.Text) == "" || !directText(p.Text, 60000) || p.BeforeRequestID == "" || !directText(p.BeforeRequestID, 128) {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, p.At); err != nil {
			return false
		}
	}
	for _, st := range d.Plan {
		if !directStepStatus[st.Status] || !directText(st.Title, 480) || !directText(st.Note, 1600) {
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
	return a != nil && b != nil && a.Engine == b.Engine && a.Version == b.Version && a.Runtime == b.Runtime && a.Model == b.Model && a.Kind == b.Kind && a.StartedAt == b.StartedAt && a.ImportedAt == b.ImportedAt
}

func validCorePresentation(d *DirectRun) bool {
	if d.Engine == "core" && d.Runtime != "neko-core-cli" || d.Engine == "" && (len(d.Segments) > 0 || d.Completion != nil) {
		return false
	}
	if d.Engine != "" && d.Engine != "core" || len(d.Segments) > 128 {
		return false
	}
	ids, total := map[string]bool{}, 0
	for _, segment := range d.Segments {
		if segment.AfterInputID != "" && !slices.Contains(d.Steered, segment.AfterInputID) {
			return false
		}
		if segment.ID == "" || ids[segment.ID] || !directText(segment.ID, 128) || !directText(segment.Text, 60000) {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, segment.At); err != nil {
			return false
		}
		ids[segment.ID] = true
		total += len(segment.Text)
	}
	if total > 240000 {
		return false
	}
	if c := d.Completion; c != nil {
		if c.Status != "ready" && c.Status != "waiting" && c.Status != "incomplete" {
			return false
		}
		if c.Revision == "" || !directText(c.Revision, 128) || !directText(c.Summary, 32000) || len(c.Evidence) > 64 {
			return false
		}
		for _, evidence := range c.Evidence {
			if !directText(evidence, 4000) {
				return false
			}
		}
	}
	return true
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
	if w.Direct.Form != nil && (typ == "work.direct_started" || typ == "work.direct_updated" || typ == "work.direct_finished" && w.Status != "completed") {
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

// MaxDirectResumes bounds automatic restarts of one direct run so a turn
// that crashes the process cannot loop forever.
const MaxDirectResumes = 2

// MaxSteered bounds owner messages one run can absorb mid-turn.
const MaxSteered = 8

// DirectResume puts an unfinished direct run (its process died or its lease
// expired) back in the queue: the partial run and reply are dropped and the
// work restarts from waiting_host on the next dispatch.
func (s *Store) DirectResume(id string) (Work, error) {
	cur, err := s.Get(id)
	if err != nil {
		return Work{}, err
	}
	return s.update(id, cur.Epoch, "work.direct_resumed", func(w *Work) error {
		if w.Status != "running" || w.Direct == nil || w.Direct.FinishedAt != "" || w.DesiredState != "running" || w.DirectResumes >= MaxDirectResumes {
			return ErrConflict
		}
		w.ResumeFrom = ResumeCheckpoint(w.Direct, w.ResumeFrom)
		w.Status, w.Direct, w.Reply, w.AttemptID, w.Failure = "waiting_host", nil, "", "", ""
		w.DirectResumes++
		return nil
	})
}

func validDirectResumed(old, w Work) bool {
	return old.Status == "running" && old.Direct != nil && old.Direct.FinishedAt == "" && old.DesiredState == "running" && old.Core == nil &&
		w.Status == "waiting_host" && w.Direct == nil && w.Reply == "" && w.AttemptID == "" && w.Failure == "" &&
		w.Epoch == old.Epoch && w.DesiredState == old.DesiredState && w.DirectResumes == old.DirectResumes+1 && w.DirectResumes <= MaxDirectResumes
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
	return s.DirectUpdateAttempt(id, epoch, "", change)
}

// An embedded child must not update a replacement attempt with the same epoch.
func (s *Store) DirectUpdateAttempt(id string, epoch uint64, attempt string, change func(*DirectRun)) (Work, error) {
	return s.update(id, epoch, "work.direct_updated", func(w *Work) error {
		if w.Status != "running" || w.Direct == nil || attempt != "" && (w.AttemptID != attempt || w.DesiredState != "running") {
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

// ResumeCheckpoint keeps what an interrupted run already finished (plan
// steps marked done with their notes, and the sources they cite) so the
// resumed run continues from the next step instead of starting over. Runs
// without a finished plan step keep the previous checkpoint (or none).
// Deterministic: replay recomputes it from the journaled run.
func ResumeCheckpoint(d, prev *DirectRun) *DirectRun {
	if d == nil || len(d.Plan) < 2 {
		return prev
	}
	cp := DirectRun{PlanIntro: d.PlanIntro, Plan: make([]DirectStep, len(d.Plan))}
	done := 0
	for i, st := range d.Plan {
		cp.Plan[i] = DirectStep{Title: st.Title, Status: "pending"}
		if st.Status == "done" && done == i { // only the finished prefix
			cp.Plan[i] = st
			done++
		}
	}
	if done == 0 {
		return prev
	}
	cp.Sources = append([]DirectSource(nil), d.Sources...)
	cp = copyDirectRun(cp)
	return &cp
}
