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
	if name == "/" {
		name = "/index.html"
	}
	body, err := assets.ReadFile("assets" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Assets are embedded in the binary; browsers must not serve a stale copy
	// from heuristic cache after a control restart shipped new code.
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(body)
}
