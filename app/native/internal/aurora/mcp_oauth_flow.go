package aurora

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
)

type mcpOAuthIdentity struct {
	WorkspaceID string `json:"workspaceId"`
	Name        string `json:"name"`
	MCPRevision uint64 `json:"mcpRevision"`
	Generation  string `json:"authorizationGeneration"`
	Revision    uint64 `json:"authorizationRevision"`
}

type mcpOAuthStart struct {
	mcpOAuthIdentity
	Issuer   string   `json:"issuer"`
	ClientID string   `json:"clientId"`
	Scopes   []string `json:"scopes"`
}

type mcpOAuthStatus struct {
	BotID            string   `json:"botId"`
	Generation       string   `json:"generation"`
	Revision         uint64   `json:"revision"`
	Phase            string   `json:"phase"`
	Issuer           string   `json:"issuer,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
	ExpiresAt        int64    `json:"expiresAt,omitempty"`
	CanRefresh       bool     `json:"canRefresh"`
	RedirectURI      string   `json:"redirectUri,omitempty"`
	AuthorizationURL string   `json:"authorizationUrl,omitempty"`
}

type mcpOAuthLocked struct {
	ctx            context.Context
	s              *Service
	bot            string
	srv            mcpServer
	configRevision uint64
	id, key        string
	release        func()
}

func (s *Service) lockMCPOAuth(ctx context.Context, bot string, in mcpOAuthIdentity) (*mcpOAuthLocked, error) {
	if s.oauth == nil || s.ext == nil {
		return nil, errMCPOAuthDisabled
	}
	if in.WorkspaceID != s.workspaceID {
		return nil, errMCPOAuthChanged
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := func() bool { return true }
	if s.ctx != nil {
		if s.ctx.Err() != nil {
			cancel()
			return nil, s.ctx.Err()
		}
		stop = context.AfterFunc(s.ctx, cancel)
	}
	cleanup := func() { stop(); cancel() }
	cache := s.mcpState()
	unlock, err := cache.lock(ctx, "configuration\x00"+bot)
	if err != nil {
		cleanup()
		return nil, err
	}
	if _, ok := s.view().bot(bot); !ok {
		unlock()
		cleanup()
		return nil, errMCPOAuthChanged
	}
	be := s.ext.bot(bot)
	srv, ok := s.mcpServerOf(bot, in.Name)
	if !ok || be.MCPRevision != in.MCPRevision {
		unlock()
		cleanup()
		return nil, errMCPOAuthChanged
	}
	key := mcpKey(bot, srv)
	connectionUnlock, err := cache.lock(ctx, key)
	if err != nil {
		unlock()
		cleanup()
		return nil, err
	}
	return &mcpOAuthLocked{ctx: ctx, s: s, bot: bot, srv: srv, configRevision: be.MCPRevision, id: mcpOAuthRecordID(s.workspaceID, bot, srv.Name), key: key, release: func() { connectionUnlock(); unlock(); cleanup() }}, nil
}

func (l *mcpOAuthLocked) drop() {
	cache := l.s.mcpState()
	cache.mu.Lock()
	cache.dropLocked(l.key)
	cache.mu.Unlock()
}

func (l *mcpOAuthLocked) matches(record *mcpOAuthRecord) bool {
	b := record.Binding
	return b.Workspace == l.s.workspaceID && b.Bot == l.bot && b.Server == l.srv && b.ConfigRevision == l.configRevision
}

func (l *mcpOAuthLocked) status(record *mcpOAuthRecord, origin string) mcpOAuthStatus {
	result := mcpOAuthStatus{Phase: "disconnected", BotID: l.bot}
	result.RedirectURI, _ = l.s.oauth.redirect(origin)
	if record == nil {
		return result
	}
	result.Generation, result.Revision = record.Generation, record.Revision
	if record.Phase == "disconnected" {
		return result
	}
	if !l.matches(record) {
		result.Phase = "configuration_changed"
		return result
	}
	result.Phase, result.Issuer = record.Phase, record.Binding.Issuer
	result.Scopes = append([]string(nil), record.GrantedScopes...)
	result.ExpiresAt = record.ExpiresAt
	if record.Phase == "pending" {
		result.Scopes = append([]string(nil), record.Binding.Scopes...)
		if record.PendingUntil <= l.s.oauth.now().Unix() {
			result.Phase = "expired"
		} else if record.Binding.RedirectURI == result.RedirectURI {
			result.AuthorizationURL, _ = mcpOAuthAuthorizationURL(record)
		}
	}
	if record.Phase == "exchanging" || record.Phase == "refreshing" {
		result.Phase = "outcome_unknown"
	}
	if record.Phase == "connected" {
		result.CanRefresh = record.secrets["refresh"] != ""
		if record.ExpiresAt != 0 && record.ExpiresAt <= l.s.oauth.now().Unix() {
			result.Phase = "expired"
		}
	}
	return result
}

func (s *Service) mcpOAuthStatus(ctx context.Context, bot, origin string, in mcpOAuthIdentity) (*mcpOAuthStatus, error) {
	l, err := s.lockMCPOAuth(ctx, bot, in)
	if err != nil {
		return nil, err
	}
	defer l.release()
	record, err := s.oauth.read(l.id)
	if err != nil {
		return nil, err
	}
	status := l.status(record, origin)
	return &status, nil
}

func mcpOAuthSameScopes(a, b []string) bool {
	if !mcpScopes(a) || len(a) != len(b) {
		return false
	}
	for i, scope := range a {
		if !slices.Contains(b, scope) || slices.Contains(a[:i], scope) {
			return false
		}
	}
	return true
}

func (s *Service) mcpOAuthStart(ctx context.Context, bot, origin string, in mcpOAuthStart) (*mcpOAuthStatus, error) {
	l, err := s.lockMCPOAuth(ctx, bot, in.mcpOAuthIdentity)
	if err != nil {
		return nil, err
	}
	defer l.release()
	redirect, err := s.oauth.redirect(origin)
	if err != nil {
		return nil, err
	}
	if l.srv.Header != "" || l.srv.Secret != "" || !mcpOAuthOpaque(in.ClientID, 2048) {
		return nil, errMCPOAuthInvalid
	}
	previous, err := s.oauth.read(l.id)
	if err != nil {
		return nil, err
	}
	if !mcpOAuthExpected(previous, in.Generation, in.Revision) {
		return nil, errMCPOAuthChanged
	}
	cache := s.mcpState()
	cache.mu.Lock()
	failure := cache.failures[l.key]
	cache.mu.Unlock()
	var challenge *mcpAuthChallenge
	if failure != nil {
		if failure.authInvalid {
			return nil, errMCPOAuthInvalid
		}
		challenge = failure.challenge
	}
	discovered, err := discoverMCPAuthorization(l.ctx, s.oauth.http(), l.srv.URL, challenge, in.Issuer)
	if err != nil {
		return nil, errMCPOAuthInvalid
	}
	metadata := discovered.Authorization
	if metadata == nil || !metadata.PKCES256 || !mcpOAuthSameScopes(in.Scopes, discovered.Scopes) {
		return nil, errMCPOAuthChanged
	}
	generation, err := mcpOAuthRandom(16)
	if err != nil {
		return nil, err
	}
	random, err := mcpOAuthRandom(32)
	if err != nil {
		return nil, err
	}
	verifier, err := mcpOAuthRandom(32)
	if err != nil {
		return nil, err
	}
	state := l.id + "." + random
	record := &mcpOAuthRecord{mcpOAuthState: mcpOAuthState{Version: 1, Generation: generation, Revision: 1, Phase: "pending", PendingUntil: s.oauth.now().Unix() + 600, StateHash: mcpOAuthHash(state), Binding: mcpOAuthBinding{
		Workspace: s.workspaceID, Bot: bot, Server: l.srv, ConfigRevision: l.configRevision, Issuer: metadata.Issuer, ClientID: in.ClientID, RedirectURI: redirect,
		AuthorizationEndpoint: metadata.Authorization, TokenEndpoint: metadata.Token, RequireIssuer: metadata.RequireIssuer, Scopes: append([]string(nil), discovered.Scopes...),
	}}, secrets: map[string]string{"state": state, "verifier": verifier}}
	if _, err := mcpOAuthAuthorizationURL(record); err != nil {
		return nil, err
	}
	if err := l.ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.oauth.save(l.id, record); err != nil {
		return nil, err
	}
	l.drop()
	status := l.status(record, origin)
	return &status, nil
}

type mcpOAuthCallback struct {
	State  string  `json:"state"`
	Code   string  `json:"code"`
	Issuer *string `json:"iss"`
	Error  string  `json:"error"`
}

func (s *Service) mcpOAuthFinish(ctx context.Context, origin string, in mcpOAuthCallback) (*mcpOAuthStatus, error) {
	if s.oauth == nil {
		return nil, errMCPOAuthDisabled
	}
	id, random, ok := strings.Cut(in.State, ".")
	decodedID, idError := hex.DecodeString(id)
	decodedRandom, randomError := base64.RawURLEncoding.DecodeString(random)
	if idError != nil || randomError != nil || len(decodedID) != 32 || len(decodedRandom) != 32 || hex.EncodeToString(decodedID) != id || base64.RawURLEncoding.EncodeToString(decodedRandom) != random {
		return nil, errMCPOAuthInvalid
	}
	if !ok || len(id) != 64 || len(random) != 43 || !mcpOAuthOpaque(in.State, 108) || (in.Code == "") == (in.Error == "") || in.Code != "" && !mcpOAuthOpaque(in.Code, 4096) || in.Error != "" && !mcpOAuthOpaque(in.Error, 128) {
		return nil, errMCPOAuthInvalid
	}
	previous, err := s.oauth.read(id)
	if err != nil {
		return nil, err
	}
	if previous == nil || previous.Binding.Workspace != s.workspaceID {
		return nil, errMCPOAuthChanged
	}
	b := previous.Binding
	l, err := s.lockMCPOAuth(ctx, b.Bot, mcpOAuthIdentity{WorkspaceID: s.workspaceID, Name: b.Server.Name, MCPRevision: b.ConfigRevision})
	if err != nil {
		return nil, err
	}
	defer l.release()
	record, err := s.oauth.read(id)
	if err != nil {
		return nil, err
	}
	if record == nil || !l.matches(record) || record.Binding.RedirectURI != origin+mcpOAuthCallbackPath || !mcpOAuthEqual(record.StateHash, mcpOAuthHash(in.State)) {
		return nil, errMCPOAuthChanged
	}
	if in.Issuer != nil && *in.Issuer != record.Binding.Issuer || in.Issuer == nil && record.Binding.RequireIssuer {
		return nil, errMCPOAuthInvalid
	}
	receiptBytes, _ := json.Marshal([]any{in.Code, in.Issuer, in.Error})
	receipt := mcpOAuthHash(string(receiptBytes))
	if (record.Phase == "connected" || record.Phase == "denied") && record.CallbackHash == receipt {
		status := l.status(record, origin)
		return &status, nil
	}
	if record.Phase == "exchanging" || record.Phase == "refreshing" {
		return nil, errMCPOAuthUnknown
	}
	if record.Phase != "pending" || record.PendingUntil <= s.oauth.now().Unix() {
		return nil, errMCPOAuthChanged
	}
	if err := l.ctx.Err(); err != nil {
		return nil, err
	}
	record.CallbackHash = receipt
	record.Revision++
	if in.Error != "" {
		record.Phase = "denied"
		record.secrets = map[string]string{}
		if err := s.oauth.save(id, record); err != nil {
			return nil, err
		}
		l.drop()
		status := l.status(record, origin)
		return &status, nil
	}
	record.Phase = "exchanging"
	if err := s.oauth.save(id, record); err != nil {
		return nil, err
	}
	l.drop()
	tokens, err := s.oauth.token(l.ctx, record.Binding, url.Values{"grant_type": {"authorization_code"}, "code": {in.Code}, "code_verifier": {record.secrets["verifier"]}, "redirect_uri": {record.Binding.RedirectURI}}, record.Binding.Scopes)
	if err != nil {
		return nil, errMCPOAuthUnknown
	}
	record.Phase = "connected"
	record.Revision++
	record.PendingUntil = 0
	record.ExpiresAt = tokens.expiresAt
	record.GrantedScopes = tokens.scopes
	record.secrets = map[string]string{"access": tokens.access}
	if tokens.refresh != "" {
		record.secrets["refresh"] = tokens.refresh
	}
	if err := s.oauth.save(id, record); err != nil {
		return nil, errMCPOAuthUnknown
	}
	status := l.status(record, origin)
	return &status, nil
}

type mcpOAuthRefresh struct {
	mcpOAuthIdentity
	Nonce string `json:"nonce"`
}

func (s *Service) mcpOAuthRefresh(ctx context.Context, bot, origin string, in mcpOAuthRefresh) (*mcpOAuthStatus, error) {
	l, err := s.lockMCPOAuth(ctx, bot, in.mcpOAuthIdentity)
	if err != nil {
		return nil, err
	}
	defer l.release()
	record, err := s.oauth.read(l.id)
	if err != nil {
		return nil, err
	}
	if record == nil || !l.matches(record) || record.Generation != in.Generation {
		return nil, errMCPOAuthChanged
	}
	if !mcpOAuthOpaque(in.Nonce, 128) || len(in.Nonce) < 8 {
		return nil, errMCPOAuthInvalid
	}
	if record.RefreshNonce == in.Nonce && record.RefreshBase == in.Revision {
		if record.Phase == "connected" {
			status := l.status(record, origin)
			return &status, nil
		}
		return nil, errMCPOAuthUnknown
	}
	if !mcpOAuthExpected(record, in.Generation, in.Revision) || record.Phase != "connected" {
		return nil, errMCPOAuthChanged
	}
	refresh := record.secrets["refresh"]
	if refresh == "" {
		return nil, errMCPOAuthInvalid
	}
	if err := l.ctx.Err(); err != nil {
		return nil, err
	}
	record.RefreshNonce, record.RefreshBase = in.Nonce, in.Revision
	record.Phase = "refreshing"
	record.Revision++
	if err := s.oauth.save(l.id, record); err != nil {
		return nil, err
	}
	l.drop()
	tokens, err := s.oauth.token(l.ctx, record.Binding, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}}, record.GrantedScopes)
	if err != nil {
		return nil, errMCPOAuthUnknown
	}
	record.Phase = "connected"
	record.Revision++
	record.ExpiresAt = tokens.expiresAt
	record.GrantedScopes = tokens.scopes
	record.secrets = map[string]string{"access": tokens.access}
	if tokens.refresh != "" {
		record.secrets["refresh"] = tokens.refresh
	}
	if err := s.oauth.save(l.id, record); err != nil {
		return nil, errMCPOAuthUnknown
	}
	status := l.status(record, origin)
	return &status, nil
}

func (s *Service) mcpOAuthDisconnect(ctx context.Context, bot, origin string, in mcpOAuthIdentity) (*mcpOAuthStatus, error) {
	l, err := s.lockMCPOAuth(ctx, bot, in)
	if err != nil {
		return nil, err
	}
	defer l.release()
	record, err := s.oauth.read(l.id)
	if err != nil {
		return nil, err
	}
	if !mcpOAuthExpected(record, in.Generation, in.Revision) {
		return nil, errMCPOAuthChanged
	}
	if err := l.ctx.Err(); err != nil {
		return nil, err
	}
	if record != nil && record.Phase != "disconnected" {
		record.Phase = "disconnected"
		record.Revision++
		record.secrets = map[string]string{}
		record.PendingUntil, record.ExpiresAt = 0, 0
		record.GrantedScopes = nil
		record.StateHash, record.CallbackHash = "", ""
		record.RefreshNonce = ""
		record.RefreshBase = 0
		if err := s.oauth.save(l.id, record); err != nil {
			return nil, err
		}
	}
	l.drop()
	status := l.status(record, origin)
	return &status, nil
}
