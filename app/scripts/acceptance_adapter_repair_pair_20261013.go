package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func independentRepairAPI(t *testing.T) (*ledger.Store, *Server, ledger.AdapterSession, ledger.AdapterSession, ledger.Work) {
	t.Helper()
	store, err := ledger.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { store.Close() })
	channel, _, err := store.SaveChannel(ledger.ChannelInput{Name: "independent bounded repair", BotIDs: []string{"bot-meiiie"}, ClientNonce: "independent-repair-channel"})
	if err != nil { t.Fatal(err) }
	register := func(id string) ledger.AdapterSession {
		adapter, _, err := store.RegisterAdapter(ledger.AdapterSessionInput{SourceApp: "wiii", SourceSessionID: id, SourceThreadID: id, ChannelID: channel.ID, ExpectedChannelRevision: channel.Revision, ClientNonce: id})
		if err != nil { t.Fatal(err) }; return adapter
	}
	a, b := register("independent-source-a"), register("independent-source-b")
	work, _, err := store.AcceptAdapter(a.ID, ledger.AdapterMessageInput{Input: ledger.Input{BotID: "bot-meiiie", ClientNonce: "independent-original-nonce", Text: "private original input canary", Lane: "foreground"}, SourceMessageID: "independent-message-a"})
	if err != nil { t.Fatal(err) }
	server, err := New(store, "127.0.0.1:8123")
	if err != nil { t.Fatal(err) }
	return store, server, a, b, work
}

func independentRepairRequest(server *Server, target string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	request := httptest.NewRequest("GET", "http://127.0.0.1:8123"+target, nil)
	request.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
	if mutate != nil { mutate(request) }
	recorder := httptest.NewRecorder(); server.ServeHTTP(recorder, request); return recorder
}

func TestIndependentAdapterRepairDeniedRequestForms(t *testing.T) {
	store, server, adapter, _, original := independentRepairAPI(t)
	before := store.Snapshot().Sequence
	base := "/v1/adapters/sessions/"+adapter.ID+"/history"
	cases := []struct{name, path string; mutate func(*http.Request); status int}{
		{"empty-query", base+"?", nil, 400},
		{"duplicate-cursor", base+"?cursor=a&cursor=a", nil, 400},
		{"unknown-query", base+"?scope=other", nil, 400},
		{"encoded-cursor", base+"?cursor=%41", nil, 400},
		{"raw-path", base, func(r *http.Request){r.URL.RawPath=base}, 400},
		{"body", base, func(r *http.Request){r.Body=http.NoBody;r.ContentLength=1}, 400},
		{"chunked-body", base, func(r *http.Request){r.TransferEncoding=[]string{"chunked"}}, 400},
		{"mutation-method", base, func(r *http.Request){r.Method="POST";r.Header.Set("Content-Type","application/json")}, 404},
		{"foreign-origin", base, func(r *http.Request){r.Header.Set("Origin","http://foreign.invalid")}, 403},
	}
	for _, tc := range cases { t.Run(tc.name, func(t *testing.T) {
		response := independentRepairRequest(server,tc.path,tc.mutate)
		if response.Code != tc.status {t.Fatalf("unexpected rejection status %d",response.Code)}
		if tc.status == 400 && !strings.Contains(response.Body.String(),"adapter_history_cursor_invalid") {t.Fatal("missing closed repair error")}
		if strings.Contains(response.Body.String(),original.Text) || strings.Contains(response.Body.String(),original.Adapter.ClientNonce) {t.Fatal("rejection leaked original input authority")}
	}) }
	if store.Snapshot().Sequence != before {t.Fatal("denied GET mutated journal")}
	retained, err := store.Get(original.RequestID)
	if err != nil || !reflect.DeepEqual(retained, original) {t.Fatal("denied forms changed accepted work or nonce")}
}

func TestIndependentAdapterRepairWitnessReplayBoundaries(t *testing.T) {
	store, server, adapter, other, original := independentRepairAPI(t)
	base := "/v1/adapters/sessions/"+adapter.ID+"/history"
	before := store.Snapshot().Sequence
	response := independentRepairRequest(server,base,nil)
	if response.Code != 200 {t.Fatalf("snapshot unavailable %d",response.Code)}
	var page ledger.AdapterHistoryPage
	if err := json.Unmarshal(response.Body.Bytes(),&page); err != nil {t.Fatal(err)}
	if !page.Complete || len(page.Rows)!=1 || page.ConfirmationCursor==nil || len(*page.ConfirmationCursor)!=215 {t.Fatal("missing terminal witness")}
	if strings.Contains(response.Body.String(),original.Text) || strings.Contains(response.Body.String(),original.Adapter.ClientNonce) {t.Fatal("history exported original intake")}
	token := *page.ConfirmationCursor
	t.Run("cross-session",func(t *testing.T){
		r:=independentRepairRequest(server,"/v1/adapters/sessions/"+other.ID+"/history?cursor="+token,nil)
		if r.Code!=403 || !strings.Contains(r.Body.String(),"adapter_history_binding_mismatch") {t.Fatal("cross-session witness allowed")}
	})
	t.Run("retired-signer",func(t *testing.T){
		restarted,err:=New(store,"127.0.0.1:8123");if err!=nil{t.Fatal(err)}
		r:=independentRepairRequest(restarted,base+"?cursor="+token,nil)
		if r.Code!=400 || !strings.Contains(r.Body.String(),"adapter_history_cursor_invalid") {t.Fatal("old signer witness allowed")}
	})
	t.Run("zero-row-confirmation",func(t *testing.T){
		r:=independentRepairRequest(server,base+"?cursor="+token,nil)
		var final ledger.AdapterHistoryPage
		if r.Code!=200 || json.Unmarshal(r.Body.Bytes(),&final)!=nil || final.Offset!=1 || len(final.Rows)!=0 || final.HistoryDigest!=page.HistoryDigest {t.Fatal("confirmation replayed rows or changed digest")}
	})
	if store.Snapshot().Sequence != before {t.Fatal("witness GET changed journal")}
	_,_,err:=store.SaveChannel(ledger.ChannelInput{Name:"unrelated typed metadata",BotIDs:[]string{"bot-meiiie"},ClientNonce:"independent-epoch-change"})
	if err!=nil{t.Fatal(err)}
	after:=store.Snapshot().Sequence
	t.Run("live-epoch-change",func(t *testing.T){
		r:=independentRepairRequest(server,base+"?cursor="+token,nil)
		if r.Code!=409 || !strings.Contains(r.Body.String(),"adapter_history_authority_changed") {t.Fatal("stale epoch confirmation allowed")}
	})
	if store.Snapshot().Sequence!=after{t.Fatal("stale authority GET changed journal")}
	retained,getErr:=store.Get(original.RequestID)
	if getErr!=nil || !reflect.DeepEqual(retained,original){t.Fatal("history witness changed accepted work or nonce")}
}
