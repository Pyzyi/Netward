// Command netzwart is a self-hosted network monitor: it discovers every
// device on the local network, tracks presence and reports what changes.
package main

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // the scratch image has no zoneinfo

	"github.com/Pyzyi/Netward/internal/app"
	"github.com/Pyzyi/Netward/internal/auth"
	"github.com/Pyzyi/Netward/internal/config"
	"github.com/Pyzyi/Netward/internal/privdrop"
	"github.com/Pyzyi/Netward/internal/store"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

//go:embed all:web
var webFS embed.FS

func newLogger(level string) *slog.Logger {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	env, err := config.LoadEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	}
	log := newLogger(env.LogLevel)
	switch cmd {
	case "serve":
		os.Exit(serve(env, log))
	case "healthcheck":
		os.Exit(healthcheck(env))
	case "reset-password":
		os.Exit(resetPassword(env))
	case "factory-reset":
		os.Exit(factoryReset(env, os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println("netzwart", version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`Usage: netzwart [command]

Commands:
  serve            run Netzwart (default)
  healthcheck      exit 0 if the web server answers (used by Docker)
  reset-password   set a new random password for the admin account
  factory-reset    remove accounts, notification channels and settings (--yes)
  version          print the version
`)
}

func serve(env config.Env, log *slog.Logger) int {
	priv, err := privdrop.Prepare(env, log)
	if err != nil {
		log.Error("cannot drop privileges", "err", err)
		return 1
	}
	static, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Error("embedded web UI missing", "err", err)
		return 1
	}
	if _, err := fs.Stat(static, "index.html"); err != nil {
		log.Warn("web UI not built into this binary; only the API is available")
		static = nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, app.Options{Env: env, Priv: priv, Version: version, Static: static, Log: log}); err != nil {
		log.Error("Netzwart stopped with an error", "err", err)
		return 1
	}
	log.Info("Netzwart stopped")
	return 0
}

func healthcheck(env config.Env) int {
	// The check needs no privileges and never touches the data directory.
	_ = privdrop.DropForCommand(env)
	host := env.Listen
	switch host {
	case "0.0.0.0", "":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	scheme := "http"
	client := &http.Client{Timeout: 3 * time.Second}
	if env.TLSCert != "" {
		scheme = "https"
		// Liveness only: no credentials are sent, so the local certificate is not verified.
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec
	}
	resp, err := client.Get(scheme + "://" + net.JoinHostPort(host, strconv.Itoa(env.Port)) + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "unhealthy: status", resp.StatusCode)
		return 1
	}
	return 0
}

func openStoreAsUser(env config.Env) (*store.Store, error) {
	if err := privdrop.DropForCommand(env); err != nil {
		return nil, err
	}
	if _, err := os.Stat(app.DBPath(env)); err != nil {
		return nil, fmt.Errorf("no database at %s: %w", app.DBPath(env), err)
	}
	return store.Open(context.Background(), app.DBPath(env))
}

func resetPassword(env config.Env) int {
	st, err := openStoreAsUser(env)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer st.Close()
	ctx := context.Background()
	u, err := st.FirstUser(ctx)
	if errors.Is(err, store.ErrNotFound) {
		fmt.Println("No account exists yet. Open the web interface to create one.")
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	pw := app.RandomPassword()
	hash, err := auth.HashPassword(ctx, pw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if err := st.SetPassword(ctx, u.ID, hash, 0); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Printf("New password for %q: %s\nAll sessions were signed out. Please change the password after logging in.\n", u.Username, pw)
	return 0
}

func factoryReset(env config.Env, args []string) int {
	if len(args) == 0 || args[0] != "--yes" {
		fmt.Println("This removes the admin account, all sessions, notification channels and settings.")
		fmt.Println("Devices and history are kept. Run again with --yes to confirm, then restart the container.")
		return 2
	}
	// The running server keeps channels and settings in memory, so the reset
	// is done by the next start: the same trigger file as without a terminal.
	if err := privdrop.DropForCommand(env); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(env.DataDir, "factory-reset"), nil, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Println("Factory reset scheduled. Restart the container now: the reset happens at start, then open the web interface to set up again.")
	return 0
}
