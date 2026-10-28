//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/itsivali/lottery-platform/internal/platform/config"
	"github.com/itsivali/lottery-platform/internal/platform/testpg"
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
	if err := run(context.Background(), command{name: "payments reverse", receipt: "NOPE", reason: "x", operator: "staff:" + opID}, &out); err == nil {
		t.Fatal("reversing an unknown receipt succeeded")
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
