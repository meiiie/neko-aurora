package hub

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type User struct {
	ID                  string            `json:"id"`
	Email               string            `json:"email"`
	Name                string            `json:"name,omitempty"`
	Provider            string            `json:"provider"` // email|google|apple
	Subjects            map[string]string `json:"subjects,omitempty"`
	Plan                string            `json:"plan"`
	Created             string            `json:"created"`
	Onboarded           bool              `json:"onboarded,omitempty"`
	Gen                 int               `json:"gen"` // bump to revoke every session
	UsageDay            string            `json:"usageDay,omitempty"`
	Used                int               `json:"used,omitempty"`
	NetworkReservations map[string]string `json:"networkReservations,omitempty"`
}

type userStore struct {
	path string
	mu   sync.Mutex
	m    map[string]*User
}

var ErrUserLimit = errors.New("user_limit")

func cloudBrowserEmails(raw string) map[string]bool {
	allowed := map[string]bool{}
	for _, entry := range strings.Split(raw, ",") {
		if email, ok := validEmail(entry); ok {
			allowed[email] = true
		}
	}
	return allowed
}

func openUsers(path string) (*userStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	s := &userStore{path: path, m: map[string]*User{}}
	if b, err := os.ReadFile(path); err == nil {
		var l []*User
		if err := json.Unmarshal(b, &l); err != nil {
			return nil, err
		}
		for _, u := range l {
			s.m[u.ID] = u
		}
	}
	return s, nil
}

func (s *userStore) saveLocked() error {
	l := make([]*User, 0, len(s.m))
	for _, u := range s.m {
		l = append(l, u)
	}
	b, _ := json.Marshal(l)
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *userStore) get(id string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.m[id]
	if !ok {
		return User{}, false
	}
	return *u, true
}

// signIn finds the account for a verified identity (provider subject first,
// then verified email) or creates one.
func (s *userStore) signIn(email, provider, subject, name, plan string, max int, now time.Time) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.m {
		if subject != "" && u.Subjects[provider] == subject || email != "" && u.Email == email {
			if subject != "" {
				if u.Subjects == nil {
					u.Subjects = map[string]string{}
				}
				u.Subjects[provider] = subject
			}
			if u.Email == "" {
				u.Email = email
			}
			_ = s.saveLocked()
			return *u, nil
		}
	}
	if len(s.m) >= max {
		return User{}, ErrUserLimit
	}
	u := &User{ID: "u" + randHex(10), Email: email, Name: name, Provider: provider, Plan: plan, Created: now.UTC().Format(time.RFC3339)}
	if subject != "" {
		u.Subjects = map[string]string{provider: subject}
	}
	s.m[u.ID] = u
	return *u, s.saveLocked()
}

func (s *userStore) update(id string, fn func(*User)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.m[id]
	if !ok {
		return errors.New("no user")
	}
	fn(u)
	return s.saveLocked()
}

func (s *userStore) remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
	_ = s.saveLocked()
}

func day(now time.Time) string { return now.UTC().Format("2006-01-02") }

// spend books one metered request against today's plan quota.
func (s *userStore) spend(id string, now time.Time, limit int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.m[id]
	if !ok {
		return false
	}
	previousDay, previousUsed := u.UsageDay, u.Used
	if u.UsageDay != day(now) {
		u.UsageDay, u.Used = day(now), 0
	}
	if u.Used >= limit {
		return false
	}
	u.Used++
	if err := s.saveLocked(); err != nil {
		u.UsageDay, u.Used = previousDay, previousUsed
		return false
	}
	return true
}

// reserveNetwork durably reserves one model turn per immutable mailbox ID.
// A failed later ledger admission retains this conservative reservation.
// IDs are kept longer than the mailbox's seven-day dispatch lifetime, so
// pruning cannot admit an expired envelope without its original reservation.
func (s *userStore) reserveNetwork(id, messageID string, now time.Time, limit int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.m[id]
	if !ok || messageID == "" {
		return errors.New("network_identity_unavailable")
	}
	if _, exists := u.NetworkReservations[messageID]; exists {
		return nil
	}
	priorDay, priorUsed, priorReservations := u.UsageDay, u.Used, u.NetworkReservations
	reservations := make(map[string]string, len(priorReservations)+1)
	for key, at := range priorReservations {
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil || now.Sub(t) <= 8*24*time.Hour {
			reservations[key] = at
		}
	}
	if len(reservations) >= 16384 {
		return errors.New("network_reservation_limit")
	}
	used := u.Used
	if u.UsageDay != day(now) {
		used = 0
	}
	if used >= limit {
		return errors.New("network_quota_exceeded")
	}
	reservations[messageID] = now.UTC().Format(time.RFC3339Nano)
	u.UsageDay, u.Used, u.NetworkReservations = day(now), used+1, reservations
	if err := s.saveLocked(); err != nil {
		u.UsageDay, u.Used, u.NetworkReservations = priorDay, priorUsed, priorReservations
		return errors.New("network_quota_storage_unavailable")
	}
	return nil
}

func (s *userStore) usedToday(id string, now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.m[id]; ok && u.UsageDay == day(now) {
		return u.Used
	}
	return 0
}
