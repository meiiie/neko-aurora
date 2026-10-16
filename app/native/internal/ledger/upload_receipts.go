package ledger

type UploadReceiptIdentity struct {
	ProjectID       string `json:"projectId"`
	ProjectRevision uint64 `json:"projectRevision"`
	ChannelID       string `json:"channelId"`
	ClientNonce     string `json:"clientNonce"`
	Filename        string `json:"filename"`
	SHA256          string `json:"sha256"`
	Bytes           int    `json:"bytes"`
}

type UploadReceipt struct {
	Version     int                    `json:"version"`
	Found       bool                   `json:"found"`
	ClientNonce string                 `json:"clientNonce"`
	Input       *UploadReceiptIdentity `json:"input,omitempty"`
	File        *UploadedFile          `json:"file,omitempty"`
}

func (s *Store) UploadReceipt(projectID, channelID, nonce string) (UploadReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return UploadReceipt{}, ErrUnavailable
	}
	if !resourceID(projectID, "project-") || !resourceID(channelID, "channel-") || !validAccessNonce(nonce) {
		return UploadReceipt{}, ErrInvalid
	}
	out := UploadReceipt{Version: 1, ClientNonce: nonce}
	event, ok := s.productNonces[nonce]
	if !ok {
		return out, nil
	}
	if event.Type != "file.uploaded" || event.Upload == nil || event.Upload.ProjectID != projectID || event.Upload.ChannelID != channelID {
		return UploadReceipt{}, ErrConflict
	}
	original := *event.Upload
	current, ok := s.uploads[original.ID]
	if !ok {
		return UploadReceipt{}, ErrUnavailable
	}
	current = publicFile(current)
	out.Found = true
	out.Input = &UploadReceiptIdentity{ProjectID: projectID, ProjectRevision: event.ResourceRevision, ChannelID: channelID, ClientNonce: nonce, Filename: original.Filename, SHA256: original.SHA256, Bytes: original.Bytes}
	out.File = &current
	return out, nil
}
