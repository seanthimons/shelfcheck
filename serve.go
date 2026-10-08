package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	bm "charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"

	"shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/tui"
)

// serve runs the TUI over SSH. Only keys listed in -authorized-keys can
// connect; any SSH username works.
//
//	ssh -p 2222 host              open the search screen
//	ssh -p 2222 -t host dune      search right away (-t is needed with a command)
func serve(args []string) error {
	home, _ := os.UserHomeDir()
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var o options
	o.register(fs)
	addr := fs.String("addr", ":2222", "address to listen on")
	hostKey := fs.String("host-key", ".ssh/shelfcheck_ed25519", "server host key; created on first run")
	authKeys := fs.String("authorized-keys", filepath.Join(home, ".ssh", "authorized_keys"), "public keys allowed to connect, in authorized_keys format")
	idle := fs.Duration("idle-timeout", time.Hour, "disconnect sessions idle this long")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkAuthorizedKeys(*authKeys); err != nil {
		return err
	}

	// One client for every session, so they share its cache and throttle.
	client := bibliocommons.New(o.library)

	handler := func(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
		return tui.New(client, o.home, !o.anywhere, strings.Join(sess.Command(), " ")), nil
	}

	srv, err := wish.NewServer(
		wish.WithAddress(*addr),
		wish.WithHostKeyPath(*hostKey),
		wish.WithAuthorizedKeys(*authKeys),
		wish.WithIdleTimeout(*idle),
		wish.WithMiddleware(
			bm.Middleware(handler),
			activeterm.Middleware(), // refuse sessions without a terminal
			logging.Middleware(),
		),
	)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		log.Info("shelfcheck listening", "addr", *addr, "library", o.library, "authorized_keys", *authKeys)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, ssh.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		return err
	}
	return nil
}

// checkAuthorizedKeys refuses to start without at least one key, so the
// server can never come up open to everyone.
func checkAuthorizedKeys(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading authorized keys: %w", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line)); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no usable public keys in %s", path)
}
