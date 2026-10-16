package ledger

import "sort"

type AdapterCatalogChannel struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Revision uint64   `json:"revision"`
	BotIDs   []string `json:"botIds"`
}
type AdapterCatalogBot struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type AdapterCatalog struct {
	Version     int                     `json:"version"`
	OwnerUserID string                  `json:"ownerUserId"`
	SpaceID     string                  `json:"spaceId"`
	Channels    []AdapterCatalogChannel `json:"channels"`
	Bots        []AdapterCatalogBot     `json:"bots"`
}

func (s *Store) AdapterCatalog() (AdapterCatalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return AdapterCatalog{}, ErrUnavailable
	}
	out := AdapterCatalog{Version: 1, OwnerUserID: "local-owner", SpaceID: "local-space", Channels: []AdapterCatalogChannel{}, Bots: []AdapterCatalogBot{}}
	for _, c := range s.channels {
		out.Channels = append(out.Channels, AdapterCatalogChannel{ID: c.ID, Name: c.Name, Revision: c.Revision, BotIDs: append([]string{}, c.BotIDs...)})
	}
	for _, b := range s.bots {
		out.Bots = append(out.Bots, AdapterCatalogBot{ID: b.ID, Name: b.Name})
	}
	sort.Slice(out.Channels, func(i, j int) bool { return out.Channels[i].ID < out.Channels[j].ID })
	sort.Slice(out.Bots, func(i, j int) bool { return out.Bots[i].ID < out.Bots[j].ID })
	return out, nil
}
