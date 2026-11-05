package aurora

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/privatefs"
)

var coreStateID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var coreOperationID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type coreEffect struct {
	Digest string        `json:"digest"`
	Status string        `json:"status"` // started|completed; started across a crash is an unknown outcome
	Reply  coreToolReply `json:"reply"`
}
type coreToolReply struct {
	Text    string          `json:"text"`
	Waiting *coreCompletion `json:"waiting,omitempty"`
}
type coreAgentState struct {
	Version      int                   `json:"version"`
	TaskID       string                `json:"taskId"`
	BotID        string                `json:"botId"`
	Epoch        uint64                `json:"epoch"`
	Model        string                `json:"model"`
	Profile      string                `json:"profile"`
	Folder       string                `json:"folder"`
	Snapshot     json.RawMessage       `json:"snapshot,omitempty"`
	Effects      map[string]coreEffect `json:"effects"`
	Written      []string              `json:"written,omitempty"`
	Presentation *ledger.DirectRun     `json:"presentation,omitempty"`
	Pending      string                `json:"pending,omitempty"`
	LastExit     *int                  `json:"lastExit,omitempty"`
	Result       *coreAgentResult      `json:"result,omitempty"`
	file         string
}

func openCoreAgentState(root string, w ledger.Work, c *Core, folder ...string) (*coreAgentState, error) {
	if root == "" || !coreStateID.MatchString(w.RequestID) {
		return nil, errors.New("core_task_invalid")
	}
	dir := filepath.Join(root, "core-agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := privatefs.ProtectDirectory(dir); err != nil {
		return nil, err
	}
	s := &coreAgentState{Version: 1, TaskID: w.RequestID, BotID: w.BotID, Epoch: w.Epoch, Model: c.Model, Profile: c.Profile,
		Effects: map[string]coreEffect{}, file: filepath.Join(dir, w.RequestID+".json")}
	if len(folder) > 0 {
		s.Folder = folder[0]
	}
	info, err := os.Lstat(s.file)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 32<<20 || privatefs.Check(s.file) != nil {
		return nil, errors.New("core_checkpoint_unavailable")
	}
	data, err := os.ReadFile(s.file)
	if err != nil {
		return nil, err
	}
	var saved coreAgentState
	if json.Unmarshal(data, &saved) != nil || saved.Version != 1 || saved.TaskID != s.TaskID || saved.BotID != s.BotID || saved.Epoch != s.Epoch || saved.Model != s.Model || saved.Profile != s.Profile || saved.Folder != s.Folder || len(saved.Effects) > 128 || len(saved.Written) > 64 {
		return nil, errors.New("core_checkpoint_binding_invalid")
	}
	if saved.Effects == nil {
		saved.Effects = map[string]coreEffect{}
	}
	if saved.Result != nil && (!validCoreCompletion(saved.Result.Completion) || len(saved.Result.Text) > 2<<20) {
		return nil, errors.New("core_checkpoint_invalid")
	}
	for id, effect := range saved.Effects {
		if !coreOperationID.MatchString(id) || len(effect.Digest) != 64 || (effect.Status != "started" && effect.Status != "completed") || len(effect.Reply.Text) > 2<<20 || effect.Reply.Waiting != nil && !validCoreCompletion(*effect.Reply.Waiting) {
			return nil, errors.New("core_checkpoint_invalid")
		}
	}
	if len(saved.Snapshot) > 0 {
		if _, err := decodeCoreSnapshot(saved.Snapshot); err != nil {
			return nil, err
		}
	}
	saved.file = s.file
	return &saved, nil
}

// A crash can occur after the durable effect receipt but before Core records
// its tool observation. Repair that gap from the receipt, never by re-executing.
func (s *coreAgentState) reconcileSnapshot() error {
	if len(s.Snapshot) == 0 {
		return nil
	}
	cp, err := decodeCoreSnapshot(s.Snapshot)
	if err != nil {
		return err
	}
	var raw struct {
		Version  int               `json:"version"`
		Messages []json.RawMessage `json:"messages"`
		Accepted []string          `json:"acceptedInputs"`
	}
	if json.Unmarshal(s.Snapshot, &raw) != nil {
		return errors.New("core_checkpoint_invalid")
	}
	var out []json.RawMessage
	changed := false
	for i, m := range cp.Messages {
		out = append(out, raw.Messages[i])
		for _, op := range m.Operations {
			if op.Version != 1 {
				return errors.New("core_operation_invalid")
			}
			effect, ok := s.Effects[op.ID]
			if !ok {
				continue
			} // checkpointed but never admitted by the host
			if effect.Status != "completed" {
				return errors.New("core_effect_unknown")
			}
			found := false
			for j := i + 1; j < len(raw.Messages); j++ {
				var result struct {
					Role string `json:"role"`
					ID   string `json:"tool_call_id"`
				}
				_ = json.Unmarshal(raw.Messages[j], &result)
				if result.Role == "assistant" {
					break
				}
				if result.Role == "tool" && result.ID == op.Call {
					found = true
					break
				}
			}
			if found {
				continue
			}
			result, err := json.Marshal(map[string]any{"role": "tool", "tool_call_id": op.Call, "content": effect.Reply.Text})
			if err != nil {
				return err
			}
			out = append(out, result)
			changed = true
		}
	}
	if changed {
		raw.Messages = out
		s.Snapshot, err = json.Marshal(raw)
		if err != nil {
			return err
		}
		return s.save()
	}
	return nil
}

func (s *coreAgentState) save() error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if len(data) > 32<<20 {
		return errors.New("core_checkpoint_too_large")
	}
	return privatefs.WriteFile(s.file, data)
}

func coreDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
