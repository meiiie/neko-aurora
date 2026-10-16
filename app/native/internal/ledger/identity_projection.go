package ledger

import (
	"bytes"
	"encoding/json"
	"nekobot.local/native/internal/identity"
	"nekobot.local/native/internal/syncstream"
	"sort"
)

type ScopedEvent struct {
	Sequence       uint64         `json:"sequence"`
	SessionBinding string         `json:"sessionBinding"`
	CacheNamespace string         `json:"cacheNamespace"`
	Event          map[string]any `json:"event"`
}

func (s *Store) ScopedAudienceBinding(p identity.VerifiedPrincipal) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.principalReadGuard(p, "")
	if err != nil {
		return "", err
	}
	defer release()
	type audience struct {
		ID       string
		Revision uint64
	}
	list := []audience{}
	for _, c := range s.channels {
		if humanCanRead(c, p) {
			list = append(list, audience{c.ID, c.Revision})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	raw, _ := json.Marshal(list)
	return manifestHash(identity.Binding(p) + "|" + string(raw)), nil
}
func memberPublicWork(w Work) Work {
	return Work{RequestID: w.RequestID, SourceMessageID: w.SourceMessageID, OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, BotID: w.BotID, ConversationID: w.ConversationID, ClientNonce: w.ClientNonce, Text: w.Text, Lane: w.Lane, Status: w.Status, DesiredState: w.DesiredState, Epoch: w.Epoch, BotConfigRevision: w.BotConfigRevision, CreatedAt: w.CreatedAt, CreatedSequence: w.CreatedSequence, Reply: w.Reply, Failure: w.Failure, Artifacts: append([]Artifact(nil), w.Artifacts...), SourceBotID: w.SourceBotID, RoutedAlias: w.RoutedAlias, Publication: copyWork(Work{Publication: w.Publication}).Publication, RequesterPin: copyRequester(w.RequesterPin)}
}
func (s *Store) memberWorkVisible(w Work, p identity.VerifiedPrincipal) bool {
	c, ok := s.channels[w.ConversationID]
	return ok && humanCanRead(c, p) && s.publicationAllowed(w) && s.contextAllowed(w) && w.Collaboration == nil
}
func (s *Store) scopedWorkLocked(w Work, p identity.VerifiedPrincipal) Work {
	if p.ActorID == "local-owner" {
		return publicContinuityWork(s.redactPublishedWork(w))
	}
	return memberPublicWork(s.redactPublishedWork(w))
}
func (s *Store) principalReadGuard(p identity.VerifiedPrincipal, channel string) (func(), error) {
	if s.identity == nil {
		return nil, ErrForbidden
	}
	if channel != "" {
		c, ok := s.channel(channel)
		if !ok || !humanCanRead(c, p) {
			return nil, ErrForbidden
		}
	}
	return s.identity.Hold(p)
}
func (s *Store) scopedSnapshotLocked(p identity.VerifiedPrincipal) Snapshot {
	if p.ActorID == "local-owner" {
		out := s.snapshotLocked()
		for i := range out.Work {
			out.Work[i] = publicContinuityWork(out.Work[i])
		}
		for i := range out.Collaborations {
			out.Collaborations[i] = PublicContinuityCollaboration(out.Collaborations[i])
		}
		for i := range out.Channels {
			out.Channels[i].SelfRole = "admin"
		}
		out.Capabilities = append(out.Capabilities, "local_fixture_identity_v1", "local_scoped_shared_v1")
		capabilities := []string{}
		for _, c := range out.Capabilities {
			if c != "local_collaboration_v1" && c != "local_collaboration_service_recovery_v1" && c != "local_task_controls_v1" && c != "queued_task_pause_v1" && c != "local_surface_bridge_v1" && c != "local_adapter_installation_intake_cas_v1" {
				capabilities = append(capabilities, c)
			}
		}
		out.Capabilities = capabilities
		return out
	}
	out := Snapshot{Sequence: uint64(len(s.events)), ReadOnly: s.readOnly, Runtime: "fixture_scoped_shared", Bots: []Bot{}, Work: []Work{}, ReportDeliveries: []ReportDelivery{}, Channels: []Channel{}, Capabilities: []string{"local_fixture_identity_v1", "local_scoped_shared_v1", "local_channel_intake_cas_v1", "local_public_history_export_v1", "local_sync_simulation_v1", "local_published_report_context_v1"}}
	visibleBots := map[string]bool{}
	for _, c := range s.channels {
		if humanCanRead(c, p) {
			copy := cloneChannel(c)
			copy.HumanACL = nil
			copy.SelfRole = humanRole(c, p)
			out.Channels = append(out.Channels, copy)
			for _, id := range c.BotIDs {
				visibleBots[id] = true
			}
		}
	}
	for _, b := range s.bots {
		if visibleBots[b.ID] {
			b.Description = ""
			out.Bots = append(out.Bots, b)
		}
	}
	for _, w := range s.sortedWorkLocked() {
		if s.memberWorkVisible(w, p) {
			out.Work = append(out.Work, s.scopedWorkLocked(w, p))
		}
	}
	for _, d := range s.reportDeliveries {
		c, ok := s.channels[d.DestinationChannelID]
		if ok && humanCanRead(c, p) {
			copy := s.publicReportDelivery(d)
			copy.Source = nil
			out.ReportDeliveries = append(out.ReportDeliveries, copy)
		}
	}
	sort.Slice(out.Channels, func(i, j int) bool { return out.Channels[i].ID < out.Channels[j].ID })
	sort.Slice(out.ReportDeliveries, func(i, j int) bool {
		return out.ReportDeliveries[i].CreatedSequence < out.ReportDeliveries[j].CreatedSequence
	})
	return out
}
func (s *Store) ScopedSnapshot(p identity.VerifiedPrincipal) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.principalReadGuard(p, "")
	if err != nil {
		return Snapshot{}, err
	}
	defer release()
	return s.scopedSnapshotLocked(p), nil
}
func (s *Store) WriteScopedSnapshot(p identity.VerifiedPrincipal, write func(Snapshot) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.principalReadGuard(p, "")
	if err != nil {
		return err
	}
	defer release()
	return write(s.scopedSnapshotLocked(p))
}
func (s *Store) scopedEventLocked(p identity.VerifiedPrincipal, e Event) ScopedEvent {
	out := ScopedEvent{Sequence: e.Sequence, SessionBinding: identity.Binding(p), CacheNamespace: identity.ActorNamespace(p), Event: map[string]any{}}
	if p.ActorID == "local-owner" {
		events, _ := s.eventsLocked(e.Sequence - 1)
		if len(events) > 0 {
			event := events[0]
			if event.Channel != nil {
				c := cloneChannel(*event.Channel)
				c.SelfRole = "admin"
				event.Channel = &c
			}
			raw, _ := json.Marshal(event)
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber()
			_ = decoder.Decode(&out.Event)
		}
		return out
	}
	if e.Type == "channel.updated" || e.Type == "channel.created" || e.Type == "work.source_access" || e.Type == "plugin.lifecycle" || e.BotState != nil || e.Upload != nil || e.ProjectState != nil || e.ProjectContinuity != nil || e.Conversation != nil || e.Type == "report.delivered" {
		out.Event = map[string]any{"type": "scope_invalidated", "sequence": e.Sequence}
		return out
	}
	if e.Work.RequestID != "" {
		current, ok := s.work[e.Work.RequestID]
		if ok && s.memberWorkVisible(current, p) {
			out.Event = map[string]any{"sequence": e.Sequence, "type": e.Type, "work": s.scopedWorkLocked(e.Work, p)}
		}
	}
	if e.Channel != nil {
		current, ok := s.channels[e.Channel.ID]
		if ok && humanCanRead(current, p) {
			c := cloneChannel(current)
			c.HumanACL = nil
			c.SelfRole = humanRole(current, p)
			out.Event = map[string]any{"sequence": e.Sequence, "type": e.Type, "channel": c}
		}
	}
	if e.ReportDelivery != nil {
		d := s.reportDeliveries[e.ReportDelivery.ID]
		c, ok := s.channels[d.DestinationChannelID]
		if ok && humanCanRead(c, p) {
			copy := s.publicReportDelivery(d)
			copy.Source = nil
			out.Event = map[string]any{"sequence": e.Sequence, "type": e.Type, "reportDelivery": copy}
		}
	}
	return out
}
func (s *Store) ScopedEvents(p identity.VerifiedPrincipal, after uint64) ([]ScopedEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.principalReadGuard(p, "")
	if err != nil {
		return nil, err
	}
	defer release()
	if after > uint64(len(s.events)) {
		return nil, ErrConflict
	}
	out := []ScopedEvent{}
	for _, e := range s.events[int(after):] {
		out = append(out, s.scopedEventLocked(p, e))
	}
	return out, nil
}
func (s *Store) WriteScopedEvent(p identity.VerifiedPrincipal, sequence uint64, write func(ScopedEvent) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.principalReadGuard(p, "")
	if err != nil {
		return err
	}
	defer release()
	if sequence == 0 || sequence > uint64(len(s.events)) {
		return ErrConflict
	}
	return write(s.scopedEventLocked(p, s.events[sequence-1]))
}
func (s *Store) ScopedPublicWork(p identity.VerifiedPrincipal, id string) (Work, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[id]
	if !ok {
		return Work{}, ErrNotFound
	}
	release, err := s.principalReadGuard(p, w.ConversationID)
	if err != nil {
		return Work{}, err
	}
	defer release()
	if p.ActorID != "local-owner" && !s.memberWorkVisible(w, p) {
		return Work{}, ErrForbidden
	}
	return s.scopedWorkLocked(w, p), nil
}
func (s *Store) ScopedLocalSync(p identity.VerifiedPrincipal, scope LocalSyncScope, token string) (syncstream.Envelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.principalReadGuard(p, scope.ChannelID)
	if err != nil {
		return syncstream.Envelope{}, err
	}
	defer release()
	return s.localSyncLocked(scope, token, &p)
}
func (s *Store) ScopedExportHistory(p identity.VerifiedPrincipal, channel, project, format string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.principalReadGuard(p, channel)
	if err != nil {
		return nil, err
	}
	defer release()
	if p.ActorID != "local-owner" && project != "" {
		return nil, ErrForbidden
	}
	return s.exportSharedHistoryLocked(channel, project, format, p.ActorID != "local-owner")
}
func (s *Store) ScopedReadArtifact(p identity.VerifiedPrincipal, channel, id string) (Artifact, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.principalReadGuard(p, channel)
	if err != nil {
		return Artifact{}, nil, err
	}
	defer release()
	if !validChannelID(channel) {
		return Artifact{}, nil, ErrForbidden
	}
	return s.readArtifactLocked(id, channel)
}
func (s *Store) ScopedReadReportArtifact(p identity.VerifiedPrincipal, delivery, id string) (Artifact, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.reportDeliveries[delivery]
	if !ok {
		return Artifact{}, nil, ErrNotFound
	}
	release, err := s.principalReadGuard(p, d.DestinationChannelID)
	if err != nil {
		return Artifact{}, nil, err
	}
	defer release()
	return s.readReportDeliveryArtifactLocked(delivery, id)
}
