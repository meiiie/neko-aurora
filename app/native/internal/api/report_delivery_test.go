package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestReportDeliveryAuthenticatedDurableClarification(t *testing.T) {
	store, server, client, channel, _ := reportAPIFixture(t, false)
	input := apiReportInput(channel, "report-delivery-baseline-no-source")
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot().Sequence
	r := post(t, client, server.URL, "/v1/report-deliveries", string(body), "")
	defer r.Body.Close()
	if r.StatusCode != http.StatusCreated {
		t.Fatal("authorized report delivery endpoint is missing or rejected", r.StatusCode)
	}
	var response struct {
		Delivery struct {
			Status   string `json:"status"`
			Question string `json:"question"`
		} `json:"delivery"`
	}
	if err = json.NewDecoder(r.Body).Decode(&response); err != nil || response.Delivery.Status != "needs_clarification" || response.Delivery.Question == "" || store.Snapshot().Sequence != before+1 {
		t.Fatal("authorized no-source command was not durably clarified", err)
	}
}
