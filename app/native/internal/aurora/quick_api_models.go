package aurora

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
)

type quickAPICatalog struct {
	key     [32]byte
	catalog quickModelCatalog
}

func (s *Service) quickKey(provider string) string {
	if s.V == nil {
		return ""
	}
	key, _ := s.V.Get(providerKey("core-" + provider))
	return key
}

func catalogProvider(p quickProvider, catalog quickModelCatalog) quickProvider {
	p.CatalogError = catalog.Error
	if catalog.Models == nil {
		return p
	}
	p.ModelSource, p.CheckedAt, p.ModelOptions = "account", catalog.CheckedAt, catalog.Models
	p.Models = []string{}
	for _, m := range catalog.Models {
		if m.Available {
			p.Models = append(p.Models, m.ID)
		}
	}
	return p
}

func (s *Service) quickProviders() []quickProvider {
	if s.quick == nil {
		return nil
	}
	rows := s.quick.providers()
	for i, p := range rows {
		if p.Auth == "chatgpt_oauth" {
			continue
		}
		key := s.quickKey(p.ID)
		s.quick.mu.Lock()
		cached, ok := s.quick.apiModels[p.ID]
		s.quick.mu.Unlock()
		if ok && key != "" && cached.key == sha256.Sum256([]byte(key)) {
			rows[i] = catalogProvider(p, cached.catalog)
		}
	}
	return rows
}

func (s *Service) quickProvider(id string) (quickProvider, bool) {
	for _, p := range s.quickProviders() {
		if p.ID == id {
			return p, true
		}
	}
	return quickProvider{}, false
}

func (s *Service) rememberAPIModels(p quickProvider, key string, catalog quickModelCatalog) {
	q := s.quick
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.apiModels == nil {
		q.apiModels = map[string]quickAPICatalog{}
	}
	fingerprint := sha256.Sum256([]byte(key))
	if catalog.Error != "" {
		if old, ok := q.apiModels[p.ID]; ok && old.key == fingerprint {
			old.catalog.Error = catalog.Error
			q.apiModels[p.ID] = old
			return
		}
	}
	q.apiModels[p.ID] = quickAPICatalog{key: fingerprint, catalog: catalog}
}

func (s *Service) loadAPIModels(ctx context.Context, p quickProvider, key string) (quickModelCatalog, error) {
	if key == "" {
		return quickModelCatalog{}, errors.New("core_auth_required")
	}
	if s.quick.runAPIModels != nil {
		return s.quick.runAPIModels(ctx, p, key)
	}
	return s.quick.Core.providerModels(ctx, p.ID, p.KeyEnv, key)
}

func validQuickKey(key string) bool { return len(key) <= 8192 && !strings.ContainsAny(key, "\r\n\x00") }

func catalogErrorCode(err error) string {
	if err != nil {
		for _, code := range []string{"core_auth_required", "core_rate_limited", "core_model_unavailable"} {
			if err.Error() == code {
				return code
			}
		}
	}
	return "core_catalog_unavailable"
}
