//go:build !linux

package computer

import (
	"context"
	"fmt"
)

func (b *Bwrap) Available() error {
	return fmt.Errorf("%w: bubblewrap requires Linux", ErrUnavailable)
}

func (b *Bwrap) Exec(context.Context, Spec) (Result, error) {
	return Result{}, b.Available()
}
