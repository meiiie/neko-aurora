package web

import (
	"embed"
	"net/http"
)

//go:embed assets/*
var assets embed.FS

var contentTypes = map[string]string{
	"/":                                    "text/html; charset=utf-8",
	"/app.js":                              "text/javascript; charset=utf-8",
	"/connections.js":                      "text/javascript; charset=utf-8",
	"/style.css":                           "text/css; charset=utf-8",
	"/duet/":                               "text/html; charset=utf-8",
	"/duet/agui.js":                        "text/javascript; charset=utf-8",
	"/duet/theme.js":                       "text/javascript; charset=utf-8",
	"/duet/app.js":                         "text/javascript; charset=utf-8",
	"/duet/style.css":                      "text/css; charset=utf-8",
	"/duet/sw.js":                          "text/javascript; charset=utf-8",
	"/duet/icon.svg":                       "image/svg+xml",
	"/duet/icon-180.png":                   "image/png",
	"/duet/icon-512.png":                   "image/png",
	"/duet/manifest.webmanifest":           "application/manifest+json",
	"/fonts/geist-latin-wght-normal.woff2": "font/woff2",
	"/fonts/geist-latin-ext-wght-normal.woff2":  "font/woff2",
	"/fonts/geist-vietnamese-wght-normal.woff2": "font/woff2",
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
	if name == "/" || name == "/duet/" {
		name += "index.html"
	}
	body, err := assets.ReadFile("assets" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	if name == "/duet/sw.js" {
		w.Header().Set("Service-Worker-Allowed", "/duet/")
		w.Header().Set("Cache-Control", "no-cache")
	}
	_, _ = w.Write(body)
}
