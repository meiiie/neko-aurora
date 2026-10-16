//go:build !windows

package providerexec

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Run the swap race in a disposable child: an old blocking FIFO open must
// fail this test without leaving a stuck reader in the parent test process.
func TestWitnessRegularToFIFOSwapCannotBlock(t *testing.T) {
	if os.Getenv("NEKO_TEST_WITNESS_FIFO_RACE") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWitnessRegularToFIFOSwapCannotBlock$")
		cmd.Env = append(os.Environ(), "NEKO_TEST_WITNESS_FIFO_RACE=1")
		output, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatal("witness reader blocked during regular-file/FIFO swap")
		}
		if err != nil {
			t.Fatalf("isolated FIFO race: %v\n%s", err, output)
		}
		return
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "witness.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if raw, err := readPrivateWitness(path); err != nil || string(raw) != "{}" {
		t.Fatal("regular witness unreadable", err)
	}
	stop := make(chan struct{})
	started := make(chan struct{})
	var swaps atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			fifo := filepath.Join(dir, "next-fifo")
			if err := syscall.Mkfifo(fifo, 0600); err != nil {
				panic(err)
			}
			if err := os.Rename(fifo, path); err != nil {
				panic(err)
			}
			regular := filepath.Join(dir, "next-regular")
			if err := os.WriteFile(regular, []byte("{}"), 0600); err != nil {
				panic(err)
			}
			if err := os.Rename(regular, path); err != nil {
				panic(err)
			}
			if swaps.Add(1) == 1 {
				close(started)
			}
		}
	}()
	<-started
	for i := 0; i < 10000; i++ {
		_, _ = readPrivateWitness(path)
	}
	close(stop)
	wg.Wait()
	if swaps.Load() < 2 {
		t.Fatal("fixture did not exercise repeated FIFO swaps")
	}
}
