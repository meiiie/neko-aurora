package ledger

import "nekobot.local/native/internal/historyexport"

func sensitiveHistory(h historyexport.History) bool {
	if hasSecret([]byte(h.Title)) {
		return true
	}
	for _, m := range h.Messages {
		if hasSecret([]byte(m.Text)) || hasSecret([]byte(m.Reply)) {
			return true
		}
		for _, f := range m.Files {
			if hasSecret([]byte(f.Name)) {
				return true
			}
		}
	}
	return false
}
