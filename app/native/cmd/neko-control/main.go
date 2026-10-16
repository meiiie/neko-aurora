package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"nekobot.local/native/internal/api"
	"nekobot.local/native/internal/connections"
	"nekobot.local/native/internal/ledger"
	"nekobot.local/native/internal/runtime"
)

func main() {
	state := flag.String("state", ".state", "Local state directory")
	listen := flag.String("listen", "127.0.0.1:8765", "Loopback address")
	fixtureConfig := flag.String("core-fixture-config", "", "Explicit controlled Core fixture configuration; no real provider")
	hostExecutable := flag.String("host-executable", "", "Trusted local Rust host for secure input; defaults next to control executable")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || host != "127.0.0.1" {
		fmt.Fprintln(os.Stderr, "Only 127.0.0.1 is permitted")
		os.Exit(2)
	}
	store, err := ledger.Open(*state)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Local store unavailable; inspect state before retry")
		os.Exit(1)
	}
	defer store.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	trustedHost := *hostExecutable
	if trustedHost == "" {
		executable, _ := os.Executable()
		trustedHost = filepath.Join(filepath.Dir(executable), "neko-host.exe")
	}
	if *fixtureConfig != "" {
		config, e := runtime.Load(*fixtureConfig)
		if e != nil {
			fmt.Fprintln(os.Stderr, "Invalid Core fixture configuration")
			os.Exit(1)
		}
		manager, e := runtime.New(store, config, *state)
		trustedHost = config.HostExecutable
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
	connectionDir, e := filepath.Abs(filepath.Join(*state, "connections"))
	if e != nil {
		fmt.Fprintln(os.Stderr, "Connection state unavailable")
		return
	}
	catalog, e := connections.Open(connectionDir, connections.HostSealer{Executable: trustedHost, State: connectionDir})
	if e != nil {
		fmt.Fprintln(os.Stderr, "Connection metadata unavailable; retain existing state")
		return
	}
	handler.EnableConnections(catalog)
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
}
