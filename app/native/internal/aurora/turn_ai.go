package aurora

import (
	"strings"
)

type turnAIKey struct{}

// A running turn keeps its route and API credential across tool steps. Settings
// edits affect the next turn; they must not silently change an in-flight reply.
type turnAI struct {
	bot    string
	config botExt
	core   *Core
	cfg    callCfg
	ai     *AI
	ok     bool
}

func (s *Service) selectTurnAI(bot string) *turnAI {
	x := &turnAI{bot: bot}
	if s.ext == nil {
		return x
	}
	x.config = s.ext.bot(bot)
	if x.config.Model == "" {
		s.ext.mu.Lock()
		x.config.Model = s.ext.d.DefaultModel
		s.ext.mu.Unlock()
	}
	if strings.HasPrefix(x.config.Model, "core-") {
		x.core = s.quickModel(x.config.Model)
	} else if x.config.Model != "" || x.config.Thinking {
		x.cfg, x.ai, x.ok = s.callFor(x.config.Model)
	}
	return x
}

func (x *turnAI) identity(fallback Model) (string, string) {
	if strings.HasPrefix(x.config.Model, "core-") {
		if x.core != nil {
			return x.core.Runtime(), x.core.ModelName()
		}
		return "neko-core-cli", ""
	}
	if x.ai != nil && x.ok {
		return x.ai.Runtime(), first(x.cfg.Model, x.ai.ModelName())
	}
	if x.config.Model == "" && fallback != nil && fallback.Configured() {
		return fallback.Runtime(), fallback.ModelName()
	}
	if fallback != nil {
		return fallback.Runtime(), ""
	}
	return "direct-http", ""
}
