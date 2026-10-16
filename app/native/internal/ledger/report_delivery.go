package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxReportDeliveries = 512
const maxReportCandidates = 128

type ReportDeliveryInput struct {
	BotID                       string `json:"botId"`
	DestinationChannelID        string `json:"destinationChannelId"`
	ExpectedDestinationRevision uint64 `json:"expectedDestinationRevision"`
	ClientNonce                 string `json:"clientNonce"`
	Text                        string `json:"text"`
	ClarificationOf             string `json:"clarificationOf,omitempty"`
}

type ReportSourcePin struct {
	RequestID            string     `json:"requestId"`
	ConversationID       string     `json:"conversationId"`
	MessageID            string     `json:"messageId"`
	Epoch                uint64     `json:"epoch"`
	AccessRevision       uint64     `json:"accessRevision"`
	ConversationRevision uint64     `json:"conversationRevision"`
	BotStateRevision     uint64     `json:"botStateRevision"`
	ResultSHA256         string     `json:"resultSha256"`
	Artifacts            []Artifact `json:"artifacts,omitempty"`
}

type ReportDelivery struct {
	Version                     int              `json:"version"`
	ID                          string           `json:"id"`
	SourceMessageID             string           `json:"sourceMessageId"`
	OwnerUserID                 string           `json:"ownerUserId"`
	SpaceID                     string           `json:"spaceId"`
	BotID                       string           `json:"botId"`
	DestinationChannelID        string           `json:"destinationChannelId"`
	ExpectedDestinationRevision uint64           `json:"expectedDestinationRevision"`
	ClientNonce                 string           `json:"clientNonce"`
	Text                        string           `json:"text"`
	ClarificationOf             string           `json:"clarificationOf,omitempty"`
	Status                      string           `json:"status"`
	CreatedAt                   string           `json:"createdAt"`
	CreatedSequence             uint64           `json:"createdSequence"`
	Reply                       string           `json:"reply,omitempty"`
	Question                    string           `json:"question,omitempty"`
	Artifacts                   []Artifact       `json:"artifacts,omitempty"`
	Source                      *ReportSourcePin `json:"source,omitempty"`
	Active                      bool             `json:"active"`
	AnsweredBy                  string           `json:"answeredBy,omitempty"`
}

type ReportDeliveryReceipt struct {
	Version  int                  `json:"version"`
	Found    bool                 `json:"found"`
	Input    *ReportDeliveryInput `json:"input,omitempty"`
	Delivery *ReportDelivery      `json:"delivery,omitempty"`
	Active   *bool                `json:"active,omitempty"`
}

func reportInput(d ReportDelivery) ReportDeliveryInput {
	return ReportDeliveryInput{BotID: d.BotID, DestinationChannelID: d.DestinationChannelID, ExpectedDestinationRevision: d.ExpectedDestinationRevision, ClientNonce: d.ClientNonce, Text: d.Text, ClarificationOf: d.ClarificationOf}
}

func copyReportDelivery(d ReportDelivery) ReportDelivery {
	d.Artifacts = append([]Artifact(nil), d.Artifacts...)
	if d.Source != nil {
		p := *d.Source
		p.Artifacts = append([]Artifact(nil), p.Artifacts...)
		d.Source = &p
	}
	return d
}

func reportFold(text string) string {
	const from = "àáạảãâầấậẩẫăằắặẳẵèéẹẻẽêềếệểễìíịỉĩòóọỏõôồốộổỗơờớợởỡùúụủũưừứựửữỳýỵỷỹđ"
	const to = "aaaaaaaaaaaaaaaaaeeeeeeeeeeeiiiiiooooooooooooooooouuuuuuuuuuuyyyyyd"
	replacements := []string{}
	for i, r := range []rune(from) {
		replacements = append(replacements, string(r), string([]rune(to)[i]))
	}
	return strings.Join(strings.Fields(strings.NewReplacer(replacements...).Replace(strings.ToLower(text))), " ")
}

type reportIntent struct {
	subject []string
	latest  bool
	files   bool
}

func (s *Store) reportIntent(in ReportDeliveryInput) (reportIntent, error) {
	return s.reportIntentDepth(in, 0)
}

func (s *Store) reportIntentDepth(in ReportDeliveryInput, depth int) (reportIntent, error) {
	return s.reportIntentVersion(in, depth, false)
}

func (s *Store) reportIntentVersion(in ReportDeliveryInput, depth int, historical bool) (reportIntent, error) {
	var out reportIntent
	if !validAccessNonce(in.ClientNonce) || !validChannelID(in.DestinationChannelID) || in.ExpectedDestinationRevision == 0 || in.ExpectedDestinationRevision >= 1<<53 || !utf8.ValidString(in.Text) || len(in.Text) == 0 || len(in.Text) > 4096 || strings.IndexFunc(in.Text, unicode.IsControl) >= 0 || hasSecret([]byte(in.Text)) {
		return out, ErrInvalid
	}
	if depth > 8 {
		return out, ErrInvalid
	}
	if in.ClarificationOf != "" {
		prior, ok := s.reportDeliveries[in.ClarificationOf]
		active := s.reportDeliveryActive(prior)
		if historical {
			active = s.reportDeliveryAuthorityActive(prior)
		}
		if !ok || prior.Status != "needs_clarification" || prior.OwnerUserID != "local-owner" || prior.SpaceID != "local-space" || prior.BotID != in.BotID || prior.DestinationChannelID != in.DestinationChannelID || prior.ExpectedDestinationRevision != in.ExpectedDestinationRevision || depth == 0 && !active {
			return out, ErrConflict
		}
		out, err := s.reportIntentVersion(reportInput(prior), depth+1, historical)
		if err != nil {
			return out, err
		}
		answer := reportFold(strings.TrimSpace(in.Text))
		if len(in.Text) > 256 || answer == "" || strings.IndexFunc(in.Text, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r) && r != '-'
		}) >= 0 {
			return out, ErrInvalid
		}
		answer = strings.TrimPrefix(answer, "bao cao ")
		answer = strings.TrimPrefix(answer, "report ")
		if answer == "moi nhat" || answer == "gan nhat" || answer == "latest" || answer == "most recent" {
			out.latest = true
			return out, nil
		}
		terms := strings.Fields(answer)
		if len(terms) == 0 || len(terms)+len(out.subject) > 12 {
			return out, ErrInvalid
		}
		for _, term := range terms {
			if len(term) > 80 {
				return out, ErrInvalid
			}
			for _, reserved := range []string{"gui", "chuyen", "dua", "send", "share", "publish", "delete", "grant", "cap", "quyen", "huy", "cancel", "bot", "kenh", "channel", "conversation", "private", "tep", "file", "attach", "dinh", "kem", "khong", "not"} {
				if term == reserved {
					return out, ErrInvalid
				}
			}
		}
		out.subject = append(out.subject, terms...)
		return out, nil
	}
	text := strings.TrimSpace(in.Text)
	if strings.HasPrefix(text, "@") {
		alias, tail, found := strings.Cut(text, " ")
		if !found {
			return out, ErrInvalid
		}
		matched := depth > 0
		for _, b := range s.bots {
			if b.ID == in.BotID && strings.EqualFold(alias[1:], b.Alias) {
				matched = true
			}
		}
		if !matched {
			return out, ErrInvalid
		}
		text = strings.TrimSpace(tail)
	}
	text = strings.TrimRight(reportFold(text), ".!?")
	action := false
	for _, prefix := range []string{"gui ", "chuyen ", "dua ", "send ", "share "} {
		if strings.HasPrefix(text, prefix) {
			text = strings.TrimPrefix(text, prefix)
			action = true
			break
		}
	}
	if !action {
		return out, ErrInvalid
	}
	text = strings.TrimPrefix(text, "the ")
	if !strings.HasPrefix(text, "bao cao ") && !strings.HasPrefix(text, "report ") && (!historical || !strings.HasPrefix(text, "bao cao") && !strings.HasPrefix(text, "report")) {
		return out, ErrInvalid
	}
	if strings.HasPrefix(text, "bao cao") {
		text = strings.TrimPrefix(text, "bao cao")
	} else {
		text = strings.TrimPrefix(text, "report")
	}
	privateAt := -1
	privateMarker := ""
	for _, marker := range []string{" o cuoc tro chuyen rieng", " trong cuoc tro chuyen rieng", " tu cuoc tro chuyen rieng", " o hoi thoai rieng", " trong hoi thoai rieng", " tu hoi thoai rieng", " from the private conversation", " from private conversation", " from the private chat", " from private chat"} {
		if at := strings.Index(text, marker); at >= 0 && (privateAt < 0 || at < privateAt) {
			privateAt, privateMarker = at, marker
		}
	}
	if privateAt < 0 {
		return out, ErrInvalid
	}
	tail := strings.TrimSpace(text[privateAt+len(privateMarker):])
	if tail != "vao kenh nay" && tail != "len kenh nay" && tail != "den kenh nay" && tail != "to this channel" && tail != "here" {
		return out, ErrInvalid
	}
	subject := strings.TrimSpace(text[:privateAt])
	if !historical && strings.ContainsAny(subject, ";|&\n\r") {
		return out, ErrInvalid
	}
	for _, word := range strings.FieldsFunc(subject, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		switch word {
		case "khong", "dung", "chua", "huy", "gui", "chuyen", "dua", "not", "no", "never", "dont", "send", "share", "publish", "delete", "cancel", "grant":
			if !historical {
				return out, ErrInvalid
			}
		}
	}
	for _, marker := range []string{"moi nhat", "gan nhat", "latest", "most recent"} {
		if strings.Contains(" "+subject+" ", " "+marker+" ") {
			out.latest = true
			subject = strings.TrimSpace(strings.ReplaceAll(" "+subject+" ", " "+marker+" ", " "))
		}
	}
	for _, marker := range []string{"tep", "file", "tai lieu", "dinh kem"} {
		if strings.Contains(" "+subject+" ", " "+marker+" ") {
			out.files = true
			subject = strings.TrimSpace(strings.ReplaceAll(" "+subject+" ", " "+marker+" ", " "))
		}
	}
	for _, marker := range []string{"da lam", "da hoan thanh", "hoan thanh", "completed", "da", "ve", "the", "about"} {
		fields := strings.Fields(subject)
		if strings.Contains(marker, " ") {
			subject = strings.TrimSpace(strings.ReplaceAll(" "+subject+" ", " "+marker+" ", " "))
			continue
		}
		kept := fields[:0]
		for _, field := range fields {
			if field != marker {
				kept = append(kept, field)
			}
		}
		subject = strings.Join(kept, " ")
	}
	for _, field := range strings.FieldsFunc(subject, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(field) > 80 || len(out.subject) >= 12 {
			return out, ErrInvalid
		}
		out.subject = append(out.subject, field)
	}
	return out, nil
}

func reportResultHash(w Work) string {
	raw, _ := json.Marshal(struct {
		Reply     string
		Artifacts []Artifact
	}{w.Reply, w.Artifacts})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func (s *Store) reportDestinationAllowed(in ReportDeliveryInput) bool {
	c, ok := s.channels[in.DestinationChannelID]
	return ok && c.OwnerUserID == "local-owner" && c.SpaceID == "local-space" && c.Revision == in.ExpectedDestinationRevision && member(c, in.BotID) && !s.archivedBot(in.BotID)
}

func (s *Store) reportSourceAllowed(w Work, bot string) bool {
	c, ok := s.conversation(w.ConversationID)
	return ok && c.BotID == bot && w.BotID == bot && w.SourceBotID == "" && w.OwnerUserID == "local-owner" && w.SpaceID == "local-space" && !validChannelID(w.ConversationID) && w.Status == "completed" && w.Failure == "" && w.Collaboration == nil && operationsComplete(w) && s.contextAllowed(w) && s.resourceAllowed(w)
}

func reportLike(w Work) bool {
	text := reportFold(w.Text + " " + w.Reply)
	if strings.Contains(text, "bao cao") || strings.Contains(text, "report") {
		return true
	}
	for _, a := range w.Artifacts {
		if reportArtifact(a) {
			return true
		}
	}
	return false
}

func reportArtifact(a Artifact) bool {
	name := strings.NewReplacer("_", " ", "-", " ").Replace(reportFold(a.Filename))
	return strings.Contains(name, "report") || strings.Contains(name, "bao cao")
}

func (s *Store) reportCompletionSequences() map[string]uint64 {
	sequences := map[string]uint64{}
	for _, event := range s.events {
		if event.Type != "work.completed" || event.Work.Status != "completed" {
			continue
		}
		current, found := s.work[event.Work.RequestID]
		if found && current.Epoch == event.Work.Epoch {
			sequences[current.RequestID] = event.Sequence
		}
	}
	return sequences
}

func (s *Store) resolveReport(in ReportDeliveryInput, intent reportIntent) (string, string, []Artifact, *ReportSourcePin, error) {
	return s.resolveReportVersion(in, intent, false)
}

func (s *Store) resolveReportVersion(in ReportDeliveryInput, intent reportIntent, historical bool) (string, string, []Artifact, *ReportSourcePin, error) {
	candidates := []Work{}
	eligible := 0
	completions := s.reportCompletionSequences()
	works := s.sortedWorkLocked()
	if historical {
		for _, w := range works {
			completions[w.RequestID] = w.CreatedSequence
		}
	}
	sort.Slice(works, func(i, j int) bool {
		return completions[works[i].RequestID] > completions[works[j].RequestID]
	})
	for _, w := range works {
		if w.BotID != in.BotID || w.OwnerUserID != "local-owner" || w.SpaceID != "local-space" || validChannelID(w.ConversationID) || w.Status != "completed" || completions[w.RequestID] == 0 {
			continue
		}
		eligible++
		if eligible > maxReportCandidates {
			break
		}
		if !s.reportSourceAllowed(w, in.BotID) || !reportLike(w) {
			continue
		}
		text := reportFold(w.Text + " " + w.Reply)
		for _, a := range w.Artifacts {
			text += " " + reportFold(a.Filename)
		}
		matches := true
		for _, term := range intent.subject {
			if !strings.Contains(text, term) {
				matches = false
				break
			}
		}
		if matches {
			candidates = append(candidates, w)
		}
	}
	if len(candidates) == 0 {
		return "", "Chưa xác định được báo cáo trong 128 kết quả riêng gần nhất. Báo cáo về chủ đề nào?", nil, nil, nil
	}
	if len(candidates) > 1 && !intent.latest {
		return "", "Báo cáo về chủ đề nào, hay báo cáo mới nhất?", nil, nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return completions[candidates[i].RequestID] > completions[candidates[j].RequestID]
	})
	w := candidates[0]
	if len(w.Reply) > 65536 || !utf8.ValidString(w.Reply) || hasSecret([]byte(w.Reply)) || len(w.Artifacts) > 8 {
		return "", "", nil, nil, ErrInvalid
	}
	var artifacts []Artifact
	reportFiles := 0
	for _, a := range w.Artifacts {
		if reportArtifact(a) {
			reportFiles++
		}
	}
	for _, a := range w.Artifacts {
		if !lowerHex(a.ID, 64) || !lowerHex(a.SHA256, 64) || a.Bytes < 0 || a.Bytes > 65536 || !botText(a.Filename, 120) || hasSecret([]byte(a.Filename)) {
			return "", "", nil, nil, ErrInvalid
		}
		if reportArtifact(a) && (intent.files || reportFiles == 1) {
			artifacts = append(artifacts, a)
		}
	}
	if intent.files && len(artifacts) == 0 {
		return "", "Báo cáo chưa có tệp kết quả. Bạn muốn gửi nội dung báo cáo nào?", nil, nil, nil
	}
	if w.Reply == "" && len(artifacts) == 0 {
		return "", "Bạn muốn gửi báo cáo nào? Hãy chỉ rõ kết quả đã hoàn thành.", nil, nil, nil
	}
	c, _ := s.conversation(w.ConversationID)
	b, _ := s.botState(in.BotID)
	pin := &ReportSourcePin{RequestID: w.RequestID, ConversationID: w.ConversationID, MessageID: w.SourceMessageID, Epoch: w.Epoch, AccessRevision: accessRevision(w), ConversationRevision: c.Revision, BotStateRevision: b.Revision, ResultSHA256: reportResultHash(w), Artifacts: append([]Artifact(nil), w.Artifacts...)}
	return w.Reply, "", artifacts, pin, nil
}

func eventCommandNonces(e Event) []string {
	out := []string{e.Nonce, e.Work.ClientNonce}
	if e.Work.ContextAccess != nil {
		out = append(out, e.Work.ContextAccess.Nonce)
	}
	if e.TaskControl != nil {
		out = append(out, e.TaskControl.Input.ClientNonce)
		if e.TaskControl.Related != nil {
			out = append(out, e.TaskControl.Related.ClientNonce)
		}
	}
	if e.Plugin != nil {
		out = append(out, e.Plugin.Input.ClientNonce)
	}
	return out
}

func (s *Store) reportOtherNonceUsed(nonce string) bool {
	for _, e := range s.events {
		for _, prior := range eventCommandNonces(e) {
			if prior == nonce {
				return true
			}
		}
	}
	return false
}

func (s *Store) reportEventNonceUsed(e Event) bool {
	for _, nonce := range eventCommandNonces(e) {
		if _, ok := s.reportDeliveryNonces[nonce]; ok {
			return true
		}
	}
	return false
}

func (s *Store) validReportDeliveryEvent(e Event) bool {
	if e.ReportDelivery == nil {
		return false
	}
	d := *e.ReportDelivery
	if e.ReportDeliverySelectionVersion != 0 && e.ReportDeliverySelectionVersion != 2 {
		return false
	}
	if !reflect.DeepEqual(e, Event{Sequence: e.Sequence, Type: "report.delivered", ReportDelivery: &d, Nonce: d.ClientNonce, ReportDeliverySelectionVersion: e.ReportDeliverySelectionVersion}) || d.Version != 1 || !resourceID(d.ID, "report-delivery-") || !resourceID(d.SourceMessageID, "message-") || d.OwnerUserID != "local-owner" || d.SpaceID != "local-space" || d.CreatedSequence != e.Sequence || !d.Active || d.AnsweredBy != "" || len(s.reportDeliveryNonces) >= maxReportDeliveries || s.reportOtherNonceUsed(d.ClientNonce) {
		return false
	}
	if _, exists := s.reportDeliveries[d.ID]; exists {
		return false
	}
	for _, prior := range s.reportDeliveries {
		if prior.SourceMessageID == d.SourceMessageID {
			return false
		}
	}
	for _, prior := range s.work {
		if prior.SourceMessageID == d.SourceMessageID {
			return false
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, d.CreatedAt); err != nil {
		return false
	}
	historical := e.ReportDeliverySelectionVersion == 0
	intent, err := s.reportIntentVersion(reportInput(d), 0, historical)
	if err != nil || !s.reportDestinationAllowed(reportInput(d)) {
		return false
	}
	reply, question, artifacts, source, err := s.resolveReportVersion(reportInput(d), intent, historical)
	status := "delivered"
	if question != "" {
		status = "needs_clarification"
	}
	return err == nil && d.Status == status && d.Reply == reply && d.Question == question && reflect.DeepEqual(d.Artifacts, artifacts) && reflect.DeepEqual(d.Source, source)
}

func (s *Store) reportDeliveryActive(d ReportDelivery) bool {
	if _, err := s.reportIntentDepth(reportInput(d), 1); err != nil {
		return false
	}
	return s.reportDeliveryAuthorityActive(d)
}

func (s *Store) reportDeliveryAuthorityActive(d ReportDelivery) bool {
	if !s.reportDestinationAllowed(reportInput(d)) {
		return false
	}
	if d.Status == "needs_clarification" {
		for _, child := range s.reportDeliveries {
			if child.ClarificationOf == d.ID {
				return false
			}
		}
		return true
	}
	if d.Source == nil {
		return false
	}
	w, ok := s.work[d.Source.RequestID]
	b, _ := s.botState(d.BotID)
	return ok && s.reportSourceAllowed(w, d.BotID) && w.ConversationID == d.Source.ConversationID && w.SourceMessageID == d.Source.MessageID && w.Epoch == d.Source.Epoch && accessRevision(w) == d.Source.AccessRevision && b.Revision == d.Source.BotStateRevision && reportResultHash(w) == d.Source.ResultSHA256 && reflect.DeepEqual(w.Artifacts, d.Source.Artifacts)
}

func (s *Store) publicReportDelivery(d ReportDelivery) ReportDelivery {
	d = copyReportDelivery(d)
	d.Active = s.reportDeliveryActive(d)
	if d.Status == "needs_clarification" {
		for _, child := range s.reportDeliveries {
			if child.ClarificationOf == d.ID {
				d.AnsweredBy = child.ID
				break
			}
		}
	}
	if d.Source != nil {
		d.Source.Artifacts = nil
	}
	if !d.Active {
		d.Reply = ""
		d.Artifacts = nil
	}
	return d
}

func (s *Store) DeliverReport(in ReportDeliveryInput) (ReportDelivery, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deliverReportLocked(in)
}
func (s *Store) deliverReportLocked(in ReportDeliveryInput) (ReportDelivery, bool, error) {
	if s.closed || s.poisoned {
		return ReportDelivery{}, false, ErrUnavailable
	}
	if s.readOnly {
		return ReportDelivery{}, false, ErrReadOnly
	}
	if !validAccessNonce(in.ClientNonce) {
		return ReportDelivery{}, false, ErrInvalid
	}
	if id, ok := s.reportDeliveryNonces[in.ClientNonce]; ok {
		d := s.reportDeliveries[id]
		if reportInput(d) != in {
			return ReportDelivery{}, false, ErrConflict
		}
		return s.publicReportDelivery(d), true, nil
	}
	if s.reportOtherNonceUsed(in.ClientNonce) {
		return ReportDelivery{}, false, ErrConflict
	}
	if len(s.reportDeliveries) >= maxReportDeliveries {
		return ReportDelivery{}, false, ErrUnavailable
	}
	intent, err := s.reportIntent(in)
	if err != nil {
		return ReportDelivery{}, false, err
	}
	if !s.reportDestinationAllowed(in) {
		return ReportDelivery{}, false, ErrConflict
	}
	reply, question, artifacts, source, err := s.resolveReport(in, intent)
	if err != nil {
		return ReportDelivery{}, false, err
	}
	for _, a := range artifacts {
		b, err := os.ReadFile(filepath.Join(s.dir, "artifacts", a.ID))
		if err != nil {
			return ReportDelivery{}, false, ErrUnavailable
		}
		if !artifactMatches(a, b) || hasSecret(b) {
			return ReportDelivery{}, false, ErrInvalid
		}
	}
	id, err := newID("report-delivery-")
	if err != nil {
		return ReportDelivery{}, false, err
	}
	message, err := newID("message-")
	if err != nil {
		return ReportDelivery{}, false, err
	}
	d := ReportDelivery{Version: 1, ID: id, SourceMessageID: message, OwnerUserID: "local-owner", SpaceID: "local-space", BotID: in.BotID, DestinationChannelID: in.DestinationChannelID, ExpectedDestinationRevision: in.ExpectedDestinationRevision, ClientNonce: in.ClientNonce, Text: in.Text, Status: "delivered", CreatedAt: s.clock().UTC().Format(time.RFC3339Nano), CreatedSequence: uint64(len(s.events) + 1), Reply: reply, Question: question, Artifacts: artifacts, Source: source, Active: true}
	d.ClarificationOf = in.ClarificationOf
	if question != "" {
		d.Status = "needs_clarification"
	}
	e := Event{Sequence: d.CreatedSequence, Type: "report.delivered", ReportDelivery: &d, Nonce: in.ClientNonce, ReportDeliverySelectionVersion: 2}
	if !s.validReportDeliveryEvent(e) {
		return ReportDelivery{}, false, ErrInvalid
	}
	if err = s.commit(e); err != nil {
		return ReportDelivery{}, false, err
	}
	return s.publicReportDelivery(d), false, nil
}

func (s *Store) ReportDeliveryReceipt(nonce string) (ReportDeliveryReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := ReportDeliveryReceipt{Version: 1}
	if s.closed || s.poisoned {
		return out, ErrUnavailable
	}
	if !validAccessNonce(nonce) {
		return out, ErrInvalid
	}
	id, found := s.reportDeliveryNonces[nonce]
	if !found {
		if s.reportOtherNonceUsed(nonce) {
			return out, ErrConflict
		}
		return out, nil
	}
	d := s.publicReportDelivery(s.reportDeliveries[id])
	in := reportInput(d)
	out.Found, out.Input, out.Delivery, out.Active = true, &in, &d, &d.Active
	return out, nil
}

func (s *Store) ReadReportDeliveryArtifact(deliveryID, artifactID string) (Artifact, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readReportDeliveryArtifactLocked(deliveryID, artifactID)
}
func (s *Store) readReportDeliveryArtifactLocked(deliveryID, artifactID string) (Artifact, []byte, error) {
	if s.closed || s.poisoned {
		return Artifact{}, nil, ErrUnavailable
	}
	d, ok := s.reportDeliveries[deliveryID]
	if !ok || !lowerHex(artifactID, 64) || !s.reportDeliveryActive(d) {
		return Artifact{}, nil, ErrNotFound
	}
	for _, a := range d.Artifacts {
		if a.ID != artifactID {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.dir, "artifacts", a.ID))
		if err != nil {
			return Artifact{}, nil, ErrUnavailable
		}
		if !artifactMatches(a, b) || hasSecret(b) {
			return Artifact{}, nil, ErrCorrupt
		}
		return a, b, nil
	}
	return Artifact{}, nil, ErrNotFound
}
