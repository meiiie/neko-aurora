package aurora

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

func TestMain(m *testing.M) {
	os.Setenv("AURORA_BRIEF_LOOP", "off")
	os.Exit(m.Run())
}

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

func scriptedAI(t *testing.T, replies ...string) *httptest.Server {
	n := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rep := replies[len(replies)-1]
		if n < len(replies) {
			rep = replies[n]
		}
		n++
		w.Header().Set("Content-Type", "text/event-stream")
		rep, cut := strings.CutSuffix(rep, "<<CUT>>") // provider stops at max_tokens
		for _, part := range strings.SplitAfter(rep, " ") {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", part)
			w.(http.Flusher).Flush()
		}
		if cut {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

func newSvc(t *testing.T) (*Service, *ledger.Store) { t.Helper(); return newSvcIn(t, t.TempDir()) }

func newSvcIn(t *testing.T, dir string) (*Service, *ledger.Store) {
	t.Helper()
	st, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc, err := New(ctx, st, filepath.Join(dir, "aurora"), Options{RunTurns: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); svc.Wait() }) // join running turns before the temp dir goes
	return svc, st
}

func firstBot(svc *Service) string { return svc.L.Snapshot().Bots[0].ID }

func waitTask(t *testing.T, svc *Service, bot string) {
	t.Helper()
	for i := 0; i < 400; i++ {
		busy := false
		for _, w := range svc.view().byBot[bot] {
			if active(w) {
				busy = true
			}
		}
		if !busy {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("still running")
}

func post(t *testing.T, url, body string) *http.Response {
	r, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestTurnIsLedgerWorkAndGates(t *testing.T) {
	fa := scriptedAI(t, "Mình đã soạn thư.\nAPPROVAL: {\"title\":\"Gửi thư cho Elena\",\"kind\":\"send\",\"body\":\"Chào Elena\"}")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "test-key")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, st := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	if r := post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"Gửi thư cho Elena"}`); r.StatusCode != 202 {
		t.Fatal(r.StatusCode)
	}
	waitTask(t, svc, bot)
	// the turn is the control plane's own task: desktop sees the same reply
	var w ledger.Work
	for _, x := range st.Snapshot().Work {
		if x.BotID == bot {
			w = x
		}
	}
	if w.Status != "completed" || w.Direct == nil || w.Direct.Runtime != "direct-http" || !strings.Contains(w.Reply, "soạn thư") || strings.Contains(w.Reply, "APPROVAL") {
		t.Fatalf("%+v", w)
	}
	p := svc.view().pending()
	if len(p) != 1 {
		t.Fatalf("pending %+v", p)
	}
	ok := post(t, srv.URL+"/app/api/approvals/"+p[0].ID, `{"decision":"approved"}`)
	dup := post(t, srv.URL+"/app/api/approvals/"+p[0].ID, `{"decision":"declined"}`)
	if ok.StatusCode != 200 || dup.StatusCode != 409 {
		t.Fatalf("decide %d dup %d", ok.StatusCode, dup.StatusCode)
	}
}

func TestNoKeyIsHonest(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "")
	t.Setenv("AURORA_AI_BASE_URL", "http://127.0.0.1:1")
	svc, _ := newSvc(t)
	bot := firstBot(svc)
	if _, _, err := svc.L.Accept(ledger.Input{BotID: bot, ClientNonce: "nokey-12345678", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.poke()
	waitTask(t, svc, bot)
	ms := svc.view().messages(bot, 0)
	if last := ms[len(ms)-1]; last.Role != "system" || !strings.Contains(last.Text, "ZAI_API_KEY") {
		t.Fatalf("%+v", last)
	}
}

func TestPlanStepsAndEditApprove(t *testing.T) {
	fa := scriptedAI(t,
		"Mình sẽ làm 2 bước.\nPLAN: [\"Tìm ý chính\",\"Soạn thư\"]",
		"Ý chính: giới thiệu sản phẩm.",
		"Thư đã soạn.\nAPPROVAL: {\"title\":\"Gửi thư\",\"kind\":\"email\",\"to\":\"a@b.co\",\"subject\":\"Chào\",\"body\":\"Nội dung\"}")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	t.Setenv("AURORA_SMTP_HOST", "")
	svc, _ := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"gửi thư cho a@b.co"}`)
	waitTask(t, svc, bot)
	var plan *Message
	for _, m := range svc.view().messages(bot, 0) {
		if m.Plan != nil {
			plan = m
		}
	}
	if plan == nil || plan.Plan.Steps[0].Status != "done" || plan.Plan.Steps[1].Status != "waiting" {
		t.Fatalf("plan %+v", plan)
	}
	p := svc.view().pending()
	if len(p) != 1 || p[0].Approval.To != "a@b.co" {
		t.Fatalf("pending %+v", p)
	}
	if r := post(t, srv.URL+"/app/api/approvals/"+p[0].ID, `{"decision":"approved","body":"Nội dung đã sửa","subject":"Chào bạn"}`); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	w, _ := svc.L.Get(p[0].TaskID)
	a := w.Direct.Approval
	if a.Body != "Nội dung đã sửa" || a.Subject != "Chào bạn" || a.Delivery == nil || a.Delivery.Status != "queued" {
		t.Fatalf("%+v", a)
	}
}

func TestEmailRecipientGuard(t *testing.T) {
	fa := scriptedAI(t, "ok\nAPPROVAL: {\"title\":\"Chào\",\"kind\":\"email\",\"to\":\"x@example.com\",\"subject\":\"Hi\",\"body\":\"Xin chào\"}")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	t.Setenv("AURORA_SMTP_HOST", "")
	svc, _ := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"chào ai đó giúp mình"}`)
	waitTask(t, svc, bot)
	p := svc.view().pending()
	post(t, srv.URL+"/app/api/approvals/"+p[0].ID, `{"decision":"approved"}`)
	w, _ := svc.L.Get(p[0].TaskID)
	if d := w.Direct.Approval.Delivery; d == nil || d.Status != "failed" || d.Detail != "recipient_not_from_owner" {
		t.Fatalf("%+v", d)
	}
}

func TestSteeringSupersedesAndStop(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 200; i++ {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x \"}}]}\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(30 * time.Millisecond):
			}
		}
	}))
	defer slow.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", slow.URL)
	svc, _ := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"việc dài"}`)
	time.Sleep(400 * time.Millisecond)
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"thêm việc"}`)
	ms := svc.view().messages(bot, 0)
	if !ms[len(ms)-1].Queued {
		t.Fatalf("follow-up not marked queued %+v", ms[len(ms)-1])
	}
	post(t, srv.URL+"/app/api/agents/"+bot+"/stop", `{}`)
	waitTask(t, svc, bot)
	v := svc.view()
	b, _ := v.bot(bot)
	if a := v.agent(b, botPrefs{}); a.Status != "idle" {
		t.Fatalf("status %s %s", a.Status, a.Snip)
	}
}

func TestSupersedePending(t *testing.T) {
	fa := scriptedAI(t, "ok\nAPPROVAL: {\"title\":\"t\",\"kind\":\"send\",\"body\":\"b\"}", "đã đổi")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"soạn"}`)
	waitTask(t, svc, bot)
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"đổi ý"}`)
	if len(svc.view().pending()) != 0 {
		t.Fatal("stale approval not superseded")
	}
}

func TestLooksPushAndActivityAreJournaled(t *testing.T) {
	fa := scriptedAI(t, "Xong việc.")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "test-key")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "aurora"), 0o700)
	var bot string
	for round := 0; round < 2; round++ {
		st, err := ledger.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if round == 0 {
			bot = st.Snapshot().Bots[0].ID
			// a pre-ledger prefs.json is imported once
			os.WriteFile(filepath.Join(dir, "aurora", "prefs.json"), []byte(`{"bots":{"`+bot+`":{"av":{"shell":2,"hat":"cap","eyes":"round","extra":"none"},"pin":true,"tag":"thư"}},"push":[{"endpoint":"https://push.example/x","keys":{"p256dh":"BAAA","auth":"q1"}}]}`), 0o600)
		}
		ctx, cancel := context.WithCancel(context.Background())
		svc, err := New(ctx, st, filepath.Join(dir, "aurora"), Options{RunTurns: round == 0})
		if err != nil {
			t.Fatal(err)
		}
		p := svc.Prefs.Bot(bot)
		if !p.Pinned || p.Tag != "thư" || p.Avatar == nil || p.Avatar.Hat != "cap" || len(svc.Prefs.PushSubs()) != 1 {
			t.Fatalf("round %d look %+v push %d", round, p, len(svc.Prefs.PushSubs()))
		}
		if round == 0 {
			srv := httptest.NewServer(svc)
			post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"Làm giúp mình"}`)
			waitTask(t, svc, bot)
			srv.Close()
		}
		acts := svc.activity(10)
		if len(acts) < 2 || !strings.Contains(acts[len(acts)-1].Text, "Bạn giao cho") {
			t.Fatalf("round %d activity %+v", round, acts)
		}
		cancel()
		st.Close()
	}
	if _, err := os.Stat(filepath.Join(dir, "aurora", "prefs.json")); !os.IsNotExist(err) {
		t.Fatal("prefs.json still live")
	}
}

func TestAlias(t *testing.T) {
	if a := alias("Thư ký Neko", map[string]bool{}); a != "thu-ky-neko" {
		t.Fatal(a)
	}
	if a := alias("Thư ký Neko", map[string]bool{"thu-ky-neko": true}); a != "thu-ky-neko-2" {
		t.Fatal(a)
	}
	if a := alias("123", map[string]bool{}); a != "agent-123" {
		t.Fatal(a)
	}
}

func TestCoreStreamStripsUsage(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "bun")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf 'Xong.\\nAPPROVAL: {\"title\":\"t\",\"kind\":\"send\",\"body\":\"b\"}\\n[session cumulative: 9 tokens]\\nmore\\n'\n"), 0o755)
	c := &Core{Bun: script, Entry: "x", Home: dir, Model: "glm-5.3", Profile: "zai", key: "k"}
	var streamed strings.Builder
	out, err := c.Stream(context.Background(), []ChatMsg{{"user", "hi"}}, func(d string) { streamed.WriteString(d) })
	if err != nil || strings.Contains(out, "session cumulative") || strings.Contains(streamed.String(), "session") {
		t.Fatalf("%v %q %q", err, out, streamed.String())
	}
	if d := ParseDirectives(out); d.Approval == nil || d.Text != "Xong." {
		t.Fatalf("%+v", d)
	}
}

func TestGroupChatFansOutToMembers(t *testing.T) {
	var seen []string
	var mu sync.Mutex
	fa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, string(b))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Phần của mình xong.\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "test-key")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, st := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bots := st.Snapshot().Bots
	r := post(t, srv.URL+"/app/api/groups", `{"name":"Đội ra mắt","bots":["`+bots[0].ID+`","`+bots[1].ID+`"]}`)
	var g Agent
	json.NewDecoder(r.Body).Decode(&g)
	if r.StatusCode != 201 || g.Kind != "group" || len(g.Members) != 2 {
		t.Fatalf("create %d %+v", r.StatusCode, g)
	}
	if r := post(t, srv.URL+"/app/api/agents/"+g.ID+"/messages", `{"text":"@tất cả lên kế hoạch ra mắt"}`); r.StatusCode != 202 {
		t.Fatal(r.StatusCode)
	}
	for i := 0; i < 300; i++ {
		busy := false
		for _, w := range svc.view().byConv[g.ID] {
			busy = busy || active(w)
		}
		if !busy {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	msgs := svc.view().groupMessages(g.ID, 0)
	users, from := 0, map[string]bool{}
	for _, m := range msgs {
		if m.Role == "user" {
			users++
		}
		if m.Role == "agent" {
			from[m.From] = true
		}
		if m.AgentID != g.ID {
			t.Fatal("message not in group thread")
		}
	}
	if users != 1 || len(from) != 2 {
		t.Fatalf("users=%d from=%v msgs=%+v", users, from, msgs)
	}
	// a second message lets each bot see the other's reply
	post(t, srv.URL+"/app/api/agents/"+g.ID+"/messages", `{"text":"Tiếp tục"}`)
	time.Sleep(600 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(strings.Join(seen, "\n"), "trong nhóm]") {
		t.Fatal("group history missing other bots' replies")
	}
}
