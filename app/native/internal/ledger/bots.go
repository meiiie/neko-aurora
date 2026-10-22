package ledger

import (
	"reflect"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type BotInput struct {
	Name        string `json:"name"`
	Alias       string `json:"alias"`
	Description string `json:"description"`
	ClientNonce string `json:"clientNonce"`
}

var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)
var mentionPattern = regexp.MustCompile(`(?i)^@([a-z][a-z0-9_-]{1,31})(?:[ \t]+|$)`)
var botIDPattern = regexp.MustCompile(`^bot-[a-f0-9]{32}$`)

func botText(value string, limit int) bool {
	if !utf8.ValidString(value) || len(value) > limit {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
// MaxActiveBots caps live (non-archived) bots; archived ones only count toward
// the much larger MaxTotalBots bound that keeps the ledger finite.
const (
	MaxActiveBots = 32
	MaxTotalBots  = 256
)

func (s *Store) activeBotCount() int {
	n := 0
	for _, b := range s.bots {
		if !s.archivedBot(b.ID) {
			n++
		}
	}
	return n
}
func validBotInput(in BotInput) bool {
	return in.Name == strings.TrimSpace(in.Name) && in.Name != "" && botText(in.Name, 80) && aliasPattern.MatchString(in.Alias) && botText(in.Description, 240) && regexp.MustCompile(`^[a-zA-Z0-9_-]{8,128}$`).MatchString(in.ClientNonce)
}
func (s *Store) validBotEvent(e Event) bool {
	if e.Bot == nil || !reflect.DeepEqual(e.Work, Work{}) || !validBotInput(BotInput{e.Bot.Name, e.Bot.Alias, e.Bot.Description, e.Nonce}) || !botIDPattern.MatchString(e.Bot.ID) || e.Bot.Revision != 1 || e.Bot.DisplayRevision != 0 || e.Bot.Color != "#7384d8" || s.activeBotCount() >= MaxActiveBots || len(s.bots) >= MaxTotalBots {
		return false
	}
	if _, exists := s.botNonces[e.Nonce]; exists {
		return false
	}
	if _, exists := s.botNameNonces[e.Nonce]; exists {
		return false
	}
	if _, exists := s.botProfileNonces[e.Nonce]; exists {
		return false
	}
	if _, state := s.botStateNonces[e.Nonce]; state {
		return false
	}
	for _, bot := range s.bots {
		if bot.ID == e.Bot.ID || bot.Alias == e.Bot.Alias || strings.EqualFold(bot.Name, e.Bot.Name) {
			return false
		}
	}
	return true
}
func (s *Store) CreateBot(in BotInput) (Bot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createBotLocked(in)
}
func (s *Store) createBotLocked(in BotInput) (Bot, bool, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Alias = strings.ToLower(strings.TrimSpace(in.Alias))
	in.Description = strings.TrimSpace(in.Description)
	if !validBotInput(in) {
		return Bot{}, false, ErrInvalid
	}
	if s.closed || s.poisoned {
		return Bot{}, false, ErrUnavailable
	}
	if _, state := s.botStateNonces[in.ClientNonce]; state {
		return Bot{}, false, ErrConflict
	}
	if _, used := s.botNameNonces[in.ClientNonce]; used {
		return Bot{}, false, ErrConflict
	}
	if _, used := s.botProfileNonces[in.ClientNonce]; used {
		return Bot{}, false, ErrConflict
	}
	if id, exists := s.botNonces[in.ClientNonce]; exists {
		for _, bot := range s.bots {
			if bot.ID == id {
				if !reflect.DeepEqual(s.botCreationInputs[in.ClientNonce], in) {
					return Bot{}, false, ErrConflict
				}
				return bot, true, nil
			}
		}
	}
	id, err := newID("bot-")
	if err != nil {
		return Bot{}, false, err
	}
	bot := Bot{ID: id, Name: in.Name, Alias: in.Alias, Description: in.Description, Color: "#7384d8", Revision: 1}
	event := Event{Type: "bot.created", Bot: &bot, Nonce: in.ClientNonce}
	if !s.validBotEvent(event) {
		return Bot{}, false, ErrConflict
	}
	if err = s.commit(event); err != nil {
		return Bot{}, false, err
	}
	return bot, false, nil
}
func workNonceKey(w Work) string {
	if w.RequesterPin != nil {
		return scopeNonceKey(Input{ClientNonce: w.ClientNonce}, w.ConversationID, w.RequesterPin)
	}
	bot := w.BotID
	if w.SourceBotID != "" {
		bot = w.SourceBotID
	}
	if w.Publication != nil && (validChannelID(w.Publication.SourceChannelID) || privateConversationID(w.Publication.SourceChannelID)) {
		return nonceKey(w.Publication.SourceChannelID+"|"+bot, w.ClientNonce)
	}
	return nonceKey(bot, w.ClientNonce)
}
func (s *Store) resolveRoute(text string) (string, string, error) {
	match := mentionPattern.FindStringSubmatch(strings.TrimSpace(text))
	if len(match) == 0 {
		return "", "", nil
	}
	alias := strings.ToLower(match[1])
	for _, bot := range s.bots {
		if bot.Alias == alias {
			return bot.ID, alias, nil
		}
	}
	return "", "", ErrNotFound
}
func (s *Store) validRoute(w Work) bool {
	if w.SourceBotID == "" {
		return w.RoutedAlias == ""
	}
	if s.validInput(Input{w.SourceBotID, w.ClientNonce, w.Text, w.Lane}) != nil || w.SourceBotID == w.BotID {
		return false
	}
	target, alias, err := s.resolveRoute(w.Text)
	return err == nil && target == w.BotID && alias == w.RoutedAlias
}
