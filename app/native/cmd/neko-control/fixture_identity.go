package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"nekobot.local/native/internal/identity"
	"nekobot.local/native/internal/protocol"
)

func loadFixtureIdentity(path, output string) (*identity.Authority, identity.Config, error) {
	var c identity.Config
	if path == "" || !filepath.IsAbs(output) {
		return nil, c, identity.ErrInvalid
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 16384 {
		return nil, c, identity.ErrInvalid
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, c, identity.ErrInvalid
	}
	canonical, err := protocol.Canonical(raw)
	if err != nil {
		return nil, c, identity.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(canonical))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return nil, c, identity.ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, c, identity.ErrInvalid
	}
	if err = fixtureOutputAvailable(output); err != nil {
		return nil, c, err
	}
	owner := false
	for _, record := range c.Actors {
		owner = owner || record.ActorID == "local-owner"
	}
	if !owner {
		return nil, c, identity.ErrInvalid
	}
	a, err := identity.NewFixture(c)
	return a, c, err
}

func fixtureOutputAvailable(path string) error {
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return errors.New("fixture_output_must_be_new")
	}
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return identity.ErrInvalid
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	return nil
}

func writeFixtureTickets(path string, a *identity.Authority, c identity.Config) error {
	type ticket struct {
		ActorID  string `json:"actorId"`
		DeviceID string `json:"deviceId"`
		Ticket   string `json:"ticket"`
	}
	entries := []ticket{}
	for _, record := range c.Actors {
		value, err := a.BootstrapTicket(record.ActorID, record.DeviceID)
		if err != nil {
			return err
		}
		entries = append(entries, ticket{record.ActorID, record.DeviceID, value})
	}
	raw, err := json.Marshal(struct {
		Version     int      `json:"version"`
		FixtureOnly bool     `json:"fixtureOnly"`
		AuthorityID string   `json:"authorityId"`
		IssuerEpoch uint64   `json:"issuerEpoch"`
		Sessions    []ticket `json:"sessions"`
	}{1, true, a.AuthorityID(), a.IssuerEpoch(), entries})
	if err != nil {
		return err
	}
	if err = fixtureOutputAvailable(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
