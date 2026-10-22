package aurora

import (
	"log"
	"strconv"

	"nekoaurora.local/native/internal/ledger"
)

// record appends one line to the journaled activity log (aurora.activity).
func (s *Service) record(botID, workID, kind, text string) {
	if s.L.ReadOnly() {
		return
	}
	if err := s.L.RecordAuroraActivity(ledger.AuroraActivity{BotID: botID, WorkID: workID, Kind: kind, Text: clean(s.V.Redact(text), 200)}); err != nil {
		log.Printf("aurora activity: %v", err)
	}
}

func (s *Service) activity(limit int) []Activity {
	out := []Activity{}
	for _, a := range s.L.AuroraActivityLog("", limit) {
		out = append(out, Activity{ID: a.At + ":" + a.Kind + ":" + strconv.Itoa(len(a.Text)), AgentID: a.BotID, Text: a.Text, T: ts(a.At)})
	}
	return out
}
