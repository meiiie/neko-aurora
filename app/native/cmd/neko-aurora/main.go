package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"nekoaurora.local/native/internal/aurora"
	"nekoaurora.local/native/internal/auroraback"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sync/atomic"
	"syscall"
	"time"

	"nekoaurora.local/native/internal/api"
	"nekoaurora.local/native/internal/connections"
	"nekoaurora.local/native/internal/documentworker"
	"nekoaurora.local/native/internal/hub"
	"nekoaurora.local/native/internal/identity"
	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/runtime"
	"nekoaurora.local/native/internal/statebackup"
)

func main() {
	for _, argument := range os.Args[1:] {
		if argument == "--document-worker-v1" {
			if len(os.Args) != 2 {
				os.Exit(2)
			}
			os.Exit(documentworker.Run(os.Stdin, os.Stdout))
		}
	}

	state := flag.String("state", ".state", "Local state directory")
	listen := flag.String("listen", "127.0.0.1:8765", "Loopback address")
	challengeSimulator := flag.Bool("local-secure-challenge-simulator", false, "Enable typed local challenge simulation; never grants execution or consumes provider credentials")
	fixtureConfig := flag.String("core-fixture-config", "", "Explicit controlled Core fixture configuration; no real provider")
	profileConfig := flag.String("core-profile-config", "", "Trusted compatible Core/source registry; profile inspection and execution require local owner consent")
	identityConfig := flag.String("fixture-identity-config", "", "Explicit ephemeral loopback fixture actors/devices; no production login or grants")
	sessionOutput := flag.String("fixture-session-output", "", "New private file for single-use fixture bootstrap tickets; values are never printed")
	hostExecutable := flag.String("host-executable", "", "Trusted local Rust host for secure input; defaults next to control executable")
	desktopReceipt := flag.String("desktop-launch-receipt", "", "New private startup receipt within this state; no profile or execution grant")
	backupCreate := flag.String("backup-create", "", "Offline encrypted snapshot into a new local directory")
	backupVerify := flag.String("backup-verify", "", "Verify a local encrypted snapshot without restoring")
	restoreBackup := flag.String("restore-dry-run", "", "Restore this snapshot into a new read-only local directory")
	restoreTarget := flag.String("restore-target", "", "Nonexistent isolated destination for restore-dry-run")
	auroraRestore := flag.String("aurora-restore", "", "Restore an Aurora snapshot (state/backups/aurora-*.tar.gz) into an empty -state directory, then exit; copy aurora/vault.key separately")
	flag.Parse()
	if *auroraRestore != "" {
		if err := auroraback.Restore(*auroraRestore, *state); err != nil {
			fmt.Fprintln(os.Stderr, "aurora_restore_failed:", err)
			os.Exit(1)
		}
		fmt.Println("aurora_restored")
		return
	}
	operations := 0
	for _, p := range []string{*backupCreate, *backupVerify, *restoreBackup} {
		if p != "" {
			operations++
		}
	}
	if operations > 0 || *restoreTarget != "" {
		if operations != 1 || *fixtureConfig != "" || *profileConfig != "" || *identityConfig != "" || *sessionOutput != "" || (*restoreBackup == "") != (*restoreTarget == "") {
			fmt.Fprintln(os.Stderr, "invalid_offline_backup_arguments")
			os.Exit(2)
		}
		var manifest statebackup.Manifest
		var e error
		operation := "verify"
		if *backupCreate != "" {
			operation = "create"
			revision := "working-tree"
			if info, ok := debug.ReadBuildInfo(); ok {
				modified := false
				for _, setting := range info.Settings {
					if setting.Key == "vcs.revision" {
						revision = setting.Value
					}
					if setting.Key == "vcs.modified" && setting.Value == "true" {
						modified = true
					}
				}
				if modified {
					revision = "working-tree"
				}
			}
			manifest, e = statebackup.Create(*state, *backupCreate, revision)
		}
		if *backupVerify != "" {
			manifest, e = statebackup.Verify(*backupVerify)
		}
		if *restoreBackup != "" {
			operation = "restore-dry-run"
			manifest, e = statebackup.RestoreDryRun(*restoreBackup, *restoreTarget)
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		var total int64
		for _, f := range manifest.Files {
			total += f.Bytes
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "PASS", "operation": operation, "version": manifest.Version, "snapshotId": manifest.SnapshotID, "files": len(manifest.Files), "bytes": total, "sequence": manifest.Sequence, "executionDisabled": true, "protection": manifest.Protection, "scope": "Local offline Windows-user-bound snapshot; restored state is read-only, no runtime or provider"})
		return
	}
	if os.Getenv("AURORA_MODE") == "hosted" {
		os.Exit(runHosted(*state, *listen))
	}
	// Container mode: AURORA_PUBLIC_HOST + a strong AURORA_OWNER_CODE allow binding
	// 0.0.0.0 (PORT from the platform); remote peers then see only /app/.
	publicReady := aurora.Env("PUBLIC_HOST") != "" && len(aurora.Env("OWNER_CODE")) >= 12
	if p := os.Getenv("PORT"); publicReady && p != "" && *listen == "127.0.0.1:8765" {
		*listen = "0.0.0.0:" + p
	}
	host, _, err := net.SplitHostPort(*listen)
	exposed := host == "0.0.0.0" && publicReady
	if err != nil || (host != "127.0.0.1" && !exposed) {
		fmt.Fprintln(os.Stderr, "Only 127.0.0.1 is permitted")
		os.Exit(2)
	}
	var principals *identity.Authority
	var principalConfig identity.Config
	if (*identityConfig == "") != (*sessionOutput == "") {
		fmt.Fprintln(os.Stderr, "fixture_identity_arguments_invalid")
		os.Exit(2)
	}
	if *identityConfig != "" {
		principals, principalConfig, err = loadFixtureIdentity(*identityConfig, *sessionOutput)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fixture_identity_config_invalid")
			os.Exit(2)
		}
	}
	if wiped, e := auroraback.WipeIfRequested(*state); e != nil {
		fmt.Fprintln(os.Stderr, "account_delete_incomplete; retrying on next start")
		os.Exit(1)
	} else if wiped {
		fmt.Println("aurora account deleted: state wiped")
	}
	documentClient, err := documentworker.NewSelf()
	if err != nil {
		fmt.Fprintln(os.Stderr, "document_worker_unavailable")
		os.Exit(1)
	}
	defer documentClient.Close()
	store, err := ledger.OpenWithDocumentWorker(*state, documentClient)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Local store unavailable; inspect state before retry")
		os.Exit(1)
	}
	defer store.Close()
	if principals != nil {
		if store.ReadOnly() {
			fmt.Fprintln(os.Stderr, "restored_state_read_only")
			os.Exit(2)
		}
		store.ConfigureIdentity(principals)
	}
	if store.ReadOnly() && (*fixtureConfig != "" || *profileConfig != "") {
		fmt.Fprintln(os.Stderr, "restored_state_read_only")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	var deleteAccount atomic.Bool
	defer stop()
	trustedHost := *hostExecutable
	if trustedHost == "" {
		executable, _ := os.Executable()
		trustedHost = filepath.Join(filepath.Dir(executable), "neko-host.exe")
	}
	var fixture *runtime.Config
	var profiles *runtime.CoreProfiles
	if *fixtureConfig != "" {
		c, e := runtime.Load(*fixtureConfig)
		if e != nil {
			fmt.Fprintln(os.Stderr, "Invalid Core fixture configuration")
			return
		}
		fixture = &c
		trustedHost = c.HostExecutable
	}
	if *profileConfig != "" {
		c, service, e := runtime.LoadCoreProfiles(*profileConfig, *state)
		if e != nil || fixture != nil && (fixture.HostExecutable != c.HostExecutable || fixture.CoreExecutable != c.CoreExecutable || fixture.Supervisor != c.Supervisor) {
			fmt.Fprintln(os.Stderr, "Invalid trusted Core profile configuration")
			return
		}
		profiles = service
		if fixture != nil {
			fixture.CoreProfiles = service
		} else {
			fixture = &c
		}
		trustedHost = c.HostExecutable
		store.ConfigureCoreProfiles(service)
	}
	connectionDir, e := filepath.Abs(filepath.Join(*state, "connections"))
	if e != nil {
		fmt.Fprintln(os.Stderr, "Connection state unavailable")
		return
	}
	var catalog *connections.Store
	if !store.ReadOnly() {
		catalog, e = connections.Open(connectionDir, connections.HostSealer{Executable: trustedHost, State: connectionDir})
		if e != nil {
			fmt.Fprintln(os.Stderr, "Connection metadata unavailable; retain existing state")
			return
		}
		store.EnableAccounts(catalog)
		if e = store.ReconcileAccounts(); e != nil {
			fmt.Fprintln(os.Stderr, "Account authority reconciliation unavailable")
			return
		}
	}
	store.ConfigureChallengeSimulator(*challengeSimulator)
	if fixture != nil {
		config := *fixture
		manager, e := runtime.New(store, config, *state)
		if e != nil {
			fmt.Fprintln(os.Stderr, "Core state requires recovery")
			os.Exit(1)
		}
		done := make(chan struct{})
		go func() { defer close(done); manager.Run(ctx) }()
		defer func() { stop(); <-done }()
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Loopback listener unavailable")
		return
	}
	handler, err := api.New(store, listener.Addr().String())
	if err != nil {
		listener.Close()
		return
	}
	if catalog != nil {
		handler.EnableConnections(catalog)
	}
	if *desktopReceipt != "" {
		if err = handler.EnableDesktopLaunch(*state, *desktopReceipt); err != nil {
			listener.Close()
			fmt.Fprintln(os.Stderr, "desktop_launch_unavailable")
			return
		}
	}
	if auroraService, e := aurora.New(ctx, store, filepath.Join(*state, "aurora"), aurora.Options{RunTurns: fixture == nil}); e != nil {
		fmt.Fprintln(os.Stderr, "aurora_unavailable")
	} else {
		auroraService.OnDeleteAccount = func() error {
			if e := auroraback.RequestWipe(*state); e != nil {
				return e
			}
			deleteAccount.Store(true)
			// Let the 202 reach the client, then shut down; the wipe runs
			// after the ledger is closed and the binary re-executes itself.
			time.AfterFunc(300*time.Millisecond, stop)
			return nil
		}
		handler.EnableAurora(auroraService)
		if exposed {
			handler.SetExposed()
		}
		// Optional HTTPS exposure of /app/ only (e.g. a tunnel). The owner
		// passcode comes from the environment and is never logged.
		if host, code := aurora.Env("PUBLIC_HOST"), aurora.Env("OWNER_CODE"); host != "" {
			if len(code) < 12 {
				fmt.Fprintln(os.Stderr, "aurora_public_requires_owner_code")
			} else {
				handler.EnableAuroraPublic(host, code)
			}
		}
	}
	if profiles != nil {
		handler.EnableCoreProfiles(profiles)
	}
	if principals != nil {
		handler.EnableFixtureIdentity(principals)
		if err = writeFixtureTickets(*sessionOutput, principals, principalConfig); err != nil {
			listener.Close()
			fmt.Fprintln(os.Stderr, "fixture_ticket_output_unavailable")
			return
		}
	}
	if every, err := time.ParseDuration(envOr("AURORA_BACKUP_EVERY", "6h")); err == nil && every >= time.Minute {
		go auroraback.Loop(ctx, *state, every, 14)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8192}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Printf("READY http://%s\n", listener.Addr().String())
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "HTTP service stopped")
	}
	if deleteAccount.Load() {
		store.Close()
		documentClient.Close()
		restartFresh()
	}
}

// restartFresh re-executes this binary with the same arguments; startup sees
// the wipe marker, erases state and serves an empty account.
func restartFresh() {
	exe, err := os.Executable()
	if err == nil {
		err = syscall.Exec(exe, os.Args, os.Environ())
	}
	fmt.Fprintln(os.Stderr, "account_delete_restart_failed; state wipes on next start")
	os.Exit(1)
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// runHosted serves the multi-user hub (AURORA_MODE=hosted): sign-up, one
// isolated workspace per account, plan quotas on the server's z.ai key.
func runHosted(state, listen string) int {
	cfg, err := hub.ConfigFromEnv(state)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hosted_mode_config:", err)
		return 2
	}
	if p := os.Getenv("PORT"); p != "" && listen == "127.0.0.1:8765" {
		listen = "0.0.0.0:" + p
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	h, err := hub.New(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hosted_mode_unavailable")
		return 1
	}
	defer h.Close()
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listener unavailable")
		return 1
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	fmt.Printf("READY hosted http://%s\n", ln.Addr().String())
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return 1
	}
	return 0
}
