package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/config"
	"github.com/itsivali/lottery-platform/internal/platform/crypto"
	"github.com/itsivali/lottery-platform/internal/platform/db"
	"github.com/itsivali/lottery-platform/internal/staff"
)

// readPassword reads the staff member's password: hidden when typed at a terminal, else the first line of stdin (a
// password manager's pipe). Tests replace it.
var readPassword = func(prompt string) (string, error) {
	in := os.Stdin
	_, _ = fmt.Fprint(os.Stderr, prompt)
	if fi, err := in.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		off := exec.Command("stty", "-echo")
		off.Stdin = in
		if off.Run() == nil {
			defer func() {
				on := exec.Command("stty", "echo")
				on.Stdin = in
				_ = on.Run()
				_, _ = fmt.Fprintln(os.Stderr)
			}()
		}
	}
	return readLine(in)
}

func readLine(in io.Reader) (string, error) {
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("reading the password: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// signIn proves who is acting (ADR 0032): the staff member signs in with their back-office email address and password,
// as on the sign-in page, and the action is recorded under their staff id. A typed id proves nothing, so a two-person
// rule could otherwise be met by one person naming a colleague. It returns the operator ("staff:<id>") and a function
// that ends the session.
func signIn(ctx context.Context, cfg config.Config, email string) (string, func(), error) {
	if !cfg.API.Enabled || len(cfg.API.AuthKey) == 0 {
		return "", nil, errors.New("operator actions sign you in as staff, with the api's AUTH_KEY: set the api's configuration")
	}
	pw, err := readPassword("Back-office password for " + email + ": ")
	if err != nil {
		return "", nil, err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return "", nil, err
	}
	svc := staff.NewService(pool, clock.Postgres{}, nil, audit.NewWriter(), crypto.MAC(cfg.API.AuthKey, "staff-password-pepper"))
	sess, err := svc.SignInWithPassword(ctx, email, pw)
	if err != nil {
		pool.Close()
		if errors.Is(err, staff.ErrUnauthenticated) {
			return "", nil, errors.New("sign-in refused: check the email address and password (members who sign in with Google or Apple only must set a password first)")
		}
		return "", nil, err
	}
	done := func() {
		_ = svc.SignOut(context.WithoutCancel(ctx), sess.Token)
		pool.Close()
	}
	if sess.Member.MustChangePassword {
		done()
		return "", nil, errors.New("your password is temporary: sign in to the back office and choose your own first")
	}
	if !sess.Member.Has(staff.Staff) {
		done()
		return "", nil, errors.New("your account has no staff role")
	}
	return sess.Member.Operator(), done, nil
}
