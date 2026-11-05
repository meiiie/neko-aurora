package aurora

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sync"
	"time"

	"nekoaurora.local/native/internal/privatefs"
)

type quickModelOption struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
	Issue     string `json:"issue,omitempty"`
	Transport string `json:"transport"`
}
type quickModelCatalog struct {
	Models    []quickModelOption `json:"models"`
	CheckedAt string             `json:"checkedAt"`
	Error     string             `json:"error,omitempty"`
}

var coreModelID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,119}$`)

// Owned by one turnAI snapshot, never shared with another account or turn.
// A tool loop should not depend on catalog discovery succeeding at every step.
type coreValidation struct {
	mu       sync.Mutex
	identity [32]byte
	checked  time.Time
}

func (c *Core) validateSelection(ctx context.Context) error {
	identity := sha256.Sum256([]byte(c.Home + "\x00" + c.Profile + "\x00" + c.Model + "\x00" + c.key))
	if c.validation != nil {
		c.validation.mu.Lock()
		defer c.validation.mu.Unlock()
		if c.validation.identity == identity && time.Since(c.validation.checked) < 5*time.Minute {
			return ctx.Err()
		}
	}
	catalog, err := c.providerModels(ctx, c.Profile, c.KeyEnv, c.key)
	if err != nil {
		return err
	}
	if issue := chatGPTSelectionIssue(catalog, c.Model); issue != "" {
		return errors.New(issue)
	}
	if c.validation != nil {
		c.validation.identity, c.validation.checked = identity, time.Now()
	}
	return nil
}

func (c *Core) chatGPTModels(ctx context.Context) (quickModelCatalog, error) {
	return c.providerModels(ctx, "chatgpt", "", "")
}

func (c *Core) providerModels(ctx context.Context, provider, keyEnv, key string) (quickModelCatalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if privatefs.CheckDirectory(c.Home) != nil {
		return quickModelCatalog{}, errors.New("core_home_unavailable")
	}
	cmd := c.setupCommand(ctx, "models")
	cmd.Args = append(cmd.Args, provider)
	if key != "" && keyEnv != "" {
		cmd.Env = append(cmd.Env, keyEnv+"="+key)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return quickModelCatalog{}, errors.New("core_catalog_unavailable")
	}
	if cmd.Start() != nil {
		return quickModelCatalog{}, errors.New("core_runtime_unavailable")
	}
	data, readErr := io.ReadAll(io.LimitReader(out, 256*1024+1))
	if readErr != nil || len(data) > 256*1024 {
		_ = cmd.Process.Kill()
	}
	runErr := cmd.Wait()
	var result quickModelCatalog
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if readErr != nil || len(data) > 256*1024 || json.Unmarshal(data, &result) != nil {
		return result, errors.New("core_catalog_unavailable")
	}
	if result.Error != "" {
		switch result.Error {
		case "core_auth_required", "core_rate_limited", "core_catalog_unavailable":
			return result, errors.New(result.Error)
		}
		return quickModelCatalog{}, errors.New("core_catalog_unavailable")
	}
	if runErr != nil || result.Models == nil || len(result.Models) > 256 {
		return quickModelCatalog{}, errors.New("core_catalog_unavailable")
	}
	for _, m := range result.Models {
		if !coreModelID.MatchString(m.ID) || len(m.Label) > 640 || (m.Transport != "codex" && m.Transport != "responses" && m.Transport != "anthropic" && m.Transport != "openai_compat") {
			return quickModelCatalog{}, errors.New("core_catalog_unavailable")
		}
	}
	return result, nil
}

func (q *quickSetup) providers() []quickProvider {
	q.mu.Lock()
	defer q.mu.Unlock()
	rows := append([]quickProvider(nil), q.Providers...)
	for i := range rows {
		p := &rows[i]
		if p.ID != "chatgpt" {
			continue
		}
		p.CatalogError = q.modelsError
		if q.models == nil {
			continue
		}
		p.ModelSource, p.CheckedAt, p.ModelOptions = "account", q.models.CheckedAt, q.models.Models
		p.Models = []string{}
		for _, m := range p.ModelOptions {
			if m.Available {
				p.Models = append(p.Models, m.ID)
			}
		}
	}
	return rows
}

func (q *quickSetup) refreshModels(ctx context.Context) error {
	q.modelsMu.Lock()
	defer q.modelsMu.Unlock()
	q.mu.Lock()
	attempt := q.login.ID
	pending := q.login.State == "starting" || q.login.State == "waiting"
	q.mu.Unlock()
	if pending || ctx.Err() != nil {
		return errors.New("core_catalog_unavailable")
	}
	catalog, err := q.runModels(ctx)
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.login.ID != attempt {
		return errors.New("core_catalog_unavailable")
	}
	q.modelsError = ""
	if err != nil {
		q.modelsError = "core_catalog_unavailable"
		for _, code := range []string{"core_auth_required", "core_rate_limited"} {
			if err.Error() == code {
				q.modelsError = code
			}
		}
		return err
	}
	q.models = &catalog
	return nil
}

func chatGPTSelectionIssue(catalog quickModelCatalog, model string) string {
	for _, m := range catalog.Models {
		if m.ID == model {
			if m.Available {
				return ""
			}
			return "core_support_required"
		}
	}
	return "core_model_unavailable"
}
