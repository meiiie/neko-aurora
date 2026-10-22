package ledger

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"nekoaurora.local/native/internal/identity"
	"nekoaurora.local/native/internal/syncstream"
)

type LocalSyncScope struct{ BotID, ChannelID, ProjectID string }

func (s *Store) openSyncIdentity() error {
	path := filepath.Join(s.dir, "local-sync.identity")
	valid := func(raw []byte) bool {
		return len(raw) == 40 && strings.HasPrefix(string(raw), "journal-") && lowerHex(string(raw[8:]), 32)
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Size() != 40 {
			return ErrUnavailable
		}
		raw, err := os.ReadFile(path)
		if err != nil || !valid(raw) {
			return ErrUnavailable
		}
		s.syncID = string(raw)
		return nil
	} else if !os.IsNotExist(err) {
		return ErrUnavailable
	}
	if s.readOnly {
		return ErrReadOnly
	}
	id, err := newID("journal-")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, "local-sync-init-")
	if err != nil {
		return ErrUnavailable
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	if _, err = io.WriteString(f, id); err != nil {
		f.Close()
		return ErrUnavailable
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return ErrUnavailable
	}
	if err = f.Close(); err != nil {
		return ErrUnavailable
	}
	if err = os.Rename(temporary, path); err != nil {
		return ErrUnavailable
	}
	s.syncID = id
	return nil
}

func (s *Store) localSyncBinding(scope LocalSyncScope) (LocalSyncScope, Channel, string, error) {
	if scope.ChannelID == "" {
		scope.ChannelID = "conversation-" + scope.BotID
	}
	c, ok := s.channel(scope.ChannelID)
	if !ok || !member(c, scope.BotID) {
		return scope, Channel{}, "", ErrNotFound
	}
	projectRevision := uint64(0)
	if scope.ProjectID != "" {
		p, ok := s.projects[scope.ProjectID]
		if !ok || !contains(p.ChannelIDs, c.ID) {
			return scope, Channel{}, "", ErrNotFound
		}
		projectRevision = p.Revision
	}
	type accessFence struct {
		ID       string
		Revision uint64
	}
	files := []accessFence{}
	for _, f := range s.uploads {
		if f.ProjectID == scope.ProjectID && f.ChannelID == scope.ChannelID && f.AccessRevision > 0 {
			files = append(files, accessFence{f.ID, f.AccessRevision})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ID < files[j].ID })
	type reportFence struct {
		ID             string
		Active         bool
		AccessRevision uint64
		Epoch          uint64
		BotRevision    uint64
	}
	reports := []reportFence{}
	if scope.ProjectID == "" {
		for _, d := range s.reportDeliveries {
			if d.DestinationChannelID != scope.ChannelID {
				continue
			}
			f := reportFence{ID: d.ID, Active: s.reportDeliveryActive(d)}
			b, _ := s.botState(d.BotID)
			f.BotRevision = b.Revision
			if d.Source != nil {
				w := s.work[d.Source.RequestID]
				f.AccessRevision, f.Epoch = accessRevision(w), w.Epoch
			}
			reports = append(reports, f)
		}
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].ID < reports[j].ID })
	type derivedFence struct {
		ID     string
		Usable bool
	}
	derived := []derivedFence{}
	for _, w := range s.work {
		if w.ConversationID == scope.ChannelID && s.reportDerived(w) && (scope.ProjectID == "" && w.Resources == nil || w.Resources != nil && w.Resources.Project.ID == scope.ProjectID) {
			derived = append(derived, derivedFence{w.RequestID, s.contextAllowed(w)})
		}
	}
	sort.Slice(derived, func(i, j int) bool { return derived[i].ID < derived[j].ID })
	raw, _ := json.Marshal(struct {
		Version                             int
		Owner, Space, Bot, Channel, Project string
		Audience, ProjectRevision           uint64
		Files                               []accessFence
		Reports                             []reportFence
		DerivedReports                      []derivedFence
	}{1, c.OwnerUserID, c.SpaceID, scope.BotID, c.ID, scope.ProjectID, c.Revision, projectRevision, files, reports, derived})
	return scope, c, "scope-" + manifestHash(string(raw)), nil
}

func syncBot(bot Bot, revision uint64) syncstream.Object {
	return syncstream.Object{Kind: "bot", ID: bot.ID, Revision: revision, Bot: &syncstream.Bot{Name: bot.Name, Alias: bot.Alias, Description: bot.Description, Color: bot.Color}}
}

func (s *Store) LocalSync(scope LocalSyncScope, token string) (syncstream.Envelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.localSyncLocked(scope, token, nil)
}
func (s *Store) localSyncLocked(scope LocalSyncScope, token string, p *identity.VerifiedPrincipal) (syncstream.Envelope, error) {
	if s.closed || s.poisoned || s.syncID == "" {
		return syncstream.Envelope{}, ErrUnavailable
	}
	scope, channel, binding, err := s.localSyncBinding(scope)
	if err != nil {
		return syncstream.Envelope{}, err
	}
	if p != nil {
		if !humanCanRead(channel, *p) || p.ActorID != "local-owner" && (!validChannelID(channel.ID) || scope.ProjectID != "") {
			return syncstream.Envelope{}, ErrForbidden
		}
		type contentFence struct {
			ID      string
			Access  uint64
			Allowed bool
		}
		fences := []contentFence{}
		for _, w := range s.sortedWorkLocked() {
			if w.ConversationID == scope.ChannelID && accessRevision(w) > 0 {
				fences = append(fences, contentFence{w.RequestID, accessRevision(w), s.contextAllowed(w)})
			}
		}
		raw, _ := json.Marshal(struct {
			Access  []contentFence
			Plugins []PluginRecord
		}{fences, s.pluginsLocked()})
		binding = "scope-" + manifestHash(binding+"|"+identity.Binding(*p)+"|"+string(raw))
	}
	highWater := uint64(len(s.events))
	if highWater >= syncstream.MaxSafeSequence {
		return syncstream.Envelope{}, syncstream.ErrLimit
	}
	var cursor syncstream.Cursor
	if token != "" {
		cursor, err = syncstream.ParseCursor(token)
		if err != nil {
			return syncstream.Envelope{}, err
		}
		if err = cursor.Check(s.syncID, binding, highWater); err != nil {
			return syncstream.Envelope{}, err
		}
	}
	objects := map[string]syncstream.Object{}
	put := func(object syncstream.Object) { objects[object.Kind+":"+object.ID] = object }
	for _, bot := range bots {
		if member(channel, bot.ID) {
			if p != nil && p.ActorID != "local-owner" {
				bot.Description = ""
			}
			put(syncBot(bot, 1))
		}
	}
	frames := []syncstream.Frame{}
	var increment syncstream.Envelope
	limit := func() (syncstream.Envelope, error) {
		if token != "" && len(frames) > 0 {
			return increment, nil
		}
		return syncstream.Envelope{}, syncstream.ErrLimit
	}
	projectMatches := func(p *ResourcePin) bool {
		if p == nil {
			return scope.ProjectID == ""
		}
		return p.Project.ID == scope.ProjectID
	}
	workVisible := func(w Work) bool {
		if p != nil && p.ActorID != "local-owner" {
			current, ok := s.work[w.RequestID]
			if !ok || !s.contextAllowed(current) {
				return false
			}
		}
		return w.RequestID != "" && w.OwnerUserID == channel.OwnerUserID && w.SpaceID == channel.SpaceID && w.ConversationID == channel.ID && projectMatches(w.Resources) && s.publicationAllowed(w) && (!s.reportDerived(w) || s.contextAllowed(w))
	}
	for _, event := range s.events {
		if token != "" {
			if event.Sequence <= cursor.Sequence {
				continue
			}
			if len(frames) == syncstream.MaxFrames {
				break
			}
		}
		revision := event.Sequence + 1
		updates := []syncstream.Object{}
		appendObject := func(object syncstream.Object) { put(object); updates = append(updates, object) }
		if event.Bot != nil && member(channel, event.Bot.ID) {
			b := *event.Bot
			if p != nil && p.ActorID != "local-owner" {
				b.Description = ""
			}
			appendObject(syncBot(b, revision))
		}
		if event.ReportDelivery != nil && scope.ProjectID == "" && event.ReportDelivery.DestinationChannelID == channel.ID {
			d := s.publicReportDelivery(*event.ReportDelivery)
			if d.Active {
				reply := d.Reply
				if d.Status == "needs_clarification" {
					reply = d.Question
				}
				if len(reply) > 12<<10 {
					return limit()
				}
				appendObject(syncstream.Object{Kind: "message", ID: d.SourceMessageID, Revision: revision, Message: &syncstream.Message{BotID: d.BotID, ChannelID: d.DestinationChannelID, Role: "user", Text: d.Text, Status: d.Status}})
				appendObject(syncstream.Object{Kind: "message", ID: "reply-" + d.SourceMessageID, Revision: revision, Message: &syncstream.Message{BotID: d.BotID, ChannelID: d.DestinationChannelID, Role: "assistant", Text: reply, Status: d.Status}})
			}
		}
		if g := event.Collaboration; g != nil && g.OwnerUserID == channel.OwnerUserID && g.SpaceID == channel.SpaceID && g.ChannelID == channel.ID && g.ExpectedChannelRevision == channel.Revision && g.ProjectID == scope.ProjectID {
			appendObject(syncstream.Object{Kind: "message", ID: "goal-" + g.ID, Revision: revision, Message: &syncstream.Message{BotID: g.CoordinatorBotID, ChannelID: g.ChannelID, Role: "user", Text: g.Goal, Status: g.Status}})
		}
		works := []Work{event.Work}
		if event.TaskControl != nil && event.TaskControl.Related != nil {
			works = append(works, *event.TaskControl.Related)
		}
		for _, w := range works {
			if !workVisible(w) || w.Collaboration != nil && w.Collaboration.Stage != 3 {
				continue
			}
			if len(w.Reply) > 12<<10 || w.Collaboration == nil && len(w.Text) > 12<<10 {
				return limit()
			}
			if w.Collaboration == nil {
				appendObject(syncstream.Object{Kind: "message", ID: w.SourceMessageID, Revision: revision, Message: &syncstream.Message{BotID: w.BotID, ChannelID: w.ConversationID, Role: "user", Text: w.Text, Status: w.Status}})
			}
			appendObject(syncstream.Object{Kind: "message", ID: "reply-" + w.SourceMessageID, Revision: revision, Message: &syncstream.Message{BotID: w.BotID, ChannelID: w.ConversationID, Role: "assistant", Text: w.Reply, Status: w.Status}})
			for _, a := range w.Artifacts {
				appendObject(syncstream.Object{Kind: "file", ID: a.ID, Revision: revision, File: &syncstream.File{Name: a.Filename, MIME: "text/plain; charset=utf-8", Size: uint64(a.Bytes), SHA256: a.SHA256, BotID: w.BotID, ChannelID: w.ConversationID, ProjectID: scope.ProjectID, Status: "ready_artifact", Version: 1, AccessRevision: 0}})
			}
		}
		if f := event.Upload; f != nil && scope.ProjectID != "" && f.ProjectID == scope.ProjectID && f.ChannelID == channel.ID {
			status := f.Status
			if !f.Granted {
				status = "access_revoked"
			}
			appendObject(syncstream.Object{Kind: "file", ID: f.ID, Revision: revision, File: &syncstream.File{Name: f.Filename, MIME: f.MIME, Size: uint64(f.Bytes), SHA256: f.SHA256, ChannelID: f.ChannelID, ProjectID: f.ProjectID, Status: status, Version: uint64(f.Version), AccessRevision: f.AccessRevision}})
		}
		if token != "" {
			candidate := append(frames, syncstream.Frame{Sequence: event.Sequence, Updates: updates})
			next, err := syncstream.NewIncrement(s.syncID, binding, cursor.Sequence, highWater, candidate)
			if err != nil {
				if errors.Is(err, syncstream.ErrLimit) {
					return limit()
				}
				return syncstream.Envelope{}, err
			}
			frames, increment = candidate, next
		}
	}
	if token != "" {
		if len(frames) > 0 {
			return increment, nil
		}
		return syncstream.NewIncrement(s.syncID, binding, cursor.Sequence, highWater, frames)
	}
	all := make([]syncstream.Object, 0, len(objects))
	for _, object := range objects {
		all = append(all, object)
	}
	return syncstream.NewSnapshot(s.syncID, binding, highWater, all)
}
