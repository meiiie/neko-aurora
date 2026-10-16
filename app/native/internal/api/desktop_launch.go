package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type desktopLaunchBinding struct {
	Version         int    `json:"version"`
	Endpoint        string `json:"endpoint"`
	ControlPID      int    `json:"controlPid"`
	ControlSHA256   string `json:"controlSha256"`
	StateSHA256     string `json:"stateSha256"`
	CreatedAtUnixMS int64  `json:"createdAtUnixMs"`
	ExpiresAtUnixMS int64  `json:"expiresAtUnixMs"`
}
type desktopLaunchReceipt struct {
	desktopLaunchBinding
	Challenge string `json:"challenge"`
}

func (s *Server) EnableDesktopLaunch(state, path string) error {
	invalid := errors.New("desktop_launch_unavailable")
	host, port, err := net.SplitHostPort(s.host)
	n, e := strconv.Atoi(port)
	if err != nil || e != nil || host != "127.0.0.1" || n < 1 || n > 65535 || port != strconv.Itoa(n) || s.desktop != nil {
		return invalid
	}
	state, err = filepath.Abs(state)
	if err != nil {
		return invalid
	}
	resolved, err := filepath.EvalSymlinks(state)
	if err != nil {
		return invalid
	}
	path, err = filepath.Abs(path)
	if err != nil || filepath.Dir(path) != state {
		return invalid
	}
	for cursor := state; ; cursor = filepath.Dir(cursor) {
		info, e := os.Lstat(cursor)
		if e != nil || info.Mode()&os.ModeSymlink != 0 {
			return invalid
		}
		if filepath.Dir(cursor) == cursor {
			break
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return invalid
	}
	raw, err := os.ReadFile(executable)
	if err != nil {
		return invalid
	}
	binaryHash := sha256.Sum256(raw)
	normalized := strings.TrimPrefix(strings.ReplaceAll(resolved, "\\", "/"), "//?/")
	bytes := []byte(normalized)
	for i, c := range bytes {
		if c >= 'A' && c <= 'Z' {
			bytes[i] = c + 32
		}
	}
	stateHash := sha256.Sum256(bytes)
	var challenge [32]byte
	if _, err = rand.Read(challenge[:]); err != nil {
		return invalid
	}
	now := time.Now().UnixMilli()
	receipt := desktopLaunchReceipt{desktopLaunchBinding{1, "http://" + s.host, os.Getpid(), hex.EncodeToString(binaryHash[:]), hex.EncodeToString(stateHash[:]), now, now + 30000}, hex.EncodeToString(challenge[:])}
	data, err := json.Marshal(receipt)
	if err != nil {
		return invalid
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return invalid
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return invalid
	}
	s.desktop = &receipt
	return nil
}

func (s *Server) desktopInstance(w http.ResponseWriter, r *http.Request) {
	if s.desktop == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.URL.ForceQuery || r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+s.host || !hmac.Equal([]byte(r.Header.Get("X-Neko-Local-Launch")), []byte(s.desktop.Challenge)) {
		http.Error(w, "local_launch_required", http.StatusForbidden)
		return
	}
	send(w, 200, s.desktop.desktopLaunchBinding)
}
