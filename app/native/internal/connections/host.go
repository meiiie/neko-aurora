package connections

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"time"
)

type HostSealer struct {
	Executable string
	State      string
}

func (h HostSealer) Seal(accountID, reference string, secret []byte) error {
	_, err := h.command("--vault-seal", accountID, reference, secret)
	return err
}
func (h HostSealer) Match(accountID, reference string, secret []byte) (bool, error) {
	return h.command("--vault-match", accountID, reference, secret)
}
func (h HostSealer) command(action, accountID, reference string, secret []byte) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, h.Executable, action, "--state-root", h.State, "--binding", "local-owner:local-space:"+accountID, "--secret-ref", reference)
	hideWindow(cmd)
	cmd.Env = []string{}
	for _, key := range []string{"SystemRoot", "WINDIR", "TEMP", "TMP", "USERPROFILE"} {
		if value := os.Getenv(key); value != "" {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	frame, err := json.Marshal(map[string]string{"secret": string(secret)})
	if err != nil {
		return false, ErrUnavailable
	}
	defer func() {
		for i := range frame {
			frame[i] = 0
		}
	}()
	cmd.Stdin = bytes.NewReader(frame)
	cmd.Stderr = io.Discard
	var output bytes.Buffer
	cmd.Stdout = &output
	if cmd.Run() != nil {
		return false, ErrUnavailable
	}
	var result struct {
		Sealed  bool `json:"sealed"`
		Matches bool `json:"matches"`
	}
	if output.Len() > 128 || json.Unmarshal(output.Bytes(), &result) != nil || (action == "--vault-seal" && !result.Sealed) {
		return false, ErrUnavailable
	}
	return result.Matches, nil
}
