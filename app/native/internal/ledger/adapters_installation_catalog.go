package ledger

import (
	"nekoaurora.local/native/internal/executors"
	"sort"
)

type AdapterCatalogProject struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Revision   uint64   `json:"revision"`
	ChannelIDs []string `json:"channelIds"`
}
type AdapterInstallationCatalog struct {
	Version      int                     `json:"version"`
	OwnerUserID  string                  `json:"ownerUserId"`
	SpaceID      string                  `json:"spaceId"`
	Installation executors.InstallPin    `json:"installation"`
	Projects     []AdapterCatalogProject `json:"projects"`
}

func (s *Store) AdapterInstallationCatalog() (AdapterInstallationCatalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned || !s.executorRequired || s.executor == nil {
		return AdapterInstallationCatalog{}, ErrUnavailable
	}
	pin, err := s.executor.ReadInstallation()
	if err != nil {
		return AdapterInstallationCatalog{}, ErrUnavailable
	}
	out := AdapterInstallationCatalog{Version: 1, OwnerUserID: "local-owner", SpaceID: "local-space", Installation: pin, Projects: []AdapterCatalogProject{}}
	for _, p := range s.projects {
		out.Projects = append(out.Projects, AdapterCatalogProject{ID: p.ID, Name: p.Name, Revision: p.Revision, ChannelIDs: append([]string{}, p.ChannelIDs...)})
	}
	sort.Slice(out.Projects, func(i, j int) bool { return out.Projects[i].ID < out.Projects[j].ID })
	return out, nil
}
