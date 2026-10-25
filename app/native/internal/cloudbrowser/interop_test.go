package cloudbrowser

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixtureWireTransport struct{ target *url.URL }

func (x fixtureWireTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	endpoint := *r.URL
	endpoint.Scheme, endpoint.Host = x.target.Scheme, x.target.Host
	copy.URL, copy.Host = &endpoint, x.target.Host
	return http.DefaultTransport.RoundTrip(copy)
}

func TestCloudWorkerSignedWireAndSealedVaultInterop(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for worker wire integration")
	}
	script, err := filepath.Abs("../../../scripts/cloud-worker-fixture.mjs")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, node, script, t.TempDir())
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
		} else {
			ready <- ""
		}
	}()
	var address string
	select {
	case line := <-ready:
		address = strings.TrimPrefix(line, "READY ")
	case <-time.After(10 * time.Second):
		t.Fatal("worker fixture did not become ready")
	}
	target, err := url.Parse(address)
	if err != nil || target.Host == "" {
		t.Fatal("worker fixture failed to start: " + diagnostics.String())
	}
	client, err := New(Config{URL: "https://worker.aurora.invalid", Key: []byte("fixture-only-wire-key-7281-not-a-credential"), StateDir: t.TempDir(), HTTP: &http.Client{Transport: fixtureWireTransport{target}}})
	if err != nil {
		t.Fatal(err)
	}
	base := Command{Profile: "bot-wire-fixture", Actor: "owner", ActorID: "fixture-owner"}
	call := func(cmd Command) Response {
		t.Helper()
		result, err := client.Do(context.Background(), cmd)
		if err != nil {
			t.Fatalf("worker wire operation %s: %v", cmd.Kind, err)
		}
		return result
	}
	cmdOpen := base
	cmdOpen.Kind, cmdOpen.Nonce = "open", "wire-open-once"
	opened := call(cmdOpen)
	if opened.Status != "ready" || opened.Control != "paused" {
		t.Fatalf("invalid opening state: %+v", opened)
	}
	control := base
	control.Kind, control.Mode, control.Epoch, control.Nonce = "control", "owner", opened.Epoch, "wire-control-once"
	controlled := call(control)
	observe := base
	observe.Kind = "observe"
	view := call(observe)
	fill := base
	fill.Kind, fill.Epoch, fill.ObservationID = "vault_fill", controlled.Epoch, view.ObservationID
	fill.ExpectedOrigin, fill.Nonce = "https://chat.zalo.me", "wire-vault-once"
	fill.Sealed, err = client.Seal(fill, Credentials{Username: "fixture-user-7281", Password: "fixture-password-7281"})
	if err != nil {
		t.Fatal(err)
	}
	first := call(fill)
	second := call(fill)
	if first.Receipt == nil || second.Receipt == nil || first.Receipt.Status != "dispatched" || second.Receipt.Status != "dispatched" {
		t.Fatal("worker failed to acknowledge the accepted sealed operation")
	}
	view = call(observe)
	if !strings.Contains(view.Text, "Fixture effects: 1") || strings.Contains(view.Text, "fixture-password-7281") {
		t.Fatalf("wrong effect count or secret exposure: %s", view.Text)
	}
}
