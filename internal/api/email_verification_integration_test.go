//go:build integration

package api_test

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/itsivali/lottery-platform/internal/identity"
)

var verifyLink = regexp.MustCompile(`https://app\.example\.com/verify-email\?token=([A-Za-z0-9_-]+)`)

func TestAPI_AnEmailAddressIsConfirmedThroughAnEmailedLinkOnce(t *testing.T) {
	sent := &outbox{}
	e := setupWith(t, func(d *apiDeps) {
		d.Identity.WithPasswordResets(identity.PasswordResets{Sender: sent, WebAppURL: "https://app.example.com"})
	})
	s := e.signIn(t)
	verified := func(want bool) {
		t.Helper()
		if r := e.do(t, http.MethodGet, "/v1/account", s.access, nil); r.Code != http.StatusOK || r.Body["email_verified"] != want {
			t.Fatalf("email_verified, want %v: %d %s", want, r.Code, r.Raw)
		}
	}
	token := func(i int) string {
		t.Helper()
		if len(sent.msgs) <= i {
			t.Fatalf("%d messages sent, want more than %d", len(sent.msgs), i)
		}
		m := verifyLink.FindStringSubmatch(sent.msgs[i].Text)
		if m == nil {
			t.Fatalf("no link in:\n%s", sent.msgs[i].Text)
		}
		return m[1]
	}
	verify := func(tok string) resp {
		return e.do(t, http.MethodPost, "/v1/auth/email/verify", "", map[string]any{"token": tok})
	}

	// sign-up sent the link to the address given, as text and as the branded page with the logo inside the message
	verified(false)
	if len(sent.msgs) != 1 || sent.msgs[0].To[0] != s.email || !strings.Contains(sent.msgs[0].Subject, "confirm your email address") {
		t.Fatalf("sent %+v", sent.msgs)
	}
	m := sent.msgs[0]
	if !strings.Contains(m.HTML, `src="cid:logo.png"`) || !strings.Contains(m.HTML, "4 DRAWS TICKET LTD") || !strings.Contains(m.HTML, token(0)) ||
		len(m.Inline) != 1 || m.Inline[0].ContentType != "image/png" || len(m.Inline[0].Data) == 0 {
		t.Fatalf("the branded page: %d inline, html:\n%s", len(m.Inline), m.HTML)
	}

	// a link that was never sent confirms nothing
	if r := verify("not-a-token"); r.Code != http.StatusGone || !hasType(r, "invalid-token") {
		t.Fatalf("bad token: %d %s", r.Code, r.Raw)
	}
	// the player may ask for the link again; each link works on its own
	if r := e.do(t, http.MethodPost, "/v1/account/email/verification", s.access, nil); r.Code != http.StatusAccepted || len(sent.msgs) != 2 {
		t.Fatalf("send again: %d %s, sent %d", r.Code, r.Raw, len(sent.msgs))
	}

	// changing the address sends a link to the new one, and the links sent to the old one confirm nothing
	next := e.email()
	if r := e.do(t, http.MethodPut, "/v1/account/email", s.access, map[string]any{"email": next, "current_password": strongPassword}); r.Code != http.StatusNoContent {
		t.Fatalf("change: %d %s", r.Code, r.Raw)
	}
	if len(sent.msgs) != 3 || sent.msgs[2].To[0] != next {
		t.Fatalf("after the change sent %d, last to %v", len(sent.msgs), sent.msgs[len(sent.msgs)-1].To)
	}
	if r := verify(token(0)); r.Code != http.StatusGone {
		t.Fatalf("a link for the old address: %d %s", r.Code, r.Raw)
	}
	verified(false)

	// the link for the address on the account confirms it, without a session, once
	if r := verify(token(2)); r.Code != http.StatusNoContent {
		t.Fatalf("verify: %d %s", r.Code, r.Raw)
	}
	verified(true)
	if r := verify(token(2)); r.Code != http.StatusGone {
		t.Fatalf("the link worked twice: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/account/email/verification", s.access, nil); r.Code != http.StatusConflict || !hasType(r, "email-already-verified") {
		t.Fatalf("send again when confirmed: %d %s", r.Code, r.Raw)
	}
	var audited int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit.audit_log WHERE action = 'identity.email_verified' AND entity_id = $1::text`, e.userID(t, s)).Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("audited %d (%v)", audited, err)
	}

	// giving the same address again keeps it confirmed; a different one is a new claim
	if r := e.do(t, http.MethodPut, "/v1/account/email", s.access, map[string]any{"email": next, "current_password": strongPassword}); r.Code != http.StatusNoContent {
		t.Fatalf("same address: %d %s", r.Code, r.Raw)
	}
	verified(true)
	if r := e.do(t, http.MethodPut, "/v1/account/email", s.access, map[string]any{"email": e.email(), "current_password": strongPassword}); r.Code != http.StatusNoContent {
		t.Fatalf("another address: %d %s", r.Code, r.Raw)
	}
	verified(false)

	// a link runs out after 24 hours
	e.exec(t, `UPDATE identity.email_verifications SET created_at = created_at - interval '2 days', expires_at = expires_at - interval '2 days' WHERE used_at IS NULL`)
	if r := verify(token(len(sent.msgs) - 1)); r.Code != http.StatusGone {
		t.Fatalf("an expired link: %d %s", r.Code, r.Raw)
	}
}

func TestAPI_WithoutAMailServerAnAccountIsStillCreatedAndTheLinkCannotBeSent(t *testing.T) {
	e := setup(t) // no web app address: nothing to send links with
	s := e.signIn(t)
	if r := e.do(t, http.MethodGet, "/v1/account", s.access, nil); r.Code != http.StatusOK || r.Body["email_verified"] != false {
		t.Fatalf("account: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/account/email/verification", s.access, nil); r.Code != http.StatusServiceUnavailable {
		t.Fatalf("send: %d %s", r.Code, r.Raw)
	}
}

func TestAPI_AnAccountMadeWithGoogleHasItsGoogleAddressAlreadyConfirmed(t *testing.T) {
	e := setup(t)
	devices := 0
	device := func() map[string]any { // a device carries two accounts at most
		devices++
		return map[string]any{"fingerprint": fmt.Sprintf("fp-g-%d", devices), "platform": "web"}
	}
	signup := func(token, msisdn string) resp {
		dev := device()
		return e.do(t, http.MethodPost, "/v1/auth/sso/google/signup", "", map[string]any{"first_name": "Amina", "second_name": "Otieno", "national_id": "24681357",
			"date_of_birth": "1990-05-17", "adult_declaration": true, "id_token": token, "nonce": "n1", "msisdn": msisdn, "device": dev})
	}
	r := signup("ok:g-amina;email=Amina.Otieno.Test@gmail.com", e.msisdn())
	if r.Code != http.StatusCreated {
		t.Fatalf("sign-up: %d %s", r.Code, r.Raw)
	}
	if a := e.do(t, http.MethodGet, "/v1/account", r.Body["access_token"].(string), nil); a.Body["email"] != "amina.otieno.test@gmail.com" || a.Body["email_verified"] != true {
		t.Fatalf("account: %s", a.Raw)
	}
	// an address Google does not vouch for is not taken from the token
	r = signup("ok:g-other;email=other@example.com", e.msisdn())
	if a := e.do(t, http.MethodGet, "/v1/account", r.Body["access_token"].(string), nil); r.Code != http.StatusCreated || a.Body["email"] != nil || a.Body["email_verified"] != false {
		t.Fatalf("unvouched: %d %s", r.Code, a.Raw)
	}
	// an address another account typed and never confirmed moves to the account Google vouches for (ADR 0041): the
	// first is told to confirm, and may instead sign up; the claim is released and recorded
	s := e.signIn(t)
	claimer := e.userID(t, s)
	e.exec(t, `UPDATE identity.users SET email = 'taken.test@gmail.com' WHERE id = $1`, claimer)
	if r := e.do(t, http.MethodPost, "/v1/auth/sso/google", "", map[string]any{"id_token": "ok:g-taken;email=taken.test@gmail.com", "nonce": "n1", "device": device()}); r.Code != http.StatusConflict || !hasType(r, "sso-email-unverified") {
		t.Fatalf("an address only claimed: %d %s", r.Code, r.Raw)
	}
	r = signup("ok:g-taken;email=taken.test@gmail.com", e.msisdn())
	if r.Code != http.StatusCreated {
		t.Fatalf("taken sign-up: %d %s", r.Code, r.Raw)
	}
	if a := e.do(t, http.MethodGet, "/v1/account", r.Body["access_token"].(string), nil); a.Body["email"] != "taken.test@gmail.com" || a.Body["email_verified"] != true {
		t.Fatalf("the Google account: %s", a.Raw)
	}
	if a := e.do(t, http.MethodGet, "/v1/account", s.access, nil); a.Code != http.StatusOK || a.Body["email"] != nil {
		t.Fatalf("the account that only claimed it: %s", a.Raw)
	}
	var released int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit.audit_log WHERE action = 'identity.email_claim_released' AND entity_id = $1::text`, claimer).Scan(&released); err != nil || released != 1 {
		t.Fatalf("released %d (%v)", released, err)
	}
	// an address another account has confirmed stays there: a Google sign-up asked for directly gets none
	owner := e.signIn(t)
	e.exec(t, `UPDATE identity.users SET email = 'owned.test@gmail.com', email_verified_at = now() WHERE id = $1`, e.userID(t, owner))
	r = signup("ok:g-owned;email=owned.test@gmail.com", e.msisdn())
	if a := e.do(t, http.MethodGet, "/v1/account", r.Body["access_token"].(string), nil); r.Code != http.StatusCreated || a.Body["email"] != nil {
		t.Fatalf("an owned address: %d %s", r.Code, a.Raw)
	}
	// a Google identity already linked to an account confirms the account's address when it is the same one
	linked := e.signIn(t)
	id := e.userID(t, linked)
	e.exec(t, `UPDATE identity.users SET email = 'linked.test@gmail.com' WHERE id = $1`, id)
	e.exec(t, `INSERT INTO identity.auth_identities (user_id, provider, subject, created_at) VALUES ($1, 'google', 'g-linked', now())`, id)
	r = e.do(t, http.MethodPost, "/v1/auth/sso/google", "", map[string]any{"id_token": "ok:g-linked;email=linked.test@gmail.com", "nonce": "n1", "device": device()})
	if a := e.do(t, http.MethodGet, "/v1/account", r.Body["access_token"].(string), nil); r.Code != http.StatusOK || a.Body["email_verified"] != true {
		t.Fatalf("linked: %d %s", r.Code, a.Raw)
	}
}

func TestAPI_AnAddressOnlyClaimedDoesNotBlockItsOwner(t *testing.T) {
	sent := &outbox{}
	e := setupWith(t, func(d *apiDeps) {
		d.Identity.WithPasswordResets(identity.PasswordResets{Sender: sent, WebAppURL: "https://app.example.com"})
	})
	signup := func(email, fp string) resp {
		return e.do(t, http.MethodPost, "/v1/auth/signup", "", map[string]any{"first_name": "Amina", "second_name": "Otieno", "national_id": "24681357",
			"date_of_birth": "1990-05-17", "adult_declaration": true, "msisdn": e.msisdn(), "email": email, "password": strongPassword,
			"device": map[string]any{"fingerprint": fp, "platform": "android"}})
	}
	account := func(access string) resp { return e.do(t, http.MethodGet, "/v1/account", access, nil) }
	linkTo := func(i int) string {
		t.Helper()
		m := verifyLink.FindStringSubmatch(sent.msgs[i].Text)
		if m == nil {
			t.Fatalf("no link in message %d", i)
		}
		return m[1]
	}
	verify := func(tok string) resp {
		return e.do(t, http.MethodPost, "/v1/auth/email/verify", "", map[string]any{"token": tok})
	}

	// someone gives an address that is not theirs and never confirms it; its owner can still sign up with it
	squatter := signup("wanjiru@example.com", "fp-squatter")
	owner := signup("wanjiru@example.com", "fp-owner")
	if squatter.Code != http.StatusCreated || owner.Code != http.StatusCreated || len(sent.msgs) != 2 {
		t.Fatalf("two claims: %d %s / %d %s, sent %d", squatter.Code, squatter.Raw, owner.Code, owner.Raw, len(sent.msgs))
	}
	sAccess, oAccess := squatter.Body["access_token"].(string), owner.Body["access_token"].(string)

	// the owner opens their link: the address is theirs, and the other account loses it
	if r := verify(linkTo(1)); r.Code != http.StatusNoContent {
		t.Fatalf("verify: %d %s", r.Code, r.Raw)
	}
	if a := account(oAccess); a.Body["email"] != "wanjiru@example.com" || a.Body["email_verified"] != true {
		t.Fatalf("the owner: %s", a.Raw)
	}
	if a := account(sAccess); a.Body["email"] != nil || a.Body["email_verified"] != false {
		t.Fatalf("the other account: %s", a.Raw)
	}
	// the link sent to the other account confirms nothing now
	if r := verify(linkTo(0)); r.Code != http.StatusGone {
		t.Fatalf("the other account's link: %d %s", r.Code, r.Raw)
	}
	// a confirmed address is taken: not at sign-up, not by a change of address
	if r := signup("wanjiru@example.com", "fp-third"); r.Code != http.StatusConflict || !hasType(r, "email-registered") {
		t.Fatalf("sign-up with a confirmed address: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/account/email", sAccess, map[string]any{"email": "wanjiru@example.com", "current_password": strongPassword}); r.Code != http.StatusConflict || !hasType(r, "email-registered") {
		t.Fatalf("change to a confirmed address: %d %s", r.Code, r.Raw)
	}
	// a password-reset link for an address goes to the account that confirmed it
	sent.msgs = nil
	if r := e.do(t, http.MethodPost, "/v1/auth/password/forgot", "", map[string]any{"email": "wanjiru@example.com"}); r.Code != http.StatusAccepted || len(sent.msgs) != 1 {
		t.Fatalf("forgot: %d %s, sent %d", r.Code, r.Raw, len(sent.msgs))
	}
	var resetFor string
	if err := e.pool.QueryRow(t.Context(), `SELECT user_id::text FROM identity.password_resets ORDER BY created_at DESC LIMIT 1`).Scan(&resetFor); err != nil {
		t.Fatal(err)
	}
	var ownerID string
	if err := e.pool.QueryRow(t.Context(), `SELECT id::text FROM identity.users WHERE email = 'wanjiru@example.com'`).Scan(&ownerID); err != nil || resetFor != ownerID {
		t.Fatalf("the reset link is for %s, want %s (%v)", resetFor, ownerID, err)
	}
}
