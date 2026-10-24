package aurora

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTooLongMessageRefusedNotTruncated(t *testing.T) {
	svc, _ := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	url := srv.URL + "/app/api/agents/" + firstBot(svc) + "/messages"
	body := `{"text":"` + strings.Repeat("á", maxMsgRunes+1) + `","clientNonce":"long-msg-1"}`
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	r, err := http.DefaultClient.Do(req)
	if err != nil || r.StatusCode != 413 {
		t.Fatalf("want 413, got %v %v", r.StatusCode, err)
	}
	if n := len(svc.view().messages(firstBot(svc), 0)); n != 0 {
		t.Fatalf("stored %d messages", n)
	}
}
