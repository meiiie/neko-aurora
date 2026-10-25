package ledger

import "nekoaurora.local/native/internal/identity"

const (
	NetworkMaxFiles           = 8
	NetworkMaxFileBytes       = 32 << 10
	NetworkMaxAttachmentBytes = 64 << 10
	NetworkMaxTextBytes       = 16 << 10
	NetworkMaxPending         = 256
)

type NetworkReadScope struct {
	Principal     *identity.VerifiedPrincipal `json:"-"`
	ActorID       string                      `json:"-"`
	SourceWorkID  string                      `json:"-"`
	SourceEpoch   uint64                      `json:"-"`
	SourceAttempt string                      `json:"-"`
}

type NetworkSessionInput struct {
	BotID                   string           `json:"botId"`
	ConversationID          string           `json:"conversationId"`
	DeviceID                string           `json:"deviceId"`
	Alias                   string           `json:"alias"`
	Tier                    string           `json:"tier"`
	ClientNonce             string           `json:"clientNonce"`
	ExpectedChannelRevision uint64           `json:"expectedChannelRevision"`
	Scope                   NetworkReadScope `json:"-"`
}

type NetworkSession struct {
	ID              string                      `json:"id"`
	InstanceID      string                      `json:"instanceId"`
	OwnerUserID     string                      `json:"ownerUserId"`
	SpaceID         string                      `json:"spaceId"`
	BotID           string                      `json:"botId"`
	ConversationID  string                      `json:"conversationId"`
	DeviceID        string                      `json:"deviceId"`
	Alias           string                      `json:"alias"`
	Tier            string                      `json:"tier"`
	ChannelRevision uint64                      `json:"channelRevision"`
	Revision        uint64                      `json:"revision"`
	ClientNonce     string                      `json:"clientNonce"`
	ActorID         string                      `json:"actorId"`
	RequesterPin    *identity.VerifiedPrincipal `json:"requesterPin,omitempty"`
}

type NetworkAttachment struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type NetworkMessageInput struct {
	ToSessionID string              `json:"toSessionId"`
	Text        string              `json:"text"`
	ClientNonce string              `json:"clientNonce"`
	ReplyTo     string              `json:"replyTo,omitempty"`
	Wake        bool                `json:"wake"`
	Attachments []NetworkAttachment `json:"attachments,omitempty"`
	Scope       NetworkReadScope    `json:"-"`
}

type NetworkMessage struct {
	ID              string                      `json:"id"`
	FromSessionID   string                      `json:"fromSessionId"`
	ToSessionID     string                      `json:"toSessionId"`
	FromBotID       string                      `json:"fromBotId,omitempty"`
	ToBotID         string                      `json:"toBotId"`
	ConversationID  string                      `json:"conversationId"`
	ActorID         string                      `json:"actorId"`
	Origin          string                      `json:"origin"`
	SourceWorkID    string                      `json:"sourceWorkId,omitempty"`
	SourceEpoch     uint64                      `json:"sourceEpoch,omitempty"`
	SourceAttempt   string                      `json:"sourceAttempt,omitempty"`
	RootWorkID      string                      `json:"rootWorkId,omitempty"`
	Text            string                      `json:"text"`
	ClientNonce     string                      `json:"clientNonce"`
	ReplyTo         string                      `json:"replyTo,omitempty"`
	CreatedAt       string                      `json:"createdAt"`
	ExpiresAt       string                      `json:"expiresAt"`
	ChannelRevision uint64                      `json:"channelRevision"`
	CreatedSequence uint64                      `json:"createdSequence"`
	Wake            bool                        `json:"wake"`
	Attachments     []NetworkAttachment         `json:"attachments,omitempty"`
	RequesterPin    *identity.VerifiedPrincipal `json:"requesterPin,omitempty"`
}

type NetworkApproval struct {
	WorkID    string `json:"workId"`
	State     string `json:"state"`
	DecidedAt string `json:"decidedAt"`
	Digest    string `json:"digest"`
}

type NetworkMessageView struct {
	NetworkMessage
	DeliveryState string           `json:"deliveryState"`
	TaskState     string           `json:"taskState,omitempty"`
	WorkID        string           `json:"workId,omitempty"`
	ReadAt        string           `json:"readAt,omitempty"`
	Approval      *NetworkApproval `json:"approval,omitempty"`
}

type NetworkSnapshot struct {
	Sessions []NetworkSession     `json:"sessions"`
	Messages []NetworkMessageView `json:"messages"`
}

type NetworkDelivery struct {
	WorkID     string   `json:"workId"`
	MessageIDs []string `json:"messageIds"`
	At         string   `json:"at"`
}

type NetworkControl struct {
	MessageID    string                      `json:"messageId"`
	Action       string                      `json:"action"`
	ActorID      string                      `json:"actorId"`
	ClientNonce  string                      `json:"clientNonce"`
	At           string                      `json:"at"`
	RequesterPin *identity.VerifiedPrincipal `json:"requesterPin,omitempty"`
}

type NetworkEvent struct {
	Session  *NetworkSession  `json:"session,omitempty"`
	Message  *NetworkMessage  `json:"message,omitempty"`
	Delivery *NetworkDelivery `json:"delivery,omitempty"`
	Control  *NetworkControl  `json:"control,omitempty"`
}

func NetworkAttempt(w Work) string {
	if w.AttemptID != "" {
		return w.AttemptID
	}
	if w.Direct != nil {
		return w.Direct.StartedAt
	}
	return ""
}
