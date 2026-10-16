package protocol

import "testing"

func TestRequesterAdmissionDoesNotExpandHistoricalRuntimeAuthority(t *testing.T) {
	if !CoreRequesterAdmission("df5c0feac1073d8471e567afca2e62a4e06f19ebb7d55f4d6537502c6dea0319") {
		t.Fatal("current requester runtime missing")
	}
	for _, old := range []string{"45d6191adc9dc4b66ee46b363676eb1706c78c01818946b570087037aaf94e52", "97ca4c8acff2e0bb6dc0b9577d856b3601d4c6c914579a9c9a1fe3cbb67c47fb", "unknown"} {
		if CoreRequesterAdmission(old) {
			t.Fatal("historical runtime acquired requester authority")
		}
	}
}

func TestPublishedReportRuntimeAndPriorContextReceipts(t *testing.T) {
	for _, digest := range []string{
		"45d6191adc9dc4b66ee46b363676eb1706c78c01818946b570087037aaf94e52",
		"045a5a81e9874c0735f3da5e8642137a5994cd1fffd69024d5401812c37c9c4f",
		"97ca4c8acff2e0bb6dc0b9577d856b3601d4c6c914579a9c9a1fe3cbb67c47fb",
	} {
		if !CoreRuntimeAllowed(digest) || !CoreContextAdmission(digest) || !CorePriorReceiptRetry(digest) {
			t.Fatal("current or historical context1 receipt lost exact runtime validation")
		}
	}
}

func TestHistoricalRuntimeReceiptsRemainReplayable(t *testing.T) {
	for _, digest := range []string{
		"fa48d43f59257dcd171441f09a8ac53d3702485ddf7f80ac13a0796d53ac6310",
		"2a618df35470fb54d6058c44275999f00a95f55c2435f3aadafe19cc6b791904",
	} {
		if !CoreRuntimeAllowed(digest) || !CorePriorReceiptRetry(digest) {
			t.Fatalf("writer-v2 receipt lost historical validation: %s", digest)
		}
	}
	v1 := "4b3fb6202551912f86b5dd5faffccf75d854174c2ddd6b9fb331470a2b2e0b36"
	if !CoreRuntimeAllowed(v1) || CorePriorReceiptRetry(v1) {
		t.Fatal("legacy replay must not acquire writer-v2 retry authority")
	}
	if CoreRuntimeAllowed("unknown") || CorePriorReceiptRetry("unknown") {
		t.Fatal("unknown runtime accepted")
	}
}
