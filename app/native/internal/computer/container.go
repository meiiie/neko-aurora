package computer

import (
	"context"
	"fmt"
)

// Container is the production backend for hosts without unprivileged user
// namespaces (Railway, Fly Machines, most PaaS). It is a stub until a runtime
// is chosen; Available reports why, and the app shows the computer as off.
//
// Railway Sandboxes (railway.go) is the implemented production backend.
// Other options, if Neko Aurora moves off Railway:
//  1. Fly Machines / Firecracker microVM per bot: start on demand, volume per
//     bot mounted at /home/bot, stop after idle. Strongest isolation; pay
//     per second. Exec over the Machines API (exec endpoint).
//  2. Docker + gVisor (runsc) on a rented VM (Hetzner/OVH): one container
//     per bot, `--runtime=runsc --network=none --memory=1g --pids-limit=256
//     --cpus=1 --read-only -v bot-home:/home/bot`. Cheapest at scale; we run
//     the VM.
//  3. Hosted sandboxes (E2B, Daytona, Modal): API create/exec/files, per-hour
//     billing, fastest to ship. Persist the home by snapshotting or syncing
//     files back into state/computers/<bot>/home.
//
// Whatever the runtime, keep the bwrap contract: clean env (no host
// secrets), home is the only writable persistent path, network off unless
// the bot's policy allows, CPU/memory/time/output caps, output untrusted.
type Container struct {
	Runtime string // "fly" | "gvisor" | "e2b"
}

func (c *Container) Name() string { return "container:" + c.Runtime }

func (c *Container) Available() error {
	return fmt.Errorf("%w: container backend %q not configured", ErrUnavailable, c.Runtime)
}

func (c *Container) Exec(context.Context, Spec) (Result, error) { return Result{}, c.Available() }

// FromEnv picks the backend:
// AURORA_COMPUTER=off|bwrap|railway|container:<runtime>.
func FromEnv(v string) Sandbox {
	switch {
	case v == "off":
		return nil
	case v == "railway":
		return NewRailwayFromEnv()
	case len(v) > 10 && v[:10] == "container:":
		return &Container{Runtime: v[10:]}
	default:
		return NewBwrap()
	}
}
