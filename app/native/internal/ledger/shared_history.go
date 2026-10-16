package ledger

import (
	"nekobot.local/native/internal/historyexport"
	"sort"
)

func (s *Store) ExportSharedHistory(channel, project, format string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exportSharedHistoryLocked(channel, project, format, false)
}
func (s *Store) exportSharedHistoryLocked(channel, project, format string, requireContext bool) ([]byte, error) {
	if s.closed || s.poisoned {
		return nil, ErrUnavailable
	}
	c, ok := s.channels[channel]
	if !ok || !validChannelID(channel) {
		return nil, ErrNotFound
	}
	title := c.Name
	if project != "" {
		p, ok := s.projects[project]
		if !ok || !contains(p.ChannelIDs, channel) {
			return nil, ErrConflict
		}
		title = p.Name + " / " + c.Name
	}
	type row struct {
		sequence uint64
		message  historyexport.Message
	}
	rows := []row{}
	matches := func(p *ResourcePin) bool { return project == "" || p != nil && p.Project.ID == project }
	files := func(w Work) []historyexport.File {
		out := []historyexport.File{}
		if s.contextAllowed(w) {
			for _, f := range w.Artifacts {
				out = append(out, historyexport.File{ID: f.ID, Name: f.Filename, SHA256: f.SHA256, Bytes: f.Bytes, DownloadPath: "/v1/channels/" + channel + "/artifacts/" + f.ID})
			}
		}
		return out
	}
	for _, w := range s.sortedWorkLocked() {
		if w.ConversationID != channel || w.OwnerUserID != "local-owner" || w.SpaceID != "local-space" || w.Collaboration != nil || !s.publicationAllowed(w) || !matches(w.Resources) {
			continue
		}
		if (requireContext || s.reportDerived(w)) && !s.contextAllowed(w) {
			continue
		}
		rows = append(rows, row{w.CreatedSequence, historyexport.Message{RequestID: w.RequestID, BotID: w.BotID, CreatedAt: w.CreatedAt, Text: w.Text, Reply: w.Reply, Status: w.Status, Files: files(w)}})
		if len(rows) > historyexport.MaxMessages {
			return nil, ErrInvalid
		}
	}
	for _, g := range s.collaborations {
		if g.ChannelID != channel || g.OwnerUserID != "local-owner" || g.SpaceID != "local-space" || g.ExpectedChannelRevision != c.Revision || !matches(g.Resources) {
			continue
		}
		m := historyexport.Message{RequestID: g.ID, BotID: g.CoordinatorBotID, CreatedAt: g.CreatedAt, Text: g.Goal, Status: g.Status, Files: []historyexport.File{}}
		if len(g.RequestIDs) == 3 {
			w := s.work[g.RequestIDs[2]]
			if w.Collaboration != nil && w.Collaboration.ID == g.ID && w.Collaboration.Stage == 3 && w.Status == "completed" && s.publicationAllowed(w) && (!(requireContext || s.reportDerived(w)) || s.contextAllowed(w)) {
				m.Reply = w.Reply
				m.Files = files(w)
			}
		}
		sequence := uint64(0)
		for _, e := range s.events {
			if e.Type == "collaboration.created" && e.Collaboration.ID == g.ID {
				sequence = e.Sequence
				break
			}
		}
		rows = append(rows, row{sequence, m})
		if len(rows) > historyexport.MaxMessages {
			return nil, ErrInvalid
		}
	}
	if project == "" {
		for _, delivery := range s.reportDeliveries {
			if delivery.DestinationChannelID != channel {
				continue
			}
			d := s.publicReportDelivery(delivery)
			if !d.Active {
				continue
			}
			m := historyexport.Message{RequestID: d.ID, BotID: d.BotID, CreatedAt: d.CreatedAt, Text: d.Text, Reply: d.Reply, Status: d.Status, Files: []historyexport.File{}}
			if d.Status == "needs_clarification" {
				m.Reply = d.Question
			}
			for _, a := range d.Artifacts {
				m.Files = append(m.Files, historyexport.File{ID: a.ID, Name: a.Filename, SHA256: a.SHA256, Bytes: a.Bytes, DownloadPath: "/v1/report-deliveries/" + d.ID + "/artifacts/" + a.ID})
			}
			rows = append(rows, row{d.CreatedSequence, m})
			if len(rows) > historyexport.MaxMessages {
				return nil, ErrInvalid
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].sequence < rows[j].sequence })
	h := historyexport.History{Version: 1, ConversationID: channel, Title: title, BotID: "shared-channel", AtSequence: uint64(len(s.events)), Messages: []historyexport.Message{}}
	for _, row := range rows {
		h.Messages = append(h.Messages, row.message)
	}
	if sensitiveHistory(h) {
		return nil, ErrInvalid
	}
	raw, err := historyexport.Encode(h, format)
	if err != nil || hasSecret(raw) {
		return nil, ErrInvalid
	}
	return raw, nil
}
