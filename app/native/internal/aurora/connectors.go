package aurora

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/outbound"
	"nekoaurora.local/native/internal/vault"
)

// Connectors: real outbound channels configured from the encrypted vault.
// Gmail = SMTP with an app password (smtp.gmail.com:587, STARTTLS); generic
// SMTP = any host. The vault wins over the AURORA_SMTP_* env fallback. All
// routes sit behind the owner session + JSON-only gate of the host server
// (local: neko_local SameSite=Strict cookie; public: owner cookie + Origin).

const (
	gmailAddr = "GMAIL_ADDRESS"
	gmailPass = "GMAIL_APP_PASSWORD"
	smtpHost  = "SMTP_HOST"
	smtpPort  = "SMTP_PORT"
	smtpUser  = "SMTP_USER"
	smtpPass  = "SMTP_PASSWORD"
	smtpFrom  = "SMTP_FROM"
)

func (s *Service) gmail() *outbound.Mailer {
	addr, ok1 := s.V.Get(gmailAddr)
	pass, ok2 := s.V.Get(gmailPass)
	if !ok1 || !ok2 {
		return nil
	}
	host, port := "smtp.gmail.com", "587"
	if o := os.Getenv("AURORA_GMAIL_SMTP_ADDR"); o != "" { // test-only: local fake SMTP
		if h, p, err := net.SplitHostPort(o); err == nil {
			host, port = h, p
		}
	}
	return outbound.NewMailer("gmail", host, port, addr, strings.ReplaceAll(pass, " ", ""), addr)
}

func (s *Service) smtp() *outbound.Mailer {
	host, ok := s.V.Get(smtpHost)
	if !ok {
		return nil
	}
	port, _ := s.V.Get(smtpPort)
	user, _ := s.V.Get(smtpUser)
	pass, _ := s.V.Get(smtpPass)
	from, _ := s.V.Get(smtpFrom)
	return outbound.NewMailer("smtp", host, port, user, pass, from)
}

// mailer picks the active email channel: vault Gmail, vault SMTP, then env.
func (s *Service) mailer() *outbound.Mailer {
	if m := s.gmail(); m != nil {
		return m
	}
	if m := s.smtp(); m != nil && m.Configured() {
		return m
	}
	return outbound.MailerFromEnv()
}

func (s *Service) secretNames() []string {
	out := []string{}
	for _, m := range s.V.List() {
		out = append(out, m.Name)
	}
	return out
}

// outboundInfo lists the real delivery channels for the connectors sheet.
func (s *Service) outboundInfo() map[string]any {
	act := s.mailer()
	g, sm, env := s.gmail(), s.smtp(), outbound.MailerFromEnv()
	info := map[string]any{
		"email": map[string]any{"configured": act.Configured(), "from": act.Sender(), "source": act.Source},
		"gmail": map[string]any{"connected": g != nil, "from": g.Sender(), "accounts": b2i(g != nil)},
		"smtp":  map[string]any{"connected": sm != nil && sm.Configured() || env.Configured(), "from": first(sm.Sender(), env.Sender()), "env": env.Configured(), "accounts": b2i(sm != nil && sm.Configured()) + b2i(env.Configured())},
		"push":  map[string]any{"connected": s.Push != nil && s.Push.Pub != "", "accounts": len(s.Prefs.PushSubs())},
	}
	return info
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func first(a ...string) string {
	for _, x := range a {
		if x != "" {
			return x
		}
	}
	return ""
}

func decodeStrict(r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16<<10))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return ledger.ErrInvalid
	}
	return nil
}

func vaultErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vault.ErrMissing):
		fail(w, ledger.ErrNotFound)
	case errors.Is(err, vault.ErrName), errors.Is(err, vault.ErrValue):
		writeJSON(w, 400, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, 500, map[string]string{"error": "vault_write_failed"})
	}
}

func (s *Service) connectorRoutes() {
	m := s.mux
	m.HandleFunc("GET /app/api/secrets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		query, err := url.ParseQuery(r.URL.RawQuery)
		if values, present := query["workspaceId"]; err != nil || present && (len(values) != 1 || values[0] == "") {
			fail(w, ledger.ErrInvalid)
			return
		}
		if !s.vaultWorkspace(w, query.Get("workspaceId")) {
			return
		}
		writeJSON(w, 200, map[string]any{"workspaceId": s.workspaceID, "secrets": s.V.List()})
	})
	m.HandleFunc("POST /app/api/secrets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var in struct {
			WorkspaceID string `json:"workspaceId"`
			Name        string `json:"name"`
			Value       string `json:"value"`
			Label       string `json:"label"`
			Work        string `json:"work"`
		}
		if s.L.ReadOnly() {
			fail(w, ledger.ErrConflict)
			return
		}
		if err := decodeVaultRequest(w, r, &in); err != nil {
			fail(w, ledger.ErrInvalid)
			return
		}
		if !s.vaultWorkspace(w, in.WorkspaceID) {
			return
		}
		meta, err := s.V.Set(vault.Meta{Name: in.Name, Label: clean(in.Label, 80), Kind: "secret"}, in.Value)
		in.Value = ""
		if err != nil {
			vaultErr(w, err)
			return
		}
		bot := ""
		if in.Work != "" {
			if wk, err := s.L.Get(in.Work); err == nil {
				bot = wk.BotID
			} else {
				in.Work = ""
			}
		}
		s.record(bot, in.Work, "secret", "Đã lưu bí mật "+meta.Name+" vào kho")
		s.sync()
		writeJSON(w, 200, meta)
	})
	m.HandleFunc("POST /app/api/secrets/{name}/delete", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.L.ReadOnly() {
			fail(w, ledger.ErrConflict)
			return
		}
		if !s.vaultActionRequest(w, r) {
			return
		}
		n := r.PathValue("name")
		if err := s.V.Delete(n); err != nil {
			vaultErr(w, err)
			return
		}
		s.record("", "", "secret", "Đã xoá bí mật "+n)
		s.sync()
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	m.HandleFunc("POST /app/api/connectors/gmail", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Address  string `json:"address"`
			Password string `json:"password"`
		}
		if err := decodeStrict(r, &in); err != nil {
			fail(w, err)
			return
		}
		addr, ok := outbound.ValidEmail(in.Address)
		pw := strings.ReplaceAll(in.Password, " ", "")
		in.Password = ""
		if !ok {
			writeJSON(w, 400, map[string]string{"error": "invalid_address"})
			return
		}
		if pw == "" && !s.V.Has(gmailPass) {
			writeJSON(w, 400, map[string]string{"error": "password_required"})
			return
		}
		if pw != "" {
			if len(pw) != 16 {
				writeJSON(w, 400, map[string]string{"error": "app_password_16_chars"})
				return
			}
			if _, err := s.V.Set(vault.Meta{Name: gmailPass, Label: "Mật khẩu ứng dụng Gmail", Kind: "secret"}, pw); err != nil {
				vaultErr(w, err)
				return
			}
		}
		if _, err := s.V.Set(vault.Meta{Name: gmailAddr, Label: "Địa chỉ Gmail", Kind: "plain", Hint: addr}, addr); err != nil {
			vaultErr(w, err)
			return
		}
		s.record("", "", "connector", "Đã kết nối Gmail "+addr)
		s.sync()
		writeJSON(w, 200, s.outboundInfo())
	})
	m.HandleFunc("POST /app/api/connectors/{id}/disconnect", func(w http.ResponseWriter, r *http.Request) {
		var names []string
		switch r.PathValue("id") {
		case "gmail":
			names = []string{gmailAddr, gmailPass}
		case "smtp":
			names = []string{smtpHost, smtpPort, smtpUser, smtpPass, smtpFrom}
		default:
			fail(w, ledger.ErrNotFound)
			return
		}
		for _, n := range names {
			_ = s.V.Delete(n)
		}
		s.record("", "", "connector", "Đã ngắt kết nối "+r.PathValue("id"))
		s.sync()
		writeJSON(w, 200, s.outboundInfo())
	})
	m.HandleFunc("POST /app/api/connectors/smtp", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Host, Port, User, Password, From string
		}
		if err := decodeStrict(r, &in); err != nil {
			fail(w, err)
			return
		}
		in.Host = strings.TrimSpace(in.Host)
		from, ok := outbound.ValidEmail(first(in.From, in.User))
		if in.Port == "" {
			in.Port = "587"
		}
		if !ok || in.Host == "" || strings.ContainsAny(in.Host, " /:") || (in.Port != "587" && in.Port != "465" && in.Port != "2525") {
			in.Password = ""
			writeJSON(w, 400, map[string]string{"error": "invalid_smtp"})
			return
		}
		set := func(n, label, kind, v string) error {
			if v == "" {
				_ = s.V.Delete(n)
				return nil
			}
			_, err := s.V.Set(vault.Meta{Name: n, Label: label, Kind: kind, Hint: map[bool]string{true: v}[kind == "plain"]}, v)
			return err
		}
		for _, e := range []error{set(smtpHost, "Máy chủ SMTP", "plain", in.Host), set(smtpPort, "Cổng SMTP", "plain", in.Port), set(smtpUser, "Tài khoản SMTP", "plain", strings.TrimSpace(in.User)), set(smtpFrom, "Địa chỉ gửi", "plain", from)} {
			if e != nil {
				in.Password = ""
				vaultErr(w, e)
				return
			}
		}
		if in.Password != "" {
			if _, err := s.V.Set(vault.Meta{Name: smtpPass, Label: "Mật khẩu SMTP", Kind: "secret"}, in.Password); err != nil {
				in.Password = ""
				vaultErr(w, err)
				return
			}
		}
		in.Password = ""
		s.record("", "", "connector", "Đã kết nối SMTP "+in.Host)
		s.sync()
		writeJSON(w, 200, s.outboundInfo())
	})
}
