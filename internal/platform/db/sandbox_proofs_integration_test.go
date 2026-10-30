//go:build integration

package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/db/migrations"
	"github.com/itsivali/lottery-platform/internal/platform/db"
	"github.com/itsivali/lottery-platform/internal/platform/testpg"
)

// Migration 00040 proves the numbers that sandbox-settled top-ups credited without proving, and only those.
func TestMigration00040_ProvesNumbersFromSandboxTopUpsOnly(t *testing.T) {
	dsn, _ := testpg.Start(t)
	ctx := context.Background()
	if err := db.MigrateUp(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	user := func(msisdn string, proved bool) string {
		t.Helper()
		var id string
		if err := conn.QueryRow(ctx, `INSERT INTO identity.users (msisdn, kyc_status, password_hash, msisdn_verified_at)
			VALUES ($1, 'none', 'x', CASE WHEN $2 THEN now() END) RETURNING id::text`, msisdn, proved).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	topUp := func(userID, msisdn, receipt string) {
		t.Helper()
		if _, err := conn.Exec(ctx, `INSERT INTO payments.payments (user_id, channel, amount_requested_kes, msisdn, idempotency_key,
			status, amount_confirmed_kes, mpesa_receipt) VALUES ($1, 'stk', 1, $2, $3, 'credited', 1, $3)`, userID, msisdn, receipt); err != nil {
			t.Fatal(err)
		}
	}
	sandboxed := user("254705369005", false) // the owner's case: credited at 11:55 EAT, number not proved
	topUp(sandboxed, "254705369005", "SANDBOX-a")
	topUp(sandboxed, "254705369005", "SANDBOX-b")
	real := user("254700000001", false) // a real receipt is not this migration's business
	topUp(real, "254700000001", "QJK1234567")
	owner := user("254700000002", true) // another account already proved the number
	other := user("254700000002", false)
	topUp(other, "254700000002", "SANDBOX-c")
	moved := user("254700000003", false) // the account's number changed since the top-up
	topUp(moved, "254700000004", "SANDBOX-d")

	raw, err := migrations.FS.ReadFile("00040_sandbox_topups_prove_phones.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, _, _ := strings.Cut(string(raw), "-- +goose Down")
	for range 2 { // and running it again changes nothing
		if _, err := conn.Exec(ctx, up); err != nil {
			t.Fatal(err)
		}
	}
	for id, want := range map[string]bool{sandboxed: true, real: false, owner: true, other: false, moved: false} {
		var proved bool
		var audits int
		if err := conn.QueryRow(ctx, `SELECT msisdn_verified_at IS NOT NULL,
			(SELECT count(*) FROM audit.audit_log WHERE entity_id = $1::text AND action = 'user.phone_verified')
			FROM identity.users WHERE id = $1::uuid`, id).Scan(&proved, &audits); err != nil {
			t.Fatal(err)
		}
		if proved != want {
			t.Errorf("user %s proved = %v, want %v", id, proved, want)
		}
		if wantAudits := map[bool]int{true: 1, false: 0}[want && id != owner]; audits != wantAudits {
			t.Errorf("user %s: %d audit records, want %d", id, audits, wantAudits)
		}
	}
}
