// Package outbound is the single path by which anything an agent drafted
// leaves the machine. It is only reached after the owner's approval has been
// journaled; adapters (email/SMTP first) deliver, and the Router enforces
// the recipient rule shared by every channel: a recipient must be something
// the owner typed, never something the model chose on its own.
package outbound

import (
	"context"
	"errors"
	"strings"
)

type Request struct {
	Kind    string // channel kind, e.g. "email"
	To      string
	Subject string
	Title   string
	Body    string
}

type Adapter interface {
	Kind() string
	Configured() bool
	Sender() string
	Validate(Request) error
	Deliver(context.Context, Request) error
}

var (
	ErrNoChannel      = errors.New("channel_not_connected")
	ErrNotConfigured  = errors.New("channel_not_configured")
	ErrRecipientGuard = errors.New("recipient_not_from_owner")
)

// OwnerTyped reports whether the owner wrote needle themselves.
type OwnerTyped func(needle string) bool

type Router struct{ adapters map[string]Adapter }

func NewRouter(a ...Adapter) *Router {
	r := &Router{adapters: map[string]Adapter{}}
	for _, x := range a {
		r.adapters[x.Kind()] = x
	}
	return r
}

func FromEnv() *Router { return NewRouter(MailerFromEnv()) }

func (r *Router) Adapter(kind string) (Adapter, bool) {
	a, ok := r.adapters[kind]
	return a, ok
}

// Check runs every pre-send rule without sending anything.
func (r *Router) Check(req Request, owner OwnerTyped) (Adapter, error) {
	a, ok := r.Adapter(req.Kind)
	if !ok {
		return nil, ErrNoChannel
	}
	if err := a.Validate(req); err != nil {
		return a, err
	}
	if strings.TrimSpace(req.To) != "" && (owner == nil || !owner(req.To)) {
		return a, ErrRecipientGuard
	}
	if !a.Configured() {
		return a, ErrNotConfigured
	}
	return a, nil
}
