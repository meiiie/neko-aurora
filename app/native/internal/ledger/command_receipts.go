package ledger

type CommandReceipt struct {
	Challenge     *SecureChallenge `json:"challenge,omitempty"`
	Bot           *Bot             `json:"bot,omitempty"`
	BotProfile    *BotProfile      `json:"botProfile,omitempty"`
	Project       *Project         `json:"project,omitempty"`
	Conversation  *Conversation    `json:"conversation,omitempty"`
	ProjectState  *ProjectState    `json:"projectState,omitempty"`
	BotState      *BotState        `json:"botState,omitempty"`
	Version       int              `json:"version"`
	Kind          string           `json:"kind"`
	Found         bool             `json:"found"`
	ClientNonce   string           `json:"clientNonce"`
	Input         any              `json:"input,omitempty"`
	Work          *Work            `json:"work,omitempty"`
	Related       *Work            `json:"related,omitempty"`
	Collaboration *Collaboration   `json:"collaboration,omitempty"`
}

func receiptWork(w Work) *Work {
	w = copyWork(w)
	w.Context = nil
	w.RuntimeContext = nil
	w.ContextAttempts = nil
	w.Resources = receiptResources(w.Resources)
	return &w
}

func receiptResources(pin *ResourcePin) *ResourcePin {
	pin = publicResources(pin)
	if pin != nil {
		for i := range pin.Files {
			pin.Files[i].Excerpt = ""
		}
	}
	return pin
}

func (s *Store) CommandReceipt(kind, nonce, collaborationID string) (CommandReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return CommandReceipt{}, ErrUnavailable
	}
	if !validAccessNonce(nonce) || s.privateSimulationValue([]byte(nonce)) {
		return CommandReceipt{}, ErrInvalid
	}
	if kind == "collaboration_cancel" || kind == "collaboration_recover" {
		if !resourceID(collaborationID, "collab-") {
			return CommandReceipt{}, ErrInvalid
		}
	} else if collaborationID != "" {
		return CommandReceipt{}, ErrInvalid
	}
	out := CommandReceipt{Version: 1, Kind: kind, ClientNonce: nonce}
	switch kind {
	case "secure_challenge":
		if e, ok := s.challengeNonces[nonce]; ok {
			in := *e.ChallengeInput
			c := s.challenges[e.Challenge.ID]
			out.Found, out.Input, out.Challenge = true, in, &c
		}
	case "bot_create", "bot_name", "bot_profile", "project_save", "conversation_create", "conversation_archive":
		if err := s.fillMetadataReceipt(&out); err != nil {
			return CommandReceipt{}, err
		}
	case "project_control":
		if e, ok := s.projectStateNonces[nonce]; ok {
			in := *e.ProjectControlInput
			p, _ := s.projectState(in.ProjectID)
			out.Found, out.Input = true, in
			out.ProjectState = &p
		}
	case "bot_control":
		if event, ok := s.botStateNonces[nonce]; ok {
			in := *event.BotControlInput
			p, _ := s.botState(in.BotID)
			out.Found, out.Input = true, in
			out.BotState = &p
		}
	case "task_control":
		if event, ok := s.taskControlNonces[nonce]; ok {
			result := s.controlResult(event)
			out.Found, out.Input = true, event.TaskControl.Input
			out.Work = receiptWork(result.Work)
			if result.Related != nil {
				out.Related = receiptWork(*result.Related)
			}
		}
	case "collaboration_create":
		if id, ok := s.collaborationNonces[nonce]; ok {
			g := copyCollaboration(s.collaborations[id])
			out.Found, out.Input = true, normalizeCollaboration(g.CollaborationInput)
			g.Resources = receiptResources(g.Resources)
			out.Collaboration = &g
		}
	case "collaboration_cancel":
		if event, ok := s.collaborationActions[nonce]; ok {
			if event.Type != "collaboration.cancelling" || event.Collaboration.ID != collaborationID {
				return CommandReceipt{}, ErrConflict
			}
			g := copyCollaboration(s.collaborations[collaborationID])
			out.Found, out.Input = true, CollaborationCancelInput{ExpectedRevision: event.Collaboration.Revision - 1, ClientNonce: nonce}
			g.Resources = receiptResources(g.Resources)
			out.Collaboration = &g
		}
	case "collaboration_recover":
		if event, ok := s.collaborationActions[nonce]; ok {
			if event.Type != "collaboration.recovered" || event.Collaboration.ID != collaborationID || event.CollaborationRecovery == nil {
				return CommandReceipt{}, ErrConflict
			}
			result := s.recoveryResult(event)
			g := result.Collaboration
			g.Resources = receiptResources(g.Resources)
			out.Found, out.Input = true, event.CollaborationRecovery.Input
			out.Collaboration = &g
			out.Work = receiptWork(result.Work)
		}
	default:
		return CommandReceipt{}, ErrInvalid
	}
	if !out.Found {
		if _, used := s.reportDeliveryNonces[nonce]; used {
			return CommandReceipt{}, ErrConflict
		}
		_, task := s.taskControlNonces[nonce]
		_, create := s.collaborationNonces[nonce]
		_, action := s.collaborationActions[nonce]
		_, bot := s.botStateNonces[nonce]
		_, project := s.projectStateNonces[nonce]
		_, product := s.productNonces[nonce]
		_, conversation := s.conversationNonces[nonce]
		if task || create || action || bot || project || product || conversation || s.botMetadataNonceUsed(nonce) || s.lifecycleNonceUsed(nonce) {
			return CommandReceipt{}, ErrConflict
		}
	}
	if out.Work != nil {
		w := s.publicWorkLocked(*out.Work)
		out.Work = &w
	}
	if out.Related != nil {
		w := s.publicWorkLocked(*out.Related)
		out.Related = &w
	}
	if out.Collaboration != nil {
		out.Collaboration.PublishedReportContext = nil
	}
	return out, nil
}
