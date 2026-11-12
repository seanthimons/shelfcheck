package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
)

// configDir holds credentials.env, session.json and libby.json; all private.
func configDir() string { return account.DefaultDir() }

func libbyPath() string { return filepath.Join(configDir(), "libby.json") }

// registerStore adds -credentials and -session flags and returns the store
// they configure.
func registerStore(fs *flag.FlagSet) *account.Store {
	st := account.DefaultStore(configDir())
	fs.StringVar(&st.Credentials, "credentials", st.Credentials, "file with SHELFCHECK_CARD and SHELFCHECK_PIN (environment variables override it)")
	fs.StringVar(&st.Session, "session", st.Session, "where the logged-in session is saved")
	return st
}

// loginCmd logs in and saves the session, or with -check verifies it.
func loginCmd(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	var o options
	if err := o.registerCatalog(fs); err != nil {
		return err
	}
	st := registerStore(fs)
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
		if s, err = bc.LoadSession(st.Session); err != nil {
			return err
		}
		if err = client.Check(ctx, s); err != nil {
			return err
		}
		fmt.Printf("session OK (created %s)\n", s.Created.Format("2006-01-02 15:04"))
	} else {
		if s, err = st.Login(ctx, client); err != nil {
			return err
		}
		fmt.Printf("logged in; session saved to %s\n", st.Session)
	}
	fmt.Printf("user %s, account %s, home branch %s\n", s.UserID, s.AccountID, s.HomeBranch)
	return nil
}
