//go:build !neko_admission_http_fixture

package runtime

import "nekoaurora.local/native/internal/ledger"

func admissionHTTPFixtureObserve(string, ledger.Work, ledger.CoreBinding, uint64) {}
