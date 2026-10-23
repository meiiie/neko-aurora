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
	ID        string            `json:"id"`
	Email     string            `json:"email"`
	Name      string            `json:"name,omitempty"`
	Provider  string            `json:"provider"` // email|google|apple
	Subjects  map[string]string `json:"subjects,omitempty"`
	Plan      string            `json:"plan"`
	Created   string            `json:"created"`
	Onboarded bool              `json:"onboarded,omitempty"`
	Gen       int               `json:"gen"` // bump to revoke every session
	UsageDay  string            `json:"usageDay,omitempty"`
	Used      int               `json:"used,omitempty"`
}

type userStore struct {
	path string
	mu   sync.Mutex
	m    map[string]*User
}

var ErrUserLimit = errors.New("user_limit")

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
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
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
	if u.UsageDay != day(now) {
		u.UsageDay, u.Used = day(now), 0
	}
	if u.Used >= limit {
		return false
	}
	u.Used++
	_ = s.saveLocked()
	return true
}

func (s *userStore) usedToday(id string, now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.m[id]; ok && u.UsageDay == day(now) {
		return u.Used
	}
	return 0
}
