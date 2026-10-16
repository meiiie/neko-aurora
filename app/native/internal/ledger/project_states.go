package ledger

import "reflect"

type ProjectState struct {
	Version   int    `json:"version"`
	ProjectID string `json:"projectId"`
	Revision  uint64 `json:"revision"`
	Archived  bool   `json:"archived"`
}
type ProjectControlInput struct {
	ProjectID        string `json:"projectId"`
	Action           string `json:"action"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	ClientNonce      string `json:"clientNonce"`
}

func (s *Store) projectState(id string) (ProjectState, bool) {
	if _, ok := s.projects[id]; !ok {
		return ProjectState{}, false
	}
	if p, ok := s.projectStates[id]; ok {
		return p, true
	}
	return ProjectState{Version: 1, ProjectID: id}, true
}
func (s *Store) archivedProject(id string) bool { p, ok := s.projectState(id); return ok && p.Archived }
func projectStateMutation(p ProjectState, in ProjectControlInput) (ProjectState, error) {
	if !validAccessNonce(in.ClientNonce) || !resourceID(in.ProjectID, "project-") || (in.Action != "archive" && in.Action != "restore") {
		return ProjectState{}, ErrInvalid
	}
	if p.ProjectID != in.ProjectID || p.Revision != in.ExpectedRevision || p.Revision >= 1<<53-1 || p.Archived == (in.Action == "archive") {
		return ProjectState{}, ErrConflict
	}
	p.Revision++
	p.Archived = in.Action == "archive"
	return p, nil
}
func (s *Store) validProjectStateEvent(e Event) bool {
	if e.Type != "project.state_updated" || e.ProjectState == nil || e.ProjectControlInput == nil || e.Nonce != e.ProjectControlInput.ClientNonce || !reflect.DeepEqual(e, Event{Sequence: e.Sequence, Type: e.Type, ProjectState: e.ProjectState, ProjectControlInput: e.ProjectControlInput, Nonce: e.Nonce}) || s.lifecycleNonceUsed(e.Nonce) || s.previousCommandNonceUsed(e.Nonce) || len(s.projectStateNonces) >= 2048 {
		return false
	}
	if _, ok := s.productNonces[e.Nonce]; ok {
		return false
	}
	if _, ok := s.projectStateNonces[e.Nonce]; ok {
		return false
	}
	p, ok := s.projectState(e.ProjectState.ProjectID)
	next, err := projectStateMutation(p, *e.ProjectControlInput)
	return ok && err == nil && next == *e.ProjectState
}
func (s *Store) ControlProject(in ProjectControlInput) (ProjectState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ProjectState{}, false, ErrUnavailable
	}
	if s.readOnly {
		return ProjectState{}, false, ErrReadOnly
	}
	if e, ok := s.projectStateNonces[in.ClientNonce]; ok {
		if *e.ProjectControlInput != in {
			return ProjectState{}, false, ErrConflict
		}
		p, _ := s.projectState(in.ProjectID)
		return p, true, nil
	}
	if _, ok := s.productNonces[in.ClientNonce]; ok {
		return ProjectState{}, false, ErrConflict
	}
	if s.lifecycleNonceUsed(in.ClientNonce) || s.previousCommandNonceUsed(in.ClientNonce) {
		return ProjectState{}, false, ErrConflict
	}
	if len(s.projectStateNonces) >= 2048 {
		return ProjectState{}, false, ErrUnavailable
	}
	p, ok := s.projectState(in.ProjectID)
	if !ok {
		return ProjectState{}, false, ErrNotFound
	}
	next, err := projectStateMutation(p, in)
	if err != nil {
		return ProjectState{}, false, err
	}
	if err = s.commit(Event{Type: "project.state_updated", ProjectState: &next, ProjectControlInput: &in, Nonce: in.ClientNonce}); err != nil {
		return ProjectState{}, false, err
	}
	return next, false, nil
}
