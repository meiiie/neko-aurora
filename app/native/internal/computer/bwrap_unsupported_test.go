//go:build !linux

package computer

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestBwrapUnavailableOnUnsupportedOS(t *testing.T) {
	b := NewBwrap()
	b.Path, b.Prlimit = os.Args[0], os.Args[0]
	if err := b.Available(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unsupported OS admitted bubblewrap: %v", err)
	}
	if _, err := b.Exec(context.Background(), Spec{Cmd: "echo unsafe", Limits: DefaultLimits()}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unsupported OS executed a command: %v", err)
	}
}
