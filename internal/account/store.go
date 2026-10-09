// Package account manages a patron's private files (library card
// credentials, the logged-in catalog session, the Libby link) and the
// account features built on them: loans, holds, history and the Libby
// wishlist. Both the CLI and the TUI use it.
package account

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
	"shelfcheck/internal/libby"
)

// Store locates the private files. The zero value isn't usable; use
// DefaultStore or set every path.
type Store struct {
	Credentials string // SHELFCHECK_CARD / SHELFCHECK_PIN env file
	Session     string // saved catalog session
	Libby       string // saved Libby link

	mu   sync.Mutex // serializes logins between concurrent callers
	hcMu sync.Mutex
	hc   *hardcover.Client
}

// DefaultDir is ~/.config/shelfcheck (or the platform equivalent).
func DefaultDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "shelfcheck")
}

// DefaultStore uses the standard file names in dir.
func DefaultStore(dir string) *Store {
	return &Store{
		Credentials: filepath.Join(dir, "credentials.env"),
		Session:     filepath.Join(dir, "session.json"),
		Libby:       filepath.Join(dir, "libby.json"),
	}
}

// HasCatalog reports whether catalog account access is set up (a saved
// session or credentials to log in with).
func (s *Store) HasCatalog() bool {
	if os.Getenv("SHELFCHECK_CARD") != "" {
		return true
	}
	for _, p := range []string{s.Session, s.Credentials} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// HasLibby reports whether a Libby link is saved.
func (s *Store) HasLibby() bool {
	_, err := os.Stat(s.Libby)
	return err == nil
}

// HasHardcover reports whether a Hardcover token is set.
func (s *Store) HasHardcover() bool { return s.HardcoverClient() != nil }

// HardcoverClient returns a Hardcover API client using HARDCOVER_TOKEN from
// the environment or the credentials file, or nil if there's no token.
func (s *Store) HardcoverClient() *hardcover.Client {
	tok := os.Getenv("HARDCOVER_TOKEN")
	if tok == "" {
		vals, _ := s.envFile()
		tok = vals["HARDCOVER_TOKEN"]
	}
	if tok == "" {
		return nil
	}
	s.hcMu.Lock()
	defer s.hcMu.Unlock()
	c := hardcover.New(tok)
	if s.hc != nil && s.hc.Token == c.Token {
		return s.hc
	}
	if dir, err := os.UserCacheDir(); err == nil {
		c.CacheDir = filepath.Join(dir, "shelfcheck", "hardcover")
	}
	s.hc = c
	return c
}

// envFile reads KEY=value lines from the credentials file, refusing a file
// other users can read.
func (s *Store) envFile() (map[string]string, error) {
	fi, err := os.Stat(s.Credentials)
	if err != nil {
		return nil, err
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is readable by other users; run: chmod 600 %s", s.Credentials, s.Credentials)
	}
	f, err := os.Open(s.Credentials)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	vals := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vals[strings.TrimSpace(strings.TrimPrefix(k, "export "))] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return vals, sc.Err()
}

// CredentialsFromEnv reads the card number and PIN from the environment or
// the credentials file. It refuses a file other users can read.
func (s *Store) CredentialsFromEnv() (card, pin string, err error) {
	card, pin = os.Getenv("SHELFCHECK_CARD"), os.Getenv("SHELFCHECK_PIN")
	if card != "" && pin != "" {
		return card, pin, nil
	}
	vals, err := s.envFile()
	if os.IsNotExist(err) {
		return "", "", fmt.Errorf("no credentials: set SHELFCHECK_CARD and SHELFCHECK_PIN in %s", s.Credentials)
	}
	if err != nil {
		return "", "", err
	}
	if card == "" {
		card = vals["SHELFCHECK_CARD"]
	}
	if pin == "" {
		pin = vals["SHELFCHECK_PIN"]
	}
	if card == "" || pin == "" {
		return "", "", fmt.Errorf("fill in SHELFCHECK_CARD and SHELFCHECK_PIN in %s", s.Credentials)
	}
	return card, pin, nil
}

// Login signs in with the saved credentials and saves the session.
func (s *Store) Login(ctx context.Context, client *bc.Client) (*bc.Session, error) {
	card, pin, err := s.CredentialsFromEnv()
	if err != nil {
		return nil, err
	}
	sess, err := client.Login(ctx, card, pin)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(s.Session), 0o700); err != nil {
		return nil, err
	}
	return sess, bc.SaveSession(s.Session, sess)
}

// WithSession runs fn with the saved session. If there's no session, or it
// has expired, it logs in once with the saved credentials and retries.
func (s *Store) WithSession(ctx context.Context, client *bc.Client, fn func(*bc.Session) error) error {
	sess, err := bc.LoadSession(s.Session)
	if err == nil && sess.Library == client.Library {
		err = fn(sess)
		if !errors.Is(err, bc.ErrSessionExpired) {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Another caller may have logged in while we waited.
	if fresh, err := bc.LoadSession(s.Session); err == nil && sess != nil && fresh.Created.After(sess.Created) {
		if err := fn(fresh); !errors.Is(err, bc.ErrSessionExpired) {
			return err
		}
	}
	sess, err = s.Login(ctx, client)
	if err != nil {
		return fmt.Errorf("logging in: %w", err)
	}
	return fn(sess)
}

// LibbyClient returns a client for the saved Libby link.
func (s *Store) LibbyClient() (*libby.Client, error) {
	saved, err := libby.Load(s.Libby)
	if err != nil {
		return nil, err
	}
	return libby.New(saved.Identity), nil
}
