package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Web connections remember a user's login for a site so a bot can use it
// later. Secrets are sealed with Windows DPAPI (CurrentUser, per-connection
// entropy) and never leave this machine; the API only ever returns metadata.
type webConnection struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	SiteURL   string    `json:"siteUrl"`
	Username  string    `json:"username"`
	Kind      string    `json:"kind"`
	Sealed    string    `json:"sealed,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type webConnectionRegistry struct {
	mu    sync.Mutex
	path  string
	items []webConnection
}

var (
	errWebConnectionInvalid = errors.New("invalid")
	errWebConnectionSeal    = errors.New("unavailable")
	errWebConnectionMissing = errors.New("not_found")
)

func (r *webConnectionRegistry) load() error {
	if r.items != nil {
		return nil
	}
	raw, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		r.items = []webConnection{}
		return nil
	}
	if err != nil {
		return err
	}
	var parsed struct {
		Version     int             `json:"version"`
		Connections []webConnection `json:"connections"`
	}
	if json.Unmarshal(raw, &parsed) != nil || parsed.Version != 1 {
		return errWebConnectionInvalid
	}
	r.items = parsed.Connections
	if r.items == nil {
		r.items = []webConnection{}
	}
	return nil
}

func (r *webConnectionRegistry) save() error {
	payload, err := json.Marshal(map[string]any{"version": 1, "connections": r.items})
	if err != nil {
		return err
	}
	temporary := r.path + ".tmp"
	if os.WriteFile(temporary, payload, 0o600) != nil {
		return errWebConnectionInvalid
	}
	if os.Rename(temporary, r.path) != nil {
		os.Remove(temporary)
		return errWebConnectionInvalid
	}
	return nil
}

func (r *webConnectionRegistry) public() []map[string]any {
	view := make([]map[string]any, 0, len(r.items))
	for _, item := range r.items {
		view = append(view, map[string]any{
			"id": item.ID, "name": item.Name, "siteUrl": item.SiteURL,
			"username": item.Username, "kind": item.Kind,
			"createdAt": item.CreatedAt, "updatedAt": item.UpdatedAt,
		})
	}
	return view
}

func validWebConnectionText(value string, bound int) bool {
	return value != "" && len(value) <= bound && !strings.ContainsAny(value, "\x00\r\n\x1f")
}

func validWebConnectionURL(value string) bool {
	if len(value) > 512 || strings.ContainsAny(value, "\x00\r\n\t ") {
		return false
	}
	return strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://")
}

func (s *Server) webConnections(w http.ResponseWriter, r *http.Request) {
	if s.web == nil {
		http.NotFound(w, r)
		return
	}
	s.web.mu.Lock()
	defer s.web.mu.Unlock()
	if err := s.web.load(); err != nil {
		sendError(w, errWebConnectionInvalid)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/web-connections":
		send(w, http.StatusOK, map[string]any{"version": 1, "connections": s.web.public()})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/web-connections":
		var input struct {
			Name     string `json:"name"`
			SiteURL  string `json:"siteUrl"`
			Username string `json:"username"`
			Kind     string `json:"kind"`
			Secret   string `json:"secret"`
		}
		if decode(w, r, &input) != nil {
			sendError(w, errWebConnectionInvalid)
			return
		}
		if !validWebConnectionText(input.Name, 80) || !validWebConnectionURL(input.SiteURL) ||
			len(input.Username) > 128 || len(input.Secret) == 0 || len(input.Secret) > 65536 {
			sendError(w, errWebConnectionInvalid)
			return
		}
		kind := input.Kind
		switch kind {
		case "login", "secret", "ssh_key", "json", "token", "text":
		default:
			kind = "text"
		}
		identity := make([]byte, 16)
		if _, err := rand.Read(identity); err != nil {
			sendError(w, errWebConnectionInvalid)
			return
		}
		id := "web-" + hex.EncodeToString(identity)
		sealed, err := sealWebSecret([]byte(input.Secret), "nekobot.web-connection.v1\x00"+id)
		clearSecret(&input.Secret)
		if err != nil {
			sendError(w, errWebConnectionSeal)
			return
		}
		now := time.Now().UTC()
		record := webConnection{ID: id, Name: strings.TrimSpace(input.Name), SiteURL: input.SiteURL,
			Username: input.Username, Kind: kind, Sealed: hex.EncodeToString(sealed), CreatedAt: now, UpdatedAt: now}
		s.web.items = append(s.web.items, record)
		if s.web.save() != nil {
			s.web.items = s.web.items[:len(s.web.items)-1]
			sendError(w, errWebConnectionInvalid)
			return
		}
		send(w, http.StatusOK, map[string]any{"version": 1, "connection": map[string]any{
			"id": record.ID, "name": record.Name, "siteUrl": record.SiteURL,
			"username": record.Username, "kind": record.Kind,
			"createdAt": record.CreatedAt, "updatedAt": record.UpdatedAt,
		}})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/web-connections/remove":
		var input struct {
			ID string `json:"id"`
		}
		if decode(w, r, &input) != nil || !strings.HasPrefix(input.ID, "web-") || len(input.ID) != 4+32 {
			sendError(w, errWebConnectionInvalid)
			return
		}
		for index, item := range s.web.items {
			if item.ID == input.ID {
				s.web.items = append(s.web.items[:index], s.web.items[index+1:]...)
				if s.web.save() != nil {
					sendError(w, errWebConnectionInvalid)
					return
				}
				send(w, http.StatusOK, map[string]any{"version": 1, "removed": input.ID})
				return
			}
		}
		sendError(w, errWebConnectionMissing)
	default:
		http.NotFound(w, r)
	}
}

// EnableWebConnections stores site logins under the local state directory.
func (s *Server) EnableWebConnections(stateDir string) error {
	absolute, err := filepath.Abs(stateDir)
	if err != nil || absolute == "" || filepath.VolumeName(absolute) == "" {
		return errWebConnectionInvalid
	}
	directory := filepath.Join(absolute, "web-connections")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	s.web = &webConnectionRegistry{path: filepath.Join(directory, "web-connections.json")}
	return nil
}

func clearSecret(value *string) {
	*value = strings.Repeat("\x00", len(*value))
	*value = ""
}
