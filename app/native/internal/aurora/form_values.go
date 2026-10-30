package aurora

import "nekoaurora.local/native/internal/ledger"

func formAnswerText(f *form, values []string) (string, error) {
	return ledger.DirectFormAnswerText(f, values)
}
