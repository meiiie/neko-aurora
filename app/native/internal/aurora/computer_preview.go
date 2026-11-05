package aurora

import (
	"context"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

// A preview is an untrusted document, never an application page. Its opaque
// origin cannot access Aurora cookies, storage, APIs or the containing window.
func (s *Service) computerPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if scope := r.URL.Query()["workspaceId"]; len(scope) != 1 || scope[0] != s.workspaceID {
		fail(w, ledger.ErrConflict)
		return
	}
	id, p := r.PathValue("id"), r.URL.Query().Get("path")
	if _, ok := newView(s.L.Snapshot()).bot(id); !ok || s.PC == nil {
		fail(w, ErrNotFound)
		return
	}
	ext := strings.ToLower(path.Ext(p))
	if ext != ".html" && ext != ".htm" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "preview_html_only"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	f, err := s.PC.OpenDownload(ctx, id, p)
	if err != nil {
		fail(w, ErrNotFound)
		return
	}
	defer f.Close()
	if f.Size > 4<<20 {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "preview_too_large"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), clipboard-read=(), clipboard-write=()")
	// Allow submit events for local JS forms; CSP still forbids every actual
	// form destination. Without allow-forms, Chromium skips the handler too.
	w.Header().Set("Content-Security-Policy", "sandbox allow-scripts allow-forms; default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; connect-src 'none'; frame-src 'none'; worker-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'")
	w.Header().Set("X-Aurora-Workspace", s.workspaceID)
	w.Header().Set("X-Aurora-Bot", id)
	if r.Method != "HEAD" {
		_, _ = io.Copy(w, f)
	}
}
