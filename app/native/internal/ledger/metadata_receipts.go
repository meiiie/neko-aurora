package ledger

type BotNameReceiptInput struct {
	BotID string `json:"botId"`
	BotNameInput
}
type BotProfileReceiptInput struct {
	BotID string `json:"botId"`
	BotProfileInput
}
type ConversationArchiveReceiptInput struct {
	ConversationID string `json:"conversationId"`
	ConversationArchiveInput
}

func (s *Store) receiptBot(id string) *Bot {
	for _, p := range s.bots {
		if p.ID == id {
			return &p
		}
	}
	return nil
}
func (s *Store) fillMetadataReceipt(out *CommandReceipt) error {
	nonce := out.ClientNonce
	switch out.Kind {
	case "bot_create":
		if id, ok := s.botNonces[nonce]; ok {
			in := s.botCreationInputs[nonce]
			if hasSecret([]byte(in.Name)) || hasSecret([]byte(in.Description)) || hasSecret([]byte(in.Alias)) {
				return ErrInvalid
			}
			out.Found, out.Input, out.Bot = true, in, s.receiptBot(id)
		}
	case "bot_name":
		if e, ok := s.botNameNonces[nonce]; ok {
			p := *e.Bot
			if hasSecret([]byte(p.Name)) {
				return ErrInvalid
			}
			out.Found, out.Input, out.Bot = true, BotNameReceiptInput{p.ID, BotNameInput{p.Name, p.DisplayRevision - 1, nonce}}, s.receiptBot(p.ID)
		}
	case "bot_profile":
		if e, ok := s.botProfileNonces[nonce]; ok {
			p := *e.BotProfile
			current, _ := s.profile(p.BotID)
			out.Found, out.Input, out.BotProfile = true, BotProfileReceiptInput{p.BotID, BotProfileInput{p.Instructions, p.IncludeHistory, p.Revision - 1, nonce}}, &current
		}
	case "project_save":
		if e, ok := s.productNonces[nonce]; ok && (e.Type == "project.created" || e.Type == "project.updated") {
			p := cloneProject(*e.Project)
			if hasSecret([]byte(p.Name)) || hasSecret([]byte(p.Goal)) {
				return ErrInvalid
			}
			id := p.ID
			if e.Type == "project.created" {
				id = ""
			}
			in := ProjectInput{ID: id, Name: p.Name, Goal: p.Goal, ChannelIDs: p.ChannelIDs, ExpectedRevision: p.Revision - 1, ClientNonce: nonce}
			current := cloneProject(s.projects[p.ID])
			out.Found, out.Input, out.Project = true, in, &current
		}
	case "conversation_create", "conversation_archive":
		if e, ok := s.conversationNonces[nonce]; ok {
			in := *e.ConversationInput
			if out.Kind == "conversation_create" && in.Kind == "create" {
				out.Found, out.Input = true, ConversationCreateInput{BotID: in.BotID, Title: in.Title, ExpectedRevision: in.ExpectedRevision, ClientNonce: nonce}
			} else if out.Kind == "conversation_archive" && in.Kind == "archive" {
				out.Found, out.Input = true, ConversationArchiveReceiptInput{in.ID, ConversationArchiveInput{Archived: in.Archived, ExpectedRevision: in.ExpectedRevision, ClientNonce: nonce}}
			}
			if out.Found {
				current, _ := s.conversation(e.Conversation.ID)
				out.Conversation = &current
			}
		}
	}
	if out.Bot != nil && (hasSecret([]byte(out.Bot.Name)) || hasSecret([]byte(out.Bot.Alias)) || hasSecret([]byte(out.Bot.Description))) {
		return ErrInvalid
	}
	if out.Project != nil && (hasSecret([]byte(out.Project.Name)) || hasSecret([]byte(out.Project.Goal))) {
		return ErrInvalid
	}
	if out.BotProfile != nil && hasSecret([]byte(out.BotProfile.Instructions)) {
		return ErrInvalid
	}
	if out.Conversation != nil && hasSecret([]byte(out.Conversation.Title)) {
		return ErrInvalid
	}
	return nil
}
