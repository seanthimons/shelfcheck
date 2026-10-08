package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	bc "shelfcheck/internal/bibliocommons"
)

// configDir holds credentials.env and session.json; both are private.
func configDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "shelfcheck")
}

type sessionFiles struct {
	credentials string
	session     string
}

func (f *sessionFiles) register(fs *flag.FlagSet) {
	fs.StringVar(&f.credentials, "credentials", filepath.Join(configDir(), "credentials.env"), "file with SHELFCHECK_CARD and SHELFCHECK_PIN (environment variables override it)")
	fs.StringVar(&f.session, "session", filepath.Join(configDir(), "session.json"), "where the logged-in session is saved")
}

// credentials reads the card number and PIN from the environment or the
// credentials file. It refuses a file other users can read.
func (f sessionFiles) credentialsFromEnv() (card, pin string, err error) {
	card, pin = os.Getenv("SHELFCHECK_CARD"), os.Getenv("SHELFCHECK_PIN")
	if card != "" && pin != "" {
		return card, pin, nil
	}
	fi, err := os.Stat(f.credentials)
	if err != nil {
		return "", "", fmt.Errorf("no credentials: set SHELFCHECK_CARD and SHELFCHECK_PIN in %s", f.credentials)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return "", "", fmt.Errorf("%s is readable by other users; run: chmod 600 %s", f.credentials, f.credentials)
	}
	file, err := os.Open(f.credentials)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(strings.TrimPrefix(k, "export ")) {
		case "SHELFCHECK_CARD":
			if card == "" {
				card = v
			}
		case "SHELFCHECK_PIN":
			if pin == "" {
				pin = v
			}
		}
	}
	if card == "" || pin == "" {
		return "", "", fmt.Errorf("fill in SHELFCHECK_CARD and SHELFCHECK_PIN in %s", f.credentials)
	}
	return card, pin, sc.Err()
}

func (f sessionFiles) login(ctx context.Context, client *bc.Client) (*bc.Session, error) {
	card, pin, err := f.credentialsFromEnv()
	if err != nil {
		return nil, err
	}
	s, err := client.Login(ctx, card, pin)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(f.session), 0o700); err != nil {
		return nil, err
	}
	return s, bc.SaveSession(f.session, s)
}

// withSession runs fn with the saved session. If there's no session, or it
// has expired, it logs in once with the saved credentials and retries.
func (f sessionFiles) withSession(ctx context.Context, client *bc.Client, fn func(*bc.Session) error) error {
	s, err := bc.LoadSession(f.session)
	if err == nil && s.Library == client.Library {
		err = fn(s)
		if !errors.Is(err, bc.ErrSessionExpired) {
			return err
		}
		fmt.Fprintln(os.Stderr, "shelfcheck: session expired, logging in again")
	}
	s, err = f.login(ctx, client)
	if err != nil {
		return fmt.Errorf("logging in: %w", err)
	}
	return fn(s)
}

// loginCmd logs in and saves the session, or with -check verifies it.
func loginCmd(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	var o options
	o.registerCatalog(fs)
	var files sessionFiles
	files.register(fs)
	check := fs.Bool("check", false, "only check whether the saved session still works")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client := bc.New(o.library)

	var s *bc.Session
	var err error
	if *check {
		if s, err = bc.LoadSession(files.session); err != nil {
			return err
		}
		if err = client.Check(ctx, s); err != nil {
			return err
		}
		fmt.Printf("session OK (created %s)\n", s.Created.Format("2006-01-02 15:04"))
	} else {
		if s, err = files.login(ctx, client); err != nil {
			return err
		}
		fmt.Printf("logged in; session saved to %s\n", files.session)
	}
	fmt.Printf("user %s, account %s, home branch %s\n", s.UserID, s.AccountID, s.HomeBranch)
	return nil
}
