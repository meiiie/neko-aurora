package outbound

import (
	"context"
	"testing"
)

type fake struct{ sent []Request }

func (f *fake) Kind() string                               { return "chat" }
func (f *fake) Configured() bool                           { return true }
func (f *fake) Sender() string                             { return "bot" }
func (f *fake) Validate(Request) error                     { return nil }
func (f *fake) Deliver(_ context.Context, r Request) error { f.sent = append(f.sent, r); return nil }

func TestRouterGuards(t *testing.T) {
	f := &fake{}
	r := NewRouter(f, &Mailer{})
	if _, err := r.Check(Request{Kind: "sms"}, nil); err != ErrNoChannel {
		t.Fatal(err)
	}
	if _, err := r.Check(Request{Kind: "chat", To: "x@y.z"}, func(string) bool { return false }); err != ErrRecipientGuard {
		t.Fatal(err)
	}
	if _, err := r.Check(Request{Kind: "email", To: "not an address"}, func(string) bool { return true }); err == nil {
		t.Fatal("bad address accepted")
	}
	if _, err := r.Check(Request{Kind: "email", To: "a@b.co"}, func(string) bool { return true }); err != ErrNotConfigured {
		t.Fatal(err)
	}
	if a, err := r.Check(Request{Kind: "chat", To: "x"}, func(string) bool { return true }); err != nil || a.Deliver(context.Background(), Request{Body: "hi"}) != nil || len(f.sent) != 1 {
		t.Fatal(err)
	}
}
