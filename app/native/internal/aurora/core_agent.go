package aurora

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"nekoaurora.local/native/internal/privatefs"
)

const coreFrameLimit = 8 << 20

type coreCompletion struct {
	Status   string   `json:"status"`
	Revision string   `json:"revision"`
	Summary  string   `json:"summary"`
	Evidence []string `json:"evidence"`
}

type coreAgentStart struct {
	Version       int             `json:"version"`
	TaskID        string          `json:"taskId"`
	History       []ChatMsg       `json:"history"`
	Instruction   string          `json:"instruction"`
	MaxSteps      int             `json:"maxSteps"`
	ContextWindow int             `json:"contextWindow"`
	Snapshot      json.RawMessage `json:"snapshot,omitempty"`
}

type coreAgentResult struct {
	Text       string         `json:"text"`
	Completion coreCompletion `json:"completion"`
}

type coreAgentHost interface {
	Request(context.Context, string, json.RawMessage) (any, error)
	Event(string, json.RawMessage) error
}

// Agent runs the full Core loop. Aurora is the exclusive tool host, not a second
// controller around completion-only CLI invocations. There is no silent fallback.
func (c *Core) Agent(ctx context.Context, start coreAgentStart, host coreAgentHost) (coreAgentResult, error) {
	var result coreAgentResult
	if !c.Configured() || !c.Quick {
		return result, ErrNoAI
	}
	if privatefs.CheckDirectory(c.Home) != nil {
		return result, errors.New("core_home_unavailable")
	}
	if err := c.validateSelection(ctx); err != nil {
		return result, err
	}
	entry := filepath.Join(filepath.Dir(c.Entry), "aurora-agent.ts")
	if _, err := os.Stat(entry); err != nil {
		return result, errors.New("core_agent_unavailable")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	args := append(c.bunFlags(), entry, c.Profile)
	cmd := exec.CommandContext(ctx, c.Bun, args...)
	cmd.Dir, cmd.Env = c.Home, c.environment()
	cmd.Env = append(cmd.Env, "NEKO_MODEL="+c.Model)
	if c.key != "" {
		cmd.Env = append(cmd.Env, first(c.KeyEnv, "ZAI_API_KEY")+"="+c.key)
	}
	cmd.WaitDelay = 5 * time.Second
	hideCoreWindow(cmd)
	input, err := cmd.StdinPipe()
	if err != nil {
		return result, err
	}
	// EOF aborts Core's loop and disposes its owned provider before forced
	// termination. WaitDelay remains the bound for an unresponsive child.
	cmd.Cancel = func() error { _ = input.Close(); return nil }
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return result, err
	}
	var stderr limited
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		input.Close()
		return result, errors.New("core_runtime_unavailable")
	}
	defer input.Close()
	protocolErr := writeCoreFrame(input, map[string]any{"type": "start", "request": start})
	gotResult := false
	requestSequence := 0
	reader := bufio.NewScanner(output)
	reader.Buffer(make([]byte, 64<<10), coreFrameLimit)
	for protocolErr == nil && reader.Scan() {
		var frame struct {
			Type       string          `json:"type"`
			ID         string          `json:"id"`
			Method     string          `json:"method"`
			Params     json.RawMessage `json:"params"`
			Kind       string          `json:"kind"`
			Value      json.RawMessage `json:"value"`
			Text       string          `json:"text"`
			Completion coreCompletion  `json:"completion"`
			Code       string          `json:"code"`
		}
		if json.Unmarshal(reader.Bytes(), &frame) != nil || gotResult {
			protocolErr = errors.New("core_protocol_invalid")
			break
		}
		switch frame.Type {
		case "request":
			requestSequence++
			if frame.ID != "r"+strconv.Itoa(requestSequence) {
				protocolErr = errors.New("core_protocol_invalid")
				break
			}
			value, requestErr := host.Request(ctx, frame.Method, frame.Params)
			response := map[string]any{"type": "response", "id": frame.ID, "result": value}
			if requestErr != nil {
				response = map[string]any{"type": "response", "id": frame.ID, "error": "core_host_operation_failed"}
				protocolErr = requestErr
			}
			if writeErr := writeCoreFrame(input, response); writeErr != nil {
				protocolErr = writeErr
			}
		case "event":
			protocolErr = host.Event(frame.Kind, frame.Value)
		case "result":
			if !validCoreCompletion(frame.Completion) {
				protocolErr = errors.New("core_protocol_invalid")
				break
			}
			gotResult = true
			result = coreAgentResult{Text: frame.Text, Completion: frame.Completion}
		case "error":
			protocolErr = coreExitError(frame.Code)
		default:
			protocolErr = errors.New("core_protocol_invalid")
		}
	}
	if protocolErr == nil {
		protocolErr = reader.Err()
	}
	if protocolErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if protocolErr != nil {
		return result, protocolErr
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if waitErr != nil {
		return result, coreExitError(stderr.String())
	}
	if !gotResult {
		return result, errors.New("core_result_missing")
	}
	return result, nil
}

func validCoreCompletion(c coreCompletion) bool {
	for _, e := range c.Evidence {
		if len(e) > 4000 {
			return false
		}
	}
	return (c.Status == "ready" || c.Status == "waiting" || c.Status == "incomplete") && c.Revision != "" && len(c.Revision) <= 128 && len(c.Summary) <= 32000 && len(c.Evidence) <= 64
}

func writeCoreFrame(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data)+1 >= coreFrameLimit {
		return errors.New("core_frame_too_large")
	}
	n, err := w.Write(append(data, '\n'))
	if err == nil && n != len(data)+1 {
		return io.ErrShortWrite
	}
	return err
}
