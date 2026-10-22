package web

import (
	"embed"
	"net/http"
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
	"/app/style.css":             "text/css; charset=utf-8",
	"/app/sw.js":                 "text/javascript; charset=utf-8",
	"/app/icon.svg":              "image/svg+xml",
	"/app/icon-180.png":          "image/png",
	"/app/icon-512.png":          "image/png",
	"/app/icon-192.png":          "image/png",
	"/app/icon-maskable-512.png": "image/png",
	"/app/favicon.ico":           "image/x-icon",
	"/app/manifest.webmanifest":  "application/manifest+json",
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
		w.Header().Set("Cache-Control", "no-cache")
	}
	_, _ = w.Write(body)
}
