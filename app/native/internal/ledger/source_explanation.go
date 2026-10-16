package ledger

type SourceExplanationItem struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	Reason          string `json:"reason"`
	PinnedRevision  uint64 `json:"pinnedRevision"`
	CurrentRevision uint64 `json:"currentRevision"`
	Usable          bool   `json:"usable"`
	Name            string `json:"name,omitempty"`
}
type SourceExplanation struct {
	Version    int                     `json:"version"`
	RequestID  string                  `json:"requestId"`
	ChannelID  string                  `json:"channelId"`
	ProjectID  string                  `json:"projectId,omitempty"`
	AtSequence uint64                  `json:"atSequence"`
	Usable     bool                    `json:"usable"`
	Omitted    int                     `json:"omitted"`
	Sources    []SourceExplanationItem `json:"sources"`
}

func (s *Store) sourceViewScope(w Work, channel, project string) bool {
	if w.OwnerUserID != "local-owner" || w.SpaceID != "local-space" || w.ConversationID != channel || !s.publicationAllowed(w) {
		return false
	}
	if w.Resources == nil {
		return project == ""
	}
	p, ok := s.projects[project]
	return project == w.Resources.Project.ID && ok && contains(p.ChannelIDs, channel)
}
func (s *Store) ExplainSources(id, channel, project string) (SourceExplanation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return SourceExplanation{}, ErrUnavailable
	}
	w, ok := s.work[id]
	if !ok {
		return SourceExplanation{}, ErrNotFound
	}
	if !s.sourceViewScope(w, channel, project) {
		return SourceExplanation{}, ErrConflict
	}
	out := SourceExplanation{Version: 1, RequestID: id, ChannelID: channel, ProjectID: project, AtSequence: uint64(len(s.events)), Usable: s.contextAllowed(w), Sources: []SourceExplanationItem{{ID: w.SourceMessageID, Kind: "primary", Reason: "owner_request", PinnedRevision: w.Epoch, CurrentRevision: w.Epoch, Usable: accessGranted(w)}}}
	if w.Context != nil {
		out.Omitted = w.Context.Omitted
		for _, p := range w.Context.PublishedReports {
			out.Sources = append(out.Sources, SourceExplanationItem{ID: p.DeliveryID, Kind: "published_report", Reason: "owner_published_result_destination_only", PinnedRevision: p.Sequence, CurrentRevision: p.Sequence, Usable: s.publishedReportAllowed(w, p, w.Context.AtSequence)})
		}
		for _, pin := range w.Context.Sources {
			current, found := s.work[pin.RequestID]
			usable := found && current.ConversationID == channel && current.OwnerUserID == w.OwnerUserID && current.SpaceID == w.SpaceID && accessRevision(current) == pin.AccessRevision && s.contextAllowed(current)
			out.Sources = append(out.Sources, SourceExplanationItem{ID: pin.RequestID, Kind: "history", Reason: "completed_turn_same_scope_bounded", PinnedRevision: pin.AccessRevision, CurrentRevision: accessRevision(current), Usable: usable})
		}
	}
	if w.BotProfile != nil {
		p, _ := s.profile(w.BotID)
		out.Sources = append(out.Sources, SourceExplanationItem{ID: w.BotID, Kind: "profile", Reason: "profile_snapshot_at_intake", PinnedRevision: w.BotProfile.Revision, CurrentRevision: p.Revision, Usable: true})
	}
	if w.Resources != nil {
		current := s.projects[project]
		out.Sources = append(out.Sources, SourceExplanationItem{ID: project, Kind: "project", Reason: "project_scope_at_intake", PinnedRevision: w.Resources.Project.Revision, CurrentRevision: current.Revision, Usable: current.Revision == w.Resources.Project.Revision})
		for _, pin := range w.Resources.Files {
			f, found := s.uploads[pin.ID]
			usable := found && f.Granted && f.AccessRevision == pin.AccessRevision && f.SHA256 == pin.SHA256 && f.ProjectID == project && f.ChannelID == channel && current.Revision == w.Resources.Project.Revision
			reason := "literal_filename_text_match_bounded"
			if contains(w.Resources.RequestedFileIDs, pin.ID) {
				reason = "explicit_file_selection_bounded"
			}
			item := SourceExplanationItem{ID: pin.ID, Kind: "file", Reason: reason, PinnedRevision: pin.AccessRevision, CurrentRevision: f.AccessRevision, Usable: usable}
			if usable && !hasSecret([]byte(pin.Filename)) {
				item.Name = pin.Filename
			}
			out.Sources = append(out.Sources, item)
		}
		if pin := w.Resources.Continuity; pin != nil {
			current := s.projectContinuity[continuityKey(project, channel)]
			out.Sources = append(out.Sources, SourceExplanationItem{ID: project, Kind: "continuity", Reason: "owner_project_metadata_snapshot", PinnedRevision: pin.Revision, CurrentRevision: current.Revision, Usable: s.continuityAllowed(w)})
		}
	}
	for _, pin := range w.PluginPins {
		p, found := s.plugin(pin.ID)
		out.Sources = append(out.Sources, SourceExplanationItem{ID: pin.ID, Kind: "plugin", Reason: "selected_tool_schema_pin", PinnedRevision: pin.Revision, CurrentRevision: p.Revision, Usable: found && p.Revision == pin.Revision && s.pluginsAllowed(w)})
	}
	return out, nil
}
