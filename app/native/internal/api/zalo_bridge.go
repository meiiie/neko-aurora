package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// The Zalo web bridge drives a Chrome window the user logged into once
// (QR). It lives as a Node script next to the toolchain; the control only
// starts/stops it and reports status.
type zaloBridge struct {
	mu         sync.Mutex
	listen     *exec.Cmd
	nodePath   string
	script     string
	dir        string
	statusPath string
}

func resolveZaloBridge() *zaloBridge {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	root := filepath.Dir(exe) // app/.build
	for i := 0; i < 2 && filepath.Base(root) != "app"; i++ {
		root = filepath.Dir(root)
	}
	workspace := filepath.Dir(filepath.Dir(root))
	dir := filepath.Join(workspace, "tools", "autorec")
	script := filepath.Join(dir, "zalo-bridge.mjs")
	if _, err := os.Stat(script); err != nil {
		fmt.Fprintln(os.Stderr, "zalo bridge script missing:", script, "workspace:", workspace)
		return nil
	}
	node := os.Getenv("NEKOBOT_NODE")
	if node == "" {
		for _, candidate := range []string{
			filepath.Join(workspace, "tools", "node", "node.exe"),
			`C:\Program Files\nodejs\node.exe`,
		} {
			if _, err := os.Stat(candidate); err == nil {
				node = candidate
				break
			}
		}
	}
	if node == "" {
		node = "node"
	}
	return &zaloBridge{nodePath: node, script: script, dir: dir, statusPath: filepath.Join(workspace, "neko-bot", "app", ".state", "local", "zalo-status.json")}
}

func (s *Server) zaloBridgeControl(w http.ResponseWriter, r *http.Request) {
	if s.zalo == nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/zalo-bridge":
		s.zalo.mu.Lock()
		listening := s.zalo.listen != nil && s.zalo.listen.Process != nil
		s.zalo.mu.Unlock()
		loggedIn := false
		if raw, err := os.ReadFile(s.zalo.statusPath); err == nil {
			var parsed struct {
				LoggedIn bool `json:"loggedIn"`
			}
			if json.Unmarshal(raw, &parsed) == nil {
				loggedIn = parsed.LoggedIn
			}
		}
		send(w, http.StatusOK, map[string]any{"version": 1, "listening": listening, "loggedIn": loggedIn})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/zalo-bridge":
		var input struct {
			Action string `json:"action"`
		}
		if decode(w, r, &input) != nil || (input.Action != "login" && input.Action != "listen" && input.Action != "stop") {
			sendError(w, errWebConnectionInvalid)
			return
		}
		s.zalo.mu.Lock()
		defer s.zalo.mu.Unlock()
		if input.Action == "stop" {
			if s.zalo.listen != nil && s.zalo.listen.Process != nil {
				_ = s.zalo.listen.Process.Kill()
			}
			s.zalo.listen = nil
			send(w, http.StatusOK, map[string]any{"version": 1, "stopped": true})
			return
		}
		command := exec.Command(s.zalo.nodePath, s.zalo.script, input.Action)
		command.Dir = s.zalo.dir
		if input.Action == "listen" {
			if s.zalo.listen != nil && s.zalo.listen.Process != nil {
				send(w, http.StatusOK, map[string]any{"version": 1, "listening": true, "duplicate": true})
				return
			}
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr
			if err := command.Start(); err != nil {
				sendError(w, errWebConnectionInvalid)
				return
			}
			s.zalo.listen = command
			go command.Wait()
			send(w, http.StatusOK, map[string]any{"version": 1, "listening": true})
			return
		}
		// login: open the window, then watch for the QR scan so the UI can
		// report "logged in" and auto-start listening.
		if err := command.Start(); err != nil {
			sendError(w, errWebConnectionInvalid)
			return
		}
		go command.Wait()
		watcher := exec.Command(s.zalo.nodePath, s.zalo.script, "wait-login")
		watcher.Dir = s.zalo.dir
		watcher.Stdout = os.Stdout
		watcher.Stderr = os.Stderr
		if watcher.Start() == nil {
			go watcher.Wait()
		}
		send(w, http.StatusOK, map[string]any{"version": 1, "login_window": true})
	default:
		http.NotFound(w, r)
	}
}
