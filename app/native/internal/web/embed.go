package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
)

//go:embed assets/*
var assets embed.FS

var contentTypes = map[string]string{
	"/":                          "text/html; charset=utf-8",
	"/app.js":                    "text/javascript; charset=utf-8",
	"/connections.js":            "text/javascript; charset=utf-8",
	"/style.css":                 "text/css; charset=utf-8",
	"/app/":                      "text/html; charset=utf-8",
	"/app/theme.js":              "text/javascript; charset=utf-8",
	"/app/app.js":                "text/javascript; charset=utf-8",
	"/app/memory-import.js":      "text/javascript; charset=utf-8",
	"/app/outbox.js":             "text/javascript; charset=utf-8",
	"/app/composer-drafts.js":    "text/javascript; charset=utf-8",
	"/app/question-card.js":      "text/javascript; charset=utf-8",
	"/app/network.js":            "text/javascript; charset=utf-8",
	"/app/network.css":           "text/css; charset=utf-8",
	"/app/cloud.js":              "text/javascript; charset=utf-8",
	"/app/cloud.css":             "text/css; charset=utf-8",
	"/app/i18n.js":               "text/javascript; charset=utf-8",
	"/app/md.js":                 "text/javascript; charset=utf-8",
	"/app/ux.js":                 "text/javascript; charset=utf-8",
	"/app/style.css":             "text/css; charset=utf-8",
	"/app/sw.js":                 "text/javascript; charset=utf-8",
	"/app/icon.svg":              "image/svg+xml",
	"/app/icon-180.png":          "image/png",
	"/app/icon-512.png":          "image/png",
	"/app/icon-192.png":          "image/png",
	"/app/icon-maskable-512.png": "image/png",
	"/app/favicon.ico":           "image/x-icon",
	"/app/manifest.webmanifest":  "application/manifest+json",
	"/app/privacy.html":          "text/html; charset=utf-8",
	"/app/terms.html":            "text/html; charset=utf-8",
	"/app/legal.css":             "text/css; charset=utf-8",
	"/app/signin.html":           "text/html; charset=utf-8",
	"/app/welcome.html":          "text/html; charset=utf-8",
	"/app/hosted.css":            "text/css; charset=utf-8",
	"/app/hosted.js":             "text/javascript; charset=utf-8",
	"/app/vault-demo.html":       "text/html; charset=utf-8",
	"/app/vault-demo.js":         "text/javascript; charset=utf-8",
}

// The Neko Aurora design system (tokens, fonts, icons) is shared by the
// desktop and mobile UIs and lives under /app/aurora/ so the public mobile
// host can serve it too.
func init() {
	types := map[string]string{".css": "text/css; charset=utf-8", ".woff2": "font/woff2", ".txt": "text/plain; charset=utf-8", ".svg": "image/svg+xml", ".js": "text/javascript; charset=utf-8"}
	entries, _ := assets.ReadDir("assets/app/aurora")
	var walk func(dir string)
	walk = func(dir string) {
		list, _ := assets.ReadDir("assets" + dir)
		for _, e := range list {
			p := dir + "/" + e.Name()
			if e.IsDir() {
				walk(p)
				continue
			}
			for ext, t := range types {
				if len(p) > len(ext) && p[len(p)-len(ext):] == ext {
					contentTypes[p] = t
				}
			}
		}
	}
	if len(entries) > 0 {
		walk("/app/aurora")
	}
}

func IsPublicAsset(path string) bool {
	_, ok := contentTypes[path]
	return ok
}

func Serve(w http.ResponseWriter, r *http.Request) {
	contentType, ok := contentTypes[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	name := r.URL.Path
	if name == "/" || name == "/app/" {
		name += "index.html"
	}
	body, err := assets.ReadFile("assets" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	if name == "/app/sw.js" {
		w.Header().Set("Service-Worker-Allowed", "/app/")
	}
	// revalidate every load (deploys change files in place), but answer an
	// unchanged file with 304 and send text gzipped (app.js 113 KB -> 39 KB)
	z := packed(name, body, strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "json") || strings.Contains(contentType, "svg"))
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", z.etag)
	w.Header().Add("Vary", "Accept-Encoding")
	if r.Header.Get("If-None-Match") == z.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if z.gz != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(z.gz)
		return
	}
	_, _ = w.Write(body)
}

type packedAsset struct {
	etag string
	gz   []byte
}

var packedCache sync.Map

func packed(name string, body []byte, text bool) packedAsset {
	if v, ok := packedCache.Load(name); ok {
		return v.(packedAsset)
	}
	h := sha256.Sum256(body)
	z := packedAsset{etag: `"` + hex.EncodeToString(h[:8]) + `"`}
	if text && len(body) > 1024 {
		var b bytes.Buffer
		g, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
		_, _ = g.Write(body)
		_ = g.Close()
		z.gz = b.Bytes()
	}
	packedCache.Store(name, z)
	return z
}
