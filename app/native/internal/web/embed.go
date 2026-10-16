package web

import (
	"embed"
	"net/http"
)

//go:embed assets/*
var assets embed.FS

func Serve(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path
	if name == "/" {
		name = "/index.html"
	}
	body, err := assets.ReadFile("assets" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	types := map[string]string{"/index.html": "text/html; charset=utf-8", "/app.js": "text/javascript; charset=utf-8", "/connections.js": "text/javascript; charset=utf-8", "/style.css": "text/css; charset=utf-8"}
	w.Header().Set("Content-Type", types[name])
	_, _ = w.Write(body)
}
