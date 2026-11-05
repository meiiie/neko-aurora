package aurora

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/privatefs"
)

type protocolHost struct{ requests int }

func (h *protocolHost) Request(_ context.Context, _ string, _ json.RawMessage) (any, error) {
	h.requests++
	return nil, nil
}
func (*protocolHost) Event(string, json.RawMessage) error { return nil }

type shortCoreWriter struct{}

func (shortCoreWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }

func TestCoreAgentTransportBoundsAndCancellation(t *testing.T) {
	bun := findCoreBun()
	if bun == "" {
		t.Skip("Bun unavailable; subprocess lifecycle not exercised")
	}
	for _, mode := range []string{"cancel", "bad-sequence", "oversize", "missing-result"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			home, bin := filepath.Join(root, "home"), filepath.Join(root, "bin")
			for _, dir := range []string{home, bin} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := privatefs.ProtectDirectory(home); err != nil {
				t.Fatal(err)
			}
			script := `import {writeFileSync} from 'node:fs'; import {join} from 'node:path';
process.stdin.once('data',()=>{
 const mode=` + string(rawCore(t, mode)) + `;
 if(mode==='cancel'){writeFileSync(join(process.env.HOME,'started'),'yes');return}
 if(mode==='bad-sequence')process.stdout.write(JSON.stringify({type:'request',id:'r2',method:'inputs',params:{}})+'\n');
 if(mode==='oversize')process.stdout.write('x'.repeat(9*1024*1024)+'\n');
 if(mode==='missing-result')process.exit(0);
});
process.stdin.on('end',()=>{writeFileSync(join(process.env.HOME,'disposed'),'yes');process.exit(0)});`
			entry := filepath.Join(bin, "aurora-agent.ts")
			if err := os.WriteFile(entry, []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "bunfig.neko.toml"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			c := &Core{Bun: bun, Entry: entry, Home: home, Profile: "zai-openai", Model: "fixture", key: "fixture-only", Quick: true}
			c.validation = &coreValidation{identity: sha256.Sum256([]byte(c.Home + "\x00" + c.Profile + "\x00" + c.Model + "\x00" + c.key)), checked: time.Now()}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if mode == "cancel" {
				go func() {
					ticker := time.NewTicker(10 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							if _, err := os.Stat(filepath.Join(home, "started")); err == nil {
								cancel()
								return
							}
						}
					}
				}()
			}
			host := &protocolHost{}
			_, err := c.Agent(ctx, coreAgentStart{}, host)
			if err == nil || host.requests != 0 {
				t.Fatal("invalid child admitted", err, host.requests)
			}
			if mode == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation hidden", err)
				}
				if _, err := os.Stat(filepath.Join(home, "disposed")); err != nil {
					t.Fatal("child had no chance to dispose on EOF", err)
				}
			}
		})
	}
}

func TestCoreAgentFrameWriteAndEvidenceBounds(t *testing.T) {
	if err := writeCoreFrame(shortCoreWriter{}, map[string]string{"type": "start"}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	if err := writeCoreFrame(io.Discard, strings.Repeat("x", coreFrameLimit)); err == nil {
		t.Fatal("oversize parent frame accepted")
	}
	if validCoreCompletion(coreCompletion{Status: "ready", Revision: "r", Evidence: []string{strings.Repeat("x", 4001)}}) {
		t.Fatal("oversize evidence accepted")
	}
}
