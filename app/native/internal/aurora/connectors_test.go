package aurora

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testAppPw = "wxyz qrst uvmn opkl"

// fakeSMTP is a STARTTLS-only SMTP sink with a self-signed cert for 127.0.0.1.
type fakeSMTP struct {
	addr, caFile string
	mu           sync.Mutex
	auth, mail   []string
	ln           net.Listener
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fake-smtp"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	cfg := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: k}}}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	f := &fakeSMTP{addr: ln.Addr().String(), caFile: ca, ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c, cfg)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(c net.Conn, cfg *tls.Config) {
	defer c.Close()
	rd, tlsOn := bufio.NewReader(c), false
	say := func(s string) { io.WriteString(c, s+"\r\n") }
	say("220 fake ESMTP")
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.Fields(line + " x")[0])
		switch cmd {
		case "EHLO", "HELO":
			if tlsOn {
				say("250-fake\r\n250 AUTH PLAIN")
			} else {
				say("250-fake\r\n250 STARTTLS")
			}
		case "STARTTLS":
			say("220 go")
			tc := tls.Server(c, cfg)
			if tc.Handshake() != nil {
				return
			}
			c, rd, tlsOn = tc, bufio.NewReader(tc), true
		case "AUTH":
			b, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.Fields(line)[2]))
			f.mu.Lock()
			f.auth = append(f.auth, string(b))
			f.mu.Unlock()
			say("235 ok")
		case "DATA":
			say("354 go")
			var sb strings.Builder
			for {
				l, err := rd.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				sb.WriteString(l)
			}
			f.mu.Lock()
			f.mail = append(f.mail, sb.String())
			f.mu.Unlock()
			say("250 queued")
		case "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func (f *fakeSMTP) got() ([]string, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.auth...), append([]string(nil), f.mail...)
}

func scanStateFor(t *testing.T, root, needle string) {
	t.Helper()
	filepath.Walk(root, func(p string, i os.FileInfo, _ error) error {
		if i != nil && !i.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), needle) {
				t.Fatalf("plaintext secret found in %s", p)
			}
		}
		return nil
	})
}

func TestSecretRequestCardStoresInVaultOnly(t *testing.T) {
	fa := scriptedAI(t, "Mình cần mật khẩu ứng dụng để gửi thư.\nSECRET: {\"name\":\"GMAIL_APP_PASSWORD\",\"label\":\"Mật khẩu ứng dụng Gmail\",\"desc\":\"Tạo ở myaccount.google.com/apppasswords\"}")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	dir := t.TempDir()
	svc, _ := newSvcIn(t, dir)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"kết nối gmail giúp mình"}`)
	waitTask(t, svc, bot)
	var card *Message
	for _, m := range svc.view().messages(bot, 0) {
		if m.Secret != nil {
			card = m
		}
	}
	if card == nil || card.Secret.Name != "GMAIL_APP_PASSWORD" || strings.Contains(card.Text, "SECRET") {
		t.Fatalf("no secret card: %+v", card)
	}
	for _, bad := range []string{`{"name":"bad name","value":"x"}`, `{"name":"OK_NAME","value":""}`, `{"name":"OK_NAME","value":"x","extra":1}`} {
		if r := post(t, srv.URL+"/app/api/secrets", bad); r.StatusCode != 400 {
			t.Fatalf("%s → %d", bad, r.StatusCode)
		}
	}
	body, _ := json.Marshal(map[string]string{"name": card.Secret.Name, "value": testAppPw, "label": card.Secret.Label, "work": card.TaskID})
	if r := post(t, srv.URL+"/app/api/secrets", string(body)); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	r, _ := srv.Client().Get(srv.URL + "/app/api/secrets")
	b, _ := io.ReadAll(r.Body)
	if !strings.Contains(string(b), "GMAIL_APP_PASSWORD") || strings.Contains(string(b), "wxyz") {
		t.Fatalf("list: %s", b)
	}
	r, _ = srv.Client().Get(srv.URL + "/app/api/state")
	b, _ = io.ReadAll(r.Body)
	if strings.Contains(string(b), "wxyz") {
		t.Fatal("state leaked secret")
	}
	scanStateFor(t, dir, testAppPw)
	scanStateFor(t, dir, strings.ReplaceAll(testAppPw, " ", ""))
	for _, a := range svc.activity(50) {
		if strings.Contains(a.Text, "wxyz") {
			t.Fatal("activity leaked")
		}
	}
}

func TestGmailVaultDeliveryAfterApproval(t *testing.T) {
	smtp := newFakeSMTP(t)
	t.Setenv("AURORA_SMTP_CA_FILE", smtp.caFile)
	t.Setenv("AURORA_GMAIL_SMTP_ADDR", smtp.addr)
	t.Setenv("AURORA_SMTP_HOST", "")
	fa := scriptedAI(t, "Đã soạn thư thử.\nAPPROVAL: {\"title\":\"Thư thử Gmail\",\"kind\":\"email\",\"to\":\"friend@example.org\",\"subject\":\"Thử\",\"body\":\"Xin chào\"}")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	dir := t.TempDir()
	svc, _ := newSvcIn(t, dir)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	if r := post(t, srv.URL+"/app/api/connectors/gmail", `{"address":"me@gmail.com","password":"short"}`); r.StatusCode != 400 {
		t.Fatal("short app password accepted")
	}
	if r := post(t, srv.URL+"/app/api/connectors/gmail", `{"address":"me@gmail.com","password":"`+testAppPw+`"}`); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	info := svc.outboundInfo()
	if g := info["gmail"].(map[string]any); g["connected"] != true || g["from"] != "me@gmail.com" {
		t.Fatalf("%+v", info)
	}
	bot := firstBot(svc)
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"gửi thư thử tới friend@example.org"}`)
	waitTask(t, svc, bot)
	p := svc.view().pending()
	if len(p) != 1 {
		t.Fatalf("pending %+v", p)
	}
	if a, m := smtp.got(); len(a)+len(m) != 0 {
		t.Fatal("sent before approval")
	}
	post(t, srv.URL+"/app/api/approvals/"+p[0].ID, `{"decision":"approved"}`)
	var mails, auths []string
	for i := 0; i < 200 && len(mails) == 0; i++ {
		time.Sleep(20 * time.Millisecond)
		auths, mails = smtp.got()
	}
	if len(mails) != 1 || !strings.Contains(mails[0], "From: me@gmail.com") || !strings.Contains(mails[0], "To: friend@example.org") {
		t.Fatalf("mail %q", mails)
	}
	if len(auths) != 1 || auths[0] != "\x00me@gmail.com\x00wxyzqrstuvmnopkl" {
		t.Fatal("auth did not use vault credentials")
	}
	for i := 0; i < 100; i++ {
		if w, _ := svc.L.Get(p[0].TaskID); w.Direct.Approval.Delivery != nil && w.Direct.Approval.Delivery.Status == "sent" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if w, _ := svc.L.Get(p[0].TaskID); w.Direct.Approval.Delivery.Status != "sent" {
		t.Fatalf("%+v", w.Direct.Approval.Delivery)
	}
	scanStateFor(t, dir, "wxyzqrstuvmnopkl")
	post(t, srv.URL+"/app/api/connectors/gmail/disconnect", `{}`)
	if svc.V.Has(gmailPass) {
		t.Fatal("disconnect kept password")
	}
}
