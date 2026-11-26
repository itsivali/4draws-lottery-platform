//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/config"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/crypto"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/db"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/testpg"
	"github.com/itsivali/4draws-lottery-platform/internal/staff"
)

func TestRun_PaymentsReconcilePrintsACleanReportOnAFreshDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", testpg.Migrated(t))
	for _, k := range []string{"AUTH_KEY", "TOKEN_SEED", "DATA_KEY", "DATA_KEYS", "IDENTITY_KEY", "VALKEY_ADDR", "DARAJA_CONSUMER_KEY"} {
		t.Setenv(k, "")
	}
	var out bytes.Buffer
	if err := run(context.Background(), command{name: "payments reconcile"}, &out); err != nil {
		t.Fatalf("reconcile: %v\n%s", err, out.String())
	}
	var r struct {
		OK         bool                `json:"ok"`
		Mismatches map[string][]string `json:"mismatches"`
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || !r.OK || len(r.Mismatches) != 0 {
		t.Fatalf("report = %s (%v)", out.String(), err)
	}
}

// ADR 0032: an operator action signs the staff member in with their back-office password; the action is audited under
// their staff id, never under a name they typed.
func TestRun_OperatorActionsSignTheStaffMemberIn(t *testing.T) {
	t.Setenv("DATABASE_URL", testpg.Migrated(t))
	t.Setenv("AUTH_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("TOKEN_SEED", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("DATA_KEYS", "1:MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("IDENTITY_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("VALKEY_ADDR", "valkey:6379")
	t.Setenv("TRUSTED_PROXY_HEADER", "none")
	for _, k := range []string{"DATA_KEY", "DARAJA_CONSUMER_KEY", "STAFF_BOOTSTRAP_ADMIN", "STAFF_BOOTSTRAP_PASSWORD"} {
		t.Setenv(k, "")
	}
	ctx := context.Background()
	var out bytes.Buffer
	if err := run(ctx, command{name: "staff password", email: opID, password: "a long temporary phrase", admin: true}, &out); err != nil {
		t.Fatal(err)
	}
	typed := "a long temporary phrase"
	readPassword = func(string) (string, error) { return typed, nil }
	t.Cleanup(func() { readPassword = func(string) (string, error) { return "", nil } })
	reverse := command{name: "payments reverse", receipt: "NOPE", reason: "x", operator: opID}
	if err := run(ctx, reverse, &out); err == nil || !strings.Contains(err.Error(), "temporary") {
		t.Fatalf("with a temporary password: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	svc := staff.NewService(pool, clock.Postgres{}, nil, audit.NewWriter(), crypto.MAC(cfg.API.AuthKey, "staff-password-pepper"))
	sess, err := svc.SignInWithPassword(ctx, opID, typed)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangePassword(ctx, sess.Member, sess.Token, typed, "my own chosen passphrase"); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, reverse, &out); err == nil || !strings.Contains(err.Error(), "sign-in refused") {
		t.Fatalf("with a wrong password: %v", err)
	}
	typed = "my own chosen passphrase"
	err = run(ctx, reverse, &out)
	if err == nil || strings.Contains(err.Error(), "sign-in") {
		t.Fatalf("signed in, reversing an unknown receipt: %v", err)
	}
	var actor string
	if err := pool.QueryRow(ctx, `SELECT actor_id FROM audit.audit_log WHERE action = 'staff.sign_in' ORDER BY id DESC LIMIT 1`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if actor != sess.Member.Operator() {
		t.Fatalf("signed in as %q, want %q", actor, sess.Member.Operator())
	}
}

func TestRun_GameReconcilePrintsACleanReportOnAFreshDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", testpg.Migrated(t))
	for _, k := range []string{"AUTH_KEY", "TOKEN_SEED", "DATA_KEY", "DATA_KEYS", "IDENTITY_KEY", "VALKEY_ADDR", "DARAJA_CONSUMER_KEY"} {
		t.Setenv(k, "")
	}
	var out bytes.Buffer
	if err := run(context.Background(), command{name: "game reconcile"}, &out); err != nil {
		t.Fatalf("reconcile: %v\n%s", err, out.String())
	}
	var r struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || !r.OK {
		t.Fatalf("report = %s (%v)", out.String(), err)
	}
}

func TestRun_StaffPasswordLetsALockedOutOwnerBackIn(t *testing.T) {
	t.Setenv("DATABASE_URL", testpg.Migrated(t))
	t.Setenv("AUTH_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("TOKEN_SEED", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("DATA_KEYS", "1:MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("IDENTITY_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("VALKEY_ADDR", "valkey:6379")
	t.Setenv("TRUSTED_PROXY_HEADER", "none")
	for _, k := range []string{"DATA_KEY", "DARAJA_CONSUMER_KEY", "STAFF_BOOTSTRAP_ADMIN", "STAFF_BOOTSTRAP_PASSWORD"} {
		t.Setenv(k, "")
	}
	var out bytes.Buffer
	// nobody exists yet: without -admin there is nobody to give a password to
	if err := run(context.Background(), command{name: "staff password", email: "owner@example.com", password: "a long temporary phrase"}, &out); err == nil {
		t.Fatal("an unknown member got a password")
	}
	if err := run(context.Background(), command{name: "staff password", email: "owner@example.com", password: "short", admin: true}, &out); err == nil {
		t.Fatal("a short password was accepted")
	}
	out.Reset()
	if err := run(context.Background(), command{name: "staff password", email: "Owner@Example.com", password: "a long temporary phrase", admin: true}, &out); err != nil {
		t.Fatalf("staff password: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("owner@example.com has a temporary password")) || !bytes.Contains(out.Bytes(), []byte("[admin]")) {
		t.Fatalf("output: %s", out.String())
	}
	// the password is read from stdin when not given, and the member is only updated the second time
	out.Reset()
	cmd := command{name: "staff password", email: "owner@example.com"}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := staffPassword(context.Background(), cfg, cmd, strings.NewReader("typed on the terminal\n"), &out); err != nil {
		t.Fatalf("stdin password: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Temporary password")) || !bytes.Contains(out.Bytes(), []byte("has a temporary password")) {
		t.Fatalf("output: %s", out.String())
	}
}
