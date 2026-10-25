package aurora

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nekoaurora.local/native/internal/cloudbrowser"
	"nekoaurora.local/native/internal/identity"
	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/vault"
)

const parkedCloud = "\x00cloud:"

func openCloudBrowser(stateDir string) *cloudbrowser.Client {
	c, err := cloudbrowser.New(cloudbrowser.Config{LocalDocker: Env("CLOUD_BROWSER_DOCKER") == "1", URL: Env("CLOUD_BROWSER_URL"), Key: []byte(Env("CLOUD_BROWSER_KEY")), StateDir: stateDir})
	if err != nil && !errors.Is(err, cloudbrowser.ErrNotConfigured) {
		log.Print("aurora cloud browser disabled: invalid configuration")
	}
	return c
}

// PurgeCloudForDeletion is shared by standalone and hosted confirmed deletion.
// A removed pilot entitlement must not erase the obligation to purge prior use.
func (s *Service) PurgeCloudForDeletion(ctx context.Context) error {
	if s.L.ReadOnly() {
		return cloudbrowser.ErrCleanupPending
	}
	if s.Cloud != nil {
		return s.Cloud.PurgeNamespace(ctx)
	}
	used, err := cloudbrowser.NamespaceUsed(s.cloudStateDir)
	if err != nil {
		return cloudbrowser.ErrCleanupPending
	}
	if !used {
		return nil
	}
	if err = cloudbrowser.MarkPurgePending(s.cloudStateDir); err != nil {
		return cloudbrowser.ErrCleanupPending
	}
	cleanup := openCloudBrowser(s.cloudStateDir)
	if cleanup == nil {
		return cloudbrowser.ErrCleanupPending
	}
	return cleanup.PurgeNamespace(ctx)
}

func (s *Service) cloudOwner(r *http.Request) (identity.RequestIdentity, error) {
	if s.L.ReadOnly() {
		return identity.RequestIdentity{}, ledger.ErrReadOnly
	}
	a, ok := identity.RequestIdentityFrom(r.Context())
	if !ok {
		return a, errCloudIdentity
	}
	if a.Principal != nil {
		return a, ledger.ErrForbidden
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return a, ledger.ErrForbidden
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, e := url.Parse(origin)
		if e != nil || u.User != nil || u.Host != r.Host || (u.Scheme != "https" && u.Scheme != "http") {
			return a, ledger.ErrForbidden
		}
	}
	return a, nil
}

var errCloudIdentity = errors.New("cloud_browser_owner_identity_required")

func (s *Service) cloudWork(w ledger.Work, completed bool) (ledger.Work, error) {
	if s.L.ReadOnly() {
		return ledger.Work{}, ledger.ErrReadOnly
	}
	cur, err := s.L.Get(w.RequestID)
	if err != nil || cur.BotID != w.BotID || cur.Epoch != w.Epoch || ledger.NetworkAttempt(cur) == "" || ledger.NetworkAttempt(cur) != ledger.NetworkAttempt(w) || cur.RequesterPin != nil || cur.SourceBotID != "" || s.networkPeerWork(cur) {
		return ledger.Work{}, ledger.ErrForbidden
	}
	want := "running"
	if completed {
		want = "completed"
	}
	if cur.Status != want || cur.Direct == nil || (!completed && cur.DesiredState != "running") {
		return ledger.Work{}, ledger.ErrForbidden
	}
	if _, ok := s.view().bot(cur.BotID); !ok || s.view().archived[cur.BotID] {
		return ledger.Work{}, ledger.ErrForbidden
	}
	if s.Cloud == nil {
		return ledger.Work{}, cloudbrowser.ErrNotConfigured
	}
	return cur, nil
}

func cloudAgentCommand(w ledger.Work, kind string) cloudbrowser.Command {
	return cloudbrowser.Command{Profile: w.BotID, Kind: kind, Actor: "agent", ActorID: w.BotID, WorkID: w.RequestID, WorkEpoch: w.Epoch, Attempt: ledger.NetworkAttempt(w)}
}

func cloudCanonicalOrigin(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	h := strings.ToLower(u.Hostname())
	port := u.Port()
	if port != "" && port != "443" {
		return "", false
	}
	return "https://" + h, true
}

type cloudProposal struct {
	Command     cloudbrowser.Command `json:"command"`
	BootID      string               `json:"bootId"`
	Item        string               `json:"item,omitempty"`
	ItemVersion string               `json:"itemVersion,omitempty"`
	Title       string               `json:"title"`
	Body        string               `json:"body"`
	Signature   string               `json:"signature,omitempty"`
}

func (s *Service) cloudSign(p cloudProposal) string {
	p.Signature = ""
	b, _ := json.Marshal(p)
	return s.Cloud.SignProposal(b)
}

func (s *Service) cloudProposal(raw string) (cloudProposal, error) {
	var p cloudProposal
	if s.Cloud == nil {
		return p, cloudbrowser.ErrNotConfigured
	}
	if len(raw) > 4000 || json.Unmarshal([]byte(raw), &p) != nil {
		return p, ledger.ErrForbidden
	}
	sig := p.Signature
	p.Signature = ""
	b, _ := json.Marshal(p)
	if !s.Cloud.VerifyProposal(b, sig) {
		return p, ledger.ErrForbidden
	}
	p.Signature = sig
	return p, nil
}

func (s *Service) cloudApproval(raw string) *Approval {
	p, err := s.cloudProposal(raw)
	if err != nil {
		return nil
	}
	return &Approval{Kind: "cloud", Title: p.Title, Body: p.Body, Use: raw, State: "pending"}
}

func (s *Service) cloudApprovedProposal(w ledger.Work, ap *Approval) (cloudProposal, error) {
	p, err := s.cloudProposal(ap.Use)
	if err != nil {
		return p, err
	}
	if _, err = s.cloudWork(w, true); err != nil {
		return p, err
	}
	c := p.Command
	if ap.Kind != "cloud" || ap.Title != p.Title || ap.Body != p.Body || c.Profile != w.BotID || c.WorkID != w.RequestID || c.WorkEpoch != w.Epoch || c.Attempt != ledger.NetworkAttempt(w) || c.Actor != "agent" || c.ActorID != w.BotID {
		return p, ledger.ErrForbidden
	}
	return p, nil
}

func (s *Service) cloudItem(item, expected string) (vault.Item, map[string]string, string, error) {
	it, ok := s.V.Item(item)
	if !ok || it.Type != "login" {
		return vault.Item{}, nil, "", ErrInvalid
	}
	fields, ok := s.V.Fields(item)
	if !ok {
		return vault.Item{}, nil, "", ErrInvalid
	}
	origin, ok := cloudCanonicalOrigin(fields["site"])
	if !ok || origin != expected {
		return vault.Item{}, nil, "", ErrInvalid
	}
	// Fingerprint changes even when a rapid item edit keeps the timestamp.
	b, _ := json.Marshal(fields)
	fingerprint := s.Cloud.SignProposal(b)
	return it, fields, it.Updated + ":" + fingerprint, nil
}

func (s *Service) cloudVaultCommand(cmd cloudbrowser.Command, item, version string) (cloudbrowser.Command, error) {
	_, fields, current, err := s.cloudItem(item, cmd.ExpectedOrigin)
	if err != nil {
		return cloudbrowser.Command{}, err
	}
	if version != "" && version != current {
		return cloudbrowser.Command{}, cloudbrowser.ErrConflict
	}
	creds := cloudbrowser.Credentials{Username: fields["username"], Password: fields["password"]}
	if secret := fields["totp"]; secret != "" {
		creds.TOTP, err = vault.TOTP(secret, s.Now())
		if err != nil {
			return cloudbrowser.Command{}, ErrInvalid
		}
	}
	return s.Cloud.PrepareVault(cmd, creds, item+":"+current)
}

func (s *Service) recoverCloudApprovals() {
	for _, w := range s.L.Snapshot().Work {
		if w.Direct == nil || w.Direct.Approval == nil {
			continue
		}
		ap := w.Direct.Approval
		if ap.Kind == "cloud" && ap.State == "approved" && ap.Delivery == nil {
			s.setDelivery(w.RequestID, ledger.DirectDelivery{Status: "failed", Detail: "unknown"}, "Aurora đã khởi động lại trước khi lưu biên nhận. Kết quả thao tác chưa xác định; không tự gửi lại. Hãy quan sát mới trước khi tạo thao tác khác.")
		}
	}
}

func (s *Service) runApprovedCloud(ctx context.Context, w ledger.Work) {
	ap := w.Direct.Approval
	p, err := s.cloudApprovedProposal(w, ap)
	var out cloudbrowser.Response
	if err == nil {
		var current cloudbrowser.Response
		current, err = s.Cloud.Do(ctx, cloudAgentCommand(w, "status"))
		if err == nil && (current.Epoch != p.Command.Epoch || current.BootID != p.BootID || current.Origin != p.Command.ExpectedOrigin) {
			err = cloudbrowser.ErrConflict
		}
	}
	cmd := p.Command
	if err == nil && cmd.Kind == "vault_fill" {
		cmd, err = s.cloudVaultCommand(cmd, p.Item, p.ItemVersion)
	}
	if err == nil {
		// Recheck the source immediately before dispatch, after network/Vault work.
		_, err = s.cloudApprovedProposal(w, ap)
		if err == nil {
			out, err = s.Cloud.Do(ctx, cmd)
		}
	}
	state, detail, text := "failed", "not_dispatched", "Thao tác chưa được xác nhận thực hiện."
	if err == nil && out.Receipt != nil {
		detail = out.Receipt.Status
		if out.Receipt.Status == "dispatched" {
			state = "sent"
			text = "Đã chuyển thao tác tới trình duyệt. Đây chưa phải xác nhận đăng nhập hay gửi tin thành công."
		}
		if out.Receipt.Status == "unknown" {
			text = "Chưa xác định được kết quả thao tác; không tự gửi lại."
		}
	} else if errors.Is(err, cloudbrowser.ErrUnknownOutcome) {
		detail = "unknown"
		text = "Mất kết nối sau khi chuyển thao tác; kết quả chưa xác định, không tự gửi lại."
	}
	s.setDelivery(w.RequestID, ledger.DirectDelivery{Status: state, Detail: detail}, text)
	if cmd.Kind == "vault_fill" {
		s.V.LogUse(vault.Use{Item: p.Item, Bot: w.BotID, Action: "cloud_fill", Target: cmd.ExpectedOrigin, Result: detail})
	}
	s.sync()
}

func cloudNonce(w ledger.Work, t toolReq) string {
	b, _ := json.Marshal(struct {
		Work    string
		Epoch   uint64
		Attempt string
		Tool    toolReq
	}{w.RequestID, w.Epoch, ledger.NetworkAttempt(w), t})
	sum := sha256.Sum256(b)
	return "cloud-" + hex.EncodeToString(sum[:24])
}

func cloudApprovalBody(cmd cloudbrowser.Command, item string) string {
	text := "Thao tác " + cmd.Kind + " tại " + cmd.ExpectedOrigin + "."
	if cmd.TargetID != "" {
		text += "\nĐối tượng: " + cmd.TargetID
	}
	if cmd.Text != "" {
		text += "\nNội dung: " + cmd.Text
	}
	if cmd.URL != "" {
		text += "\nĐịa chỉ: " + cmd.URL
	}
	if cmd.Key != "" {
		text += "\nPhím: " + cmd.Key
	}
	if cmd.Kind == "scroll" {
		text += fmt.Sprintf("\nCuộn: %.0f", cmd.DeltaY)
	}
	if item != "" {
		text += "\nMục Vault: " + item + ". Chỉ điền thông tin đăng nhập; không gửi form."
	}
	return text + "\nDuyệt đúng thao tác này một lần; kết quả nhập không chứng minh tác vụ ngoài đã hoàn tất."
}

func cloudActionKind(kind string) bool {
	switch kind {
	case "navigate", "click", "fill", "press", "scroll":
		return true
	}
	return false
}

func cloudTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 40*time.Second)
}
