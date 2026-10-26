//go:build integration

package identity_test

import (
	"context"
	"testing"
	"time"
)

func TestClosureRequest_KeepsTheFirstRequestAndClosesAtMostOnce(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tok := e.login(t, e.newMSISDN(), phone)
	p, err := e.svc.Authenticate(ctx, tok.Access)
	if err != nil {
		t.Fatal(err)
	}
	first, err := e.svc.RequestClosure(ctx, p.UserID)
	if err != nil || !first.Equal(start) {
		t.Fatalf("first request at %v (%v), want %v", first, err, start)
	}
	e.clk.Advance(time.Hour)
	if again, err := e.svc.RequestClosure(ctx, p.UserID); err != nil || !again.Equal(first) {
		t.Fatalf("second request at %v (%v), want the first kept", again, err)
	}
	a, err := e.svc.Account(ctx, p.UserID)
	if err != nil || a.ClosureRequestedAt == nil || !a.ClosureRequestedAt.Equal(first) || a.Status != "active" {
		t.Fatalf("account %+v %v: the request alone must not close it", a, err)
	}

	staff := "0192f000-0000-7000-8000-0000000000aa"
	for name, sql := range map[string]string{
		"change the request time": `UPDATE identity.closure_requests SET requested_at = requested_at - interval '1 day' WHERE user_id = $1`,
		"delete the request":      `DELETE FROM identity.closure_requests WHERE user_id = $1`,
		"close without staff":     `UPDATE identity.closure_requests SET closed_at = now() WHERE user_id = $1`,
	} {
		if _, err := e.pool.Exec(ctx, sql, p.UserID); err == nil {
			t.Errorf("%s: allowed", name)
		}
	}
	if _, err := e.pool.Exec(ctx, `UPDATE identity.closure_requests SET closed_at = requested_at + interval '1 hour', closed_by = $2 WHERE user_id = $1`, p.UserID, staff); err != nil {
		t.Fatalf("closing an open request: %v", err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE identity.closure_requests SET closed_at = requested_at + interval '2 hours', closed_by = $2 WHERE user_id = $1`, p.UserID, staff); err == nil {
		t.Fatal("a closed request was closed again")
	}
}
