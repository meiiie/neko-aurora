package duet

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSplitApproval(t *testing.T) {
	txt, a := SplitApproval("Đã soạn xong.\nAPPROVAL: {\"title\":\"Gửi Elena\",\"kind\":\"send\",\"body\":\"Chào Elena\"}")
	if a == nil || a.Title != "Gửi Elena" || a.State != "pending" || txt != "Đã soạn xong." {
		t.Fatalf("%q %+v", txt, a)
	}
	if _, a := SplitApproval("không có gì"); a != nil {
		t.Fatal("unexpected approval")
	}
	if _, a := SplitApproval("APPROVAL: {broken"); a != nil {
		t.Fatal("broken json must not create approval")
	}
}

func TestDecideOnce(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ag := s.Agents()[0]
	m := &Message{AgentID: ag.ID, Role: "agent", Text: "x", Approval: &Approval{Title: "t", Body: "b", State: "pending"}}
	if err := s.AddMessage(m); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Decide(m.ID, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Decide(m.ID, "declined"); err != ErrConflict {
		t.Fatalf("second decision must conflict, got %v", err)
	}
	if len(s.Pending()) != 0 {
		t.Fatal("pending left")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenStore(dir)
	a, err := s.CreateAgent("Tester", "d", Avatar{1, "cap", "round", "bow"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAgent("bad", "", Avatar{99, "x", "y", "z"}); err != ErrInvalid {
		t.Fatal("invalid avatar accepted")
	}
	s2, _ := OpenStore(dir)
	if _, err := s2.Agent(a.ID); err != nil {
		t.Fatal("not persisted")
	}
}

func TestPushEncryptRoundTrip(t *testing.T) {
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)
	var sub PushSub
	sub.Keys.P256dh, sub.Keys.Auth = b64.EncodeToString(ua.PublicKey().Bytes()), b64.EncodeToString(auth)
	body, err := Encrypt(sub, []byte(`{"title":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	salt, idlen := body[:16], int(body[20])
	asPub, ct := body[21:21+idlen], body[21+idlen:]
	pk, _ := ecdh.P256().NewPublicKey(asPub)
	shared, _ := ua.ECDH(pk)
	prkKey, _ := hkdf.Extract(sha256.New, shared, auth)
	ikm, _ := hkdf.Expand(sha256.New, prkKey, "WebPush: info\x00"+string(ua.PublicKey().Bytes())+string(asPub), 32)
	prk, _ := hkdf.Extract(sha256.New, ikm, salt)
	cek, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	blk, _ := aes.NewCipher(cek)
	g, _ := cipher.NewGCM(blk)
	pt, err := g.Open(nil, nonce, ct, nil)
	if err != nil || string(pt) != "{\"title\":\"hi\"}\x02" {
		t.Fatalf("decrypt %q %v", pt, err)
	}
}

// fake OpenAI-compatible streaming endpoint
func fakeAI(t *testing.T, reply string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, part := range strings.SplitAfter(reply, " ") {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", part)
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

func TestTurnStreamsAndGates(t *testing.T) {
	fa := fakeAI(t, "Mình đã soạn thư.\nAPPROVAL: {\"title\":\"Gửi thư cho Elena\",\"kind\":\"send\",\"body\":\"Chào Elena\"}")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "test-key")
	t.Setenv("DUET_AI_BASE_URL", fa.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc, err := New(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(svc)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/duet/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	ag := svc.Store.Agents()[0]
	r2, _ := http.Post(srv.URL+"/duet/api/agents/"+ag.ID+"/messages", "application/json", strings.NewReader(`{"text":"Gửi thư cho Elena"}`))
	if r2.StatusCode != 202 {
		t.Fatal(r2.StatusCode)
	}
	sc := bufio.NewScanner(res.Body)
	deltas, gotMsg := 0, false
	deadline := time.After(5 * time.Second)
	done := make(chan struct{})
	go func() {
		for sc.Scan() {
			l := sc.Text()
			if l == "event: delta" {
				deltas++
			}
			if strings.HasPrefix(l, "data:") && strings.Contains(l, `"approval"`) && strings.Contains(l, "pending") {
				gotMsg = true
				close(done)
				return
			}
		}
	}()
	select {
	case <-done:
	case <-deadline:
		t.Fatal("no approval message over SSE")
	}
	if deltas < 2 || !gotMsg {
		t.Fatalf("deltas=%d", deltas)
	}
	p := svc.Store.Pending()
	if len(p) != 1 || strings.Contains(p[0].Text, "APPROVAL") {
		t.Fatalf("pending %+v", p)
	}
	ok, _ := http.Post(srv.URL+"/duet/api/approvals/"+p[0].ID, "application/json", strings.NewReader(`{"decision":"approved"}`))
	dup, _ := http.Post(srv.URL+"/duet/api/approvals/"+p[0].ID, "application/json", strings.NewReader(`{"decision":"declined"}`))
	if ok.StatusCode != 200 || dup.StatusCode != 409 {
		t.Fatalf("decide %d dup %d", ok.StatusCode, dup.StatusCode)
	}
}

func TestNoKeyIsHonest(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc, _ := New(ctx, t.TempDir())
	ag := svc.Store.Agents()[0]
	svc.Store.AddMessage(&Message{AgentID: ag.ID, Role: "user", Text: "hi"})
	svc.runTurn(ag.ID)
	ms, _ := svc.Store.Messages(ag.ID, 0)
	if last := ms[len(ms)-1]; last.Role != "system" || !strings.Contains(last.Text, "ZAI_API_KEY") {
		t.Fatalf("%+v", last)
	}
}

func TestEmailApprovalGuards(t *testing.T) {
	t.Setenv("DUET_SMTP_HOST", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc, _ := New(ctx, t.TempDir())
	ag := svc.Store.Agents()[0]
	_, a := SplitApproval("ok\nAPPROVAL: {\"title\":\"Chào\",\"kind\":\"email\",\"to\":\"x@example.com\",\"subject\":\"Hi\",\"body\":\"Xin chào\"}")
	if a == nil || a.To != "x@example.com" || a.Subject != "Hi" {
		t.Fatalf("%+v", a)
	}
	m := &Message{AgentID: ag.ID, Role: "agent", Approval: a}
	svc.Store.AddMessage(m)
	svc.Store.Decide(m.ID, "approved")
	if txt := svc.deliverEmail(m); !strings.Contains(txt, "không phải do bạn nhập") {
		t.Fatal("recipient guard missing: " + txt)
	}
	svc.Store.AddMessage(&Message{AgentID: ag.ID, Role: "user", Text: "gửi cho x@example.com"})
	if txt := svc.deliverEmail(m); !strings.Contains(txt, "hàng đợi") {
		t.Fatal("expected queued: " + txt)
	}
	if ob := svc.Store.Outbox(); len(ob) != 1 || ob[0].Approval.Delivery.Status != "queued" {
		t.Fatalf("%+v", ob)
	}
}
