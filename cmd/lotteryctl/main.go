// Command lotteryctl is the admin CLI: migrations and the payments operator actions (reverse a top-up M-Pesa took
// back, resolve an unconfirmed top-up from the statement, credit or refund a paybill payment held in suspense, check an
// unverified paybill payment again, run the internal reconciliation), manual KYC review and the emergency staff password. It needs DATABASE_URL; operator actions
// (-operator EMAIL) sign the staff member in with their back-office password and are audited as staff "staff:<id>" (ADR 0032). Registering the paybill URLs and simulating a sandbox payment need the api's
// Daraja configuration.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/itsivali/lottery-platform/internal/alerts"
	"github.com/itsivali/lottery-platform/internal/app"
	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/compliance"
	"github.com/itsivali/lottery-platform/internal/draws"
	"github.com/itsivali/lottery-platform/internal/draws/drand"
	"github.com/itsivali/lottery-platform/internal/game"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/kyc"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/payments"
	"github.com/itsivali/lottery-platform/internal/payments/daraja"
	"github.com/itsivali/lottery-platform/internal/payouts"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/config"
	"github.com/itsivali/lottery-platform/internal/platform/crypto"
	"github.com/itsivali/lottery-platform/internal/platform/db"
	"github.com/itsivali/lottery-platform/internal/rg"
	"github.com/itsivali/lottery-platform/internal/staff"
	"github.com/itsivali/lottery-platform/internal/wallet"
)

const usage = `usage:
  lotteryctl migrate up|down
  lotteryctl payments reconcile
  lotteryctl payments reverse -receipt R -reason TEXT -operator EMAIL
  lotteryctl payments resolve -payment ID -receipt R -amount KES -operator EMAIL
  lotteryctl payments suspense-credit -receipt R -user ID -operator EMAIL
  lotteryctl payments suspense-refund -receipt R -refund-ref REF -operator EMAIL
  lotteryctl payments c2b-recheck -receipt R -operator EMAIL
  lotteryctl payments c2b-register
  lotteryctl payments c2b-simulate -amount KES -msisdn 2547XXXXXXXX -account ACCOUNT
  lotteryctl kyc pending -operator EMAIL
  lotteryctl kyc approve|reject -user ID -reason TEXT -operator EMAIL
  lotteryctl game reconcile
  lotteryctl game simulate [-days N] [-slots N] [-lucky N] [-price KES]   (no database needed)
  lotteryctl draws list
  lotteryctl draws show -draw ID
  lotteryctl draws sign|approve -draw ID -operator EMAIL
  lotteryctl draws void -draw ID -reason TEXT -operator EMAIL
  lotteryctl draws entries -draw ID     (the full entry list as JSON lines, for tools/verifier draw -entries)
  lotteryctl draws reconcile
  lotteryctl draws overdue
  lotteryctl payouts list -state held|manual|sent|timed_out|result_received|pending|succeeded|failed
  lotteryctl payouts approve -payout ID -operator EMAIL
  lotteryctl payouts reject -payout ID -reason TEXT -operator EMAIL
  lotteryctl payouts settle-paid -payout ID -receipt R -operator EMAIL        (a manual payout found on the statement: M-Pesa is asked; paid once it confirms the receipt)
  lotteryctl payouts settle-failed -payout ID -reason TEXT -operator EMAIL    (a manual payout not on the statement: a second person confirms)
  lotteryctl payouts retry-prize -result ID -operator EMAIL                   (a prize whose payout failed)
  lotteryctl payouts reconcile
  lotteryctl tax batch -operator EMAIL                    (freeze the withholding not yet batched into the next numbered batch)
  lotteryctl tax file -batch N                           (the batch's file, rebuilt and checked against its SHA-256)
  lotteryctl tax remit -batch N -prn PRN -amount KES -operator EMAIL   (record the KRA payment; not the preparer)
  lotteryctl tax batches
  lotteryctl alerts list [-all] [-kind K] [-state open|acknowledged|resolved|all]   (-all is -state all)
  lotteryctl alerts ack -alert ID -note TEXT -operator EMAIL   (stops reminders; closes a payout freeze)
  lotteryctl alerts watch                                (run the checks and send what is due now)
  lotteryctl compliance status                          (the operating mode as recorded, live requests waiting, the latest changes)
  lotteryctl compliance stop -reason TEXT -operator EMAIL   (emergency: stop top-ups, plays, withdrawals and payouts; an admin
                                                         resumes in the back office, where going live takes two admins)
  lotteryctl staff password -email E [-password P] [-admin]   (emergency: a temporary back-office password for E, read
                                                              from stdin without -password; -admin also makes E an
                                                              administrator, creating them if needed)`

type command struct {
	name                               string
	receipt, reason, operator, payment string
	user, refundRef, msisdn, account   string
	draw, payout, state, result        string
	prn, alert, note, kind             string
	email, password                    string
	all, admin                         bool
	batch                              int64
	amount                             int64
	days, slots, lucky                 int
	price                              int64
}

// flagsOf lists, per command, the flags it requires; every other flag must be absent.
var flagsOf = map[string][]string{
	"migrate up": nil, "migrate down": nil, "payments reconcile": nil, "payments c2b-register": nil,
	"payments reverse":         {"receipt", "reason", "operator"},
	"payments resolve":         {"payment", "receipt", "amount", "operator"},
	"payments suspense-credit": {"receipt", "user", "operator"},
	"payments suspense-refund": {"receipt", "refund-ref", "operator"},
	"payments c2b-recheck":     {"receipt", "operator"},
	"payments c2b-simulate":    {"amount", "msisdn", "account"},
	"kyc pending":              {"operator"},
	"kyc approve":              {"user", "reason", "operator"},
	"kyc reject":               {"user", "reason", "operator"},
	"game reconcile":           nil,
	"game simulate":            nil,
	"draws list":               nil,
	"draws show":               {"draw"},
	"draws sign":               {"draw", "operator"},
	"draws approve":            {"draw", "operator"},
	"draws void":               {"draw", "reason", "operator"},
	"draws entries":            {"draw"},
	"draws reconcile":          nil,
	"draws overdue":            nil,
	"payouts list":             {"state"},
	"payouts approve":          {"payout", "operator"},
	"payouts reject":           {"payout", "reason", "operator"},
	"payouts settle-paid":      {"payout", "receipt", "operator"},
	"payouts settle-failed":    {"payout", "reason", "operator"},
	"payouts retry-prize":      {"result", "operator"},
	"payouts reconcile":        nil,
	"tax batch":                {"operator"},
	"tax file":                 {"batch"},
	"tax remit":                {"batch", "prn", "amount", "operator"},
	"tax batches":              nil,
	"alerts list":              nil,
	"alerts ack":               {"alert", "note", "operator"},
	"alerts watch":             nil,
	"staff password":           {"email"},
	"compliance status":        nil,
	"compliance stop":          {"reason", "operator"},
}

// optionalOf lists, per command, the flags it may take; the rest of a command's flags are as in flagsOf.
var optionalOf = map[string][]string{
	"game simulate":  {"days", "slots", "lucky", "price"},
	"alerts list":    {"all", "kind", "state"},
	"staff password": {"password", "admin"},
}

func parse(args []string) (command, error) {
	bad := errors.New(usage)
	if len(args) < 2 {
		return command{}, bad
	}
	c := command{name: args[0] + " " + args[1]}
	required, ok := flagsOf[c.name]
	if !ok {
		return command{}, bad
	}
	fs := flag.NewFlagSet(c.name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.receipt, "receipt", "", "M-Pesa receipt")
	fs.StringVar(&c.operator, "operator", "", "your back-office email address: you are asked for your password, and the action is audited as staff:<your id>")
	fs.StringVar(&c.reason, "reason", "", "why the top-up was reversed")
	fs.StringVar(&c.payment, "payment", "", "payment id")
	fs.StringVar(&c.user, "user", "", "player (user id) to credit")
	fs.StringVar(&c.refundRef, "refund-ref", "", "M-Pesa reference of the refund")
	fs.StringVar(&c.msisdn, "msisdn", "", "payer number (sandbox)")
	fs.StringVar(&c.account, "account", "", "account number typed by the payer")
	fs.StringVar(&c.draw, "draw", "", "draw id")
	fs.StringVar(&c.payout, "payout", "", "payout id")
	fs.StringVar(&c.state, "state", "", "payout state, or alert state (open, acknowledged, resolved, all)")
	fs.StringVar(&c.kind, "kind", "", "alert kind (topup.unconfirmed, jobs.failing, ...)")
	fs.StringVar(&c.result, "result", "", "draw result (prize) id")
	fs.Int64Var(&c.amount, "amount", 0, "amount, whole KES")
	fs.Int64Var(&c.batch, "batch", 0, "KRA remittance batch number")
	fs.StringVar(&c.prn, "prn", "", "KRA payment registration number")
	fs.StringVar(&c.alert, "alert", "", "alert id")
	fs.StringVar(&c.note, "note", "", "what was found and done (audited)")
	fs.BoolVar(&c.all, "all", false, "include resolved alerts")
	fs.StringVar(&c.email, "email", "", "a staff member's email address")
	fs.StringVar(&c.password, "password", "", "the temporary password (omit it to type it on stdin, so it stays out of the shell history)")
	fs.BoolVar(&c.admin, "admin", false, "also make the member an administrator (creating them if needed)")
	fs.IntVar(&c.days, "days", 0, "simulated player-days per number of plays")
	fs.IntVar(&c.slots, "slots", 0, "slot count")
	fs.IntVar(&c.lucky, "lucky", 0, "lucky slots")
	fs.Int64Var(&c.price, "price", 0, "price of a play, whole KES")
	if err := fs.Parse(args[2:]); err != nil || fs.NArg() != 0 {
		return command{}, bad
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		set[f.Name] = f.Value.String() != "" && f.Value.String() != "0" && f.Value.String() != "false"
	})
	for _, name := range required {
		if !set[name] {
			return command{}, bad
		}
		delete(set, name)
	}
	for _, name := range optionalOf[c.name] {
		delete(set, name)
	}
	if len(set) > 0 || c.amount < 0 || c.batch < 0 || c.days < 0 || c.slots < 0 || c.lucky < 0 || c.price < 0 {
		return command{}, bad
	}
	if c.operator != "" {
		// the staff member's back-office email address: run signs them in with their password and records the action
		// under their staff id, as the back office does, so a two-person rule (payout approval and settlement, draw
		// sign-off, KRA remittance) is met only by two people who each proved who they are (ADR 0032)
		c.operator = strings.ToLower(strings.TrimSpace(c.operator))
		if !strings.Contains(c.operator, "@") {
			return command{}, errors.New("-operator is your back-office email address; you are asked for your password")
		}
	}
	return c, nil
}

func main() {
	cmd, err := parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := run(context.Background(), cmd, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd command, out io.Writer) error {
	if cmd.name == "game simulate" {
		return simulate(cmd, out)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch cmd.name {
	case "migrate up":
		return db.MigrateUp(ctx, cfg.DatabaseURL)
	case "migrate down":
		return db.MigrateDown(ctx, cfg.DatabaseURL)
	}

	if cmd.operator != "" {
		op, done, err := signIn(ctx, cfg, cmd.operator)
		if err != nil {
			return err
		}
		defer done()
		cmd.operator = op
	}

	switch cmd.name {
	case "payments c2b-register", "payments c2b-simulate":
		return paybill(ctx, cfg, cmd, out)
	case "kyc pending", "kyc approve", "kyc reject":
		return kycReview(ctx, cfg, cmd, out)
	case "game reconcile":
		return gameReconcile(ctx, cfg, out)
	case "draws list", "draws show", "draws sign", "draws approve", "draws void", "draws entries", "draws reconcile", "draws overdue":
		return drawsCmd(ctx, cfg, cmd, out)
	case "payouts list", "payouts approve", "payouts reject", "payouts settle-paid", "payouts settle-failed", "payouts retry-prize",
		"payouts reconcile", "tax batch", "tax file", "tax remit", "tax batches":
		return payoutsCmd(ctx, cfg, cmd, out)
	case "alerts list", "alerts ack", "alerts watch":
		return alertsCmd(ctx, cfg, cmd, out)
	case "staff password":
		return staffPassword(ctx, cfg, cmd, os.Stdin, out)
	case "compliance status", "compliance stop":
		return complianceCmd(ctx, cfg, cmd, out)
	}

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	// Operator actions need no Daraja, job queue or secrets: they work on recorded payments, the ledger and wallets.
	aw := audit.NewWriter()
	clk := clock.Postgres{}
	ls := ledger.NewStore()
	svc := payments.NewService(pool, clk, nil, nil, identity.NewService(pool, clk, nil, aw, identity.Config{}),
		rg.NewService(pool, clk, aw, rg.Config{CoolingOff: 24 * time.Hour}), wallet.NewService(ls), ls, aw, payments.Config{})

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	switch cmd.name {
	case "payments reconcile":
		r, err := svc.Reconcile(ctx)
		if err != nil {
			return err
		}
		if err := enc.Encode(r); err != nil {
			return err
		}
		if !r.OK {
			return errors.New("reconciliation found mismatches")
		}
		return nil
	case "payments reverse":
		r, err := svc.Reverse(ctx, cmd.receipt, cmd.reason, cmd.operator)
		if err != nil {
			return err
		}
		return enc.Encode(r)
	case "payments resolve":
		if err := svc.ResolveUnconfirmed(ctx, cmd.payment, cmd.receipt, cmd.amount, cmd.operator); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "credited", cmd.payment, "from receipt", cmd.receipt)
		return err
	case "payments suspense-credit":
		id, err := svc.ResolveSuspense(ctx, cmd.receipt, cmd.user, cmd.operator)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "credited suspense receipt", cmd.receipt, "as payment", id)
		return err
	case "payments suspense-refund":
		if err := svc.RefundSuspense(ctx, cmd.receipt, cmd.refundRef, cmd.operator); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "recorded refund of suspense receipt", cmd.receipt)
		return err
	case "payments c2b-recheck":
		if err := svc.RecheckC2B(ctx, cmd.receipt, cmd.operator); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "paybill payment", cmd.receipt, "will be verified again by the worker")
		return err
	}
	return fmt.Errorf("unknown command %q", cmd.name)
}

// paybill runs the commands that talk to Daraja: they need the api's payments configuration with C2B.
func paybill(ctx context.Context, cfg config.Config, cmd command, out io.Writer) error {
	if !cfg.Payments.C2BEnabled() {
		return errors.New("paybill payments are not configured (DARAJA_* and DARAJA_C2B_SHORTCODE, DARAJA_INITIATOR_NAME, DARAJA_SECURITY_CREDENTIAL)")
	}
	client, err := app.NewDaraja(cfg.Payments)
	if err != nil {
		return err
	}
	if cmd.name == "payments c2b-simulate" {
		if err := client.SimulateC2B(ctx, cmd.amount, cmd.msisdn, cmd.account); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "simulated a paybill payment; Daraja will call the confirmation URL")
		return err
	}
	// Cancel when our validation URL cannot be reached: a payment we could not check is refused, not taken.
	v, c := cfg.Payments.C2BURL("validate"), cfg.Payments.C2BURL("confirm")
	if err := client.RegisterC2BURLs(ctx, v, c, daraja.C2BCancelOnTimeout); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "registered the paybill URLs for shortcode", cfg.Payments.C2BShortCode)
	return err
}

// kycReview lists submissions waiting for a staff decision or records one (ADR 0009: identities are checked by hand).
// Listing decrypts personal data, so it needs the api's DATA_KEY (and the rest of its configuration) and is audited.
func kycReview(ctx context.Context, cfg config.Config, cmd command, out io.Writer) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	var kr *crypto.Keyring
	if cmd.name == "kyc pending" {
		if !cfg.API.Enabled {
			return errors.New("listing submissions decrypts them: set the api's configuration (DATA_KEY and the other api secrets)")
		}
		if kr, err = crypto.NewKeyring(cfg.API.DataKeyActive, cfg.API.DataKeys); err != nil {
			return err
		}
	}
	svc := kyc.NewService(pool, clock.Postgres{}, kyc.ManualProvider{}, kr, cfg.API.IdentityKey, audit.NewWriter(), nil)
	if cmd.name == "kyc pending" {
		list, err := svc.AwaitingReview(ctx, cmd.operator)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	}
	st, err := svc.Review(ctx, cmd.user, cmd.operator, cmd.name == "kyc approve", cmd.reason)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "user", cmd.user, "is now", st)
	return err
}

// staffPassword is the way back into the back office when every administrator is locked out (ADR 0019): it gives a
// member a temporary password (changed at their next sign-in), re-enables them and ends their sessions. It needs the
// api's AUTH_KEY (the password pepper) and the database; the password is read from stdin unless -password is given.
func staffPassword(ctx context.Context, cfg config.Config, cmd command, in io.Reader, out io.Writer) error {
	if !cfg.API.Enabled || len(cfg.API.AuthKey) == 0 {
		return errors.New("staff passwords are peppered with the api's AUTH_KEY: set the api's configuration")
	}
	pw := cmd.password
	if pw == "" {
		_, _ = fmt.Fprint(out, "Temporary password (12 characters or more): ")
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && line == "" {
			return fmt.Errorf("reading the password: %w", err)
		}
		pw = strings.TrimRight(line, "\r\n")
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	svc := staff.NewService(pool, clock.Postgres{}, nil, audit.NewWriter(), crypto.MAC(cfg.API.AuthKey, "staff-password-pepper"))
	rec, err := svc.EmergencyPassword(ctx, cmd.email, pw, cmd.admin)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s has a temporary password and must change it at the next sign-in (roles: %v, active: %v, sessions ended)\n", rec.Email, rec.Roles, rec.Active)
	return err
}

// gameReconcile runs the game reconciliation (stakes, tickets, spins, seed chains); it needs only the database.
func gameReconcile(ctx context.Context, cfg config.Config, out io.Writer) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	svc := game.NewService(pool, clock.Postgres{}, nil, nil, nil, nil, nil)
	r, err := svc.Reconcile(ctx)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return err
	}
	if !r.OK {
		return errors.New("game reconciliation found mismatches")
	}
	return nil
}

// simulate reports how often a day is lucky and what a draw chance costs under an odds version, from real fair spins.
// Unset flags take the odds version in force (placeholders until the owner sets them).
func simulate(cmd command, out io.Writer) error {
	def := func(v, d int) int {
		if v == 0 {
			return d
		}
		return v
	}
	if cmd.price == 0 {
		cmd.price = 100
	}
	cmd.days, cmd.slots, cmd.lucky = def(cmd.days, 100_000), def(cmd.slots, 12), def(cmd.lucky, 1)
	rep, err := game.Simulate(game.SimConfig{Slots: cmd.slots, LuckySlots: cmd.lucky, PriceKES: cmd.price,
		Tiers: map[int]int{1: 1, 2: 2, 3: 3, 4: 5}, Days: cmd.days})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// drawsCmd shows, exports, reconciles and monitors draws, and records officials' sign-offs (two different officials sign the frozen entry list before the
// drand round and approve the result). Voiding re-runs the draw with a new company seed, which is encrypted with the
// api's DATA_KEY, so it needs the api's configuration; the other commands need only the database.
func drawsCmd(ctx context.Context, cfg config.Config, cmd command, out io.Writer) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	var kr *crypto.Keyring
	if cmd.name == "draws void" {
		if !cfg.API.Enabled {
			return errors.New("voiding re-runs the draw with a new encrypted seed: set the api's configuration (DATA_KEY and the other api secrets)")
		}
		if kr, err = crypto.NewKeyring(cfg.API.DataKeyActive, cfg.API.DataKeys); err != nil {
			return err
		}
	}
	svc := draws.NewService(pool, clock.Postgres{}, kr, drand.New(drand.Quicknet, drand.DefaultRelays, &http.Client{Timeout: 20 * time.Second}), ledger.NewStore())
	var res any
	switch cmd.name {
	case "draws list":
		var page draws.Page
		page, err = svc.List(ctx, 40, "")
		res = page.Draws
	case "draws show":
		res, err = svc.Get(ctx, cmd.draw)
	case "draws sign":
		res, err = svc.Sign(ctx, cmd.draw, cmd.operator)
	case "draws approve":
		res, err = svc.Approve(ctx, cmd.draw, cmd.operator)
	case "draws void":
		res, err = svc.Void(ctx, cmd.draw, cmd.operator, cmd.reason)
	case "draws reconcile":
		res, err = svc.Reconcile(ctx)
	case "draws overdue":
		res, err = svc.Overdue(ctx)
	case "draws entries":
		entries, err := svc.EntryList(ctx, cmd.draw)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(out) // one entry per line
		for _, e := range entries {
			if err := enc.Encode(e); err != nil {
				return err
			}
		}
		return nil
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

// payoutsCmd lists and reconciles payouts, batches and records remittances of withheld tax, and records operators'
// decisions:
// releasing or refusing a held payout, settling one whose outcome Daraja could not confirm from the M-Pesa statement,
// paying a failed prize again. Approving and retrying queue a B2C request, so they need the api's payments
// configuration with B2C; the other commands need only the database.
func payoutsCmd(ctx context.Context, cfg config.Config, cmd command, out io.Writer) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: db.RiverSchema})
	if err != nil {
		return err
	}
	var d payouts.Daraja
	if cmd.name == "payouts approve" || cmd.name == "payouts retry-prize" || cmd.name == "payouts settle-paid" {
		if !cfg.Payments.B2CEnabled() {
			return errors.New("this sends a payout or asks M-Pesa about one: set the api's payments configuration with DARAJA_B2C_SHORTCODE")
		}
		if d, err = app.NewDaraja(cfg.Payments); err != nil {
			return err
		}
	}
	aw := audit.NewWriter()
	clk := clock.Postgres{}
	ls := ledger.NewStore()
	svc := payouts.NewService(pool, clk, d, payouts.NewRiverEnqueuer(insertOnly), identity.NewService(pool, clk, nil, aw, identity.Config{}),
		wallet.NewService(ls), ls, aw, payouts.Config{}, wallet.ErrInsufficientFunds)
	var res any
	switch cmd.name {
	case "payouts list":
		res, _, err = svc.InState(ctx, cmd.state, 100, "")
	case "payouts approve":
		res, err = svc.Approve(ctx, cmd.payout, cmd.operator)
	case "payouts reject":
		res, err = svc.Reject(ctx, cmd.payout, cmd.operator, cmd.reason)
	case "payouts settle-paid":
		res, err = svc.Settle(ctx, cmd.payout, cmd.operator, true, cmd.receipt, "")
	case "payouts settle-failed":
		res, err = svc.Settle(ctx, cmd.payout, cmd.operator, false, "", cmd.reason)
	case "payouts retry-prize":
		res, err = svc.RetryPrize(ctx, cmd.result, cmd.operator)
	case "payouts reconcile":
		var r payouts.Report
		if r, err = svc.Reconcile(ctx); err == nil && !r.OK {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			if err := enc.Encode(r); err != nil {
				return err
			}
			return errors.New("reconciliation found mismatches")
		}
		res = r
	case "tax batch":
		res, _, err = svc.PrepareBatch(ctx, cmd.operator, 10_000)
	case "tax file":
		var file []byte
		if _, file, err = svc.BatchFile(ctx, cmd.batch); err != nil {
			return err
		}
		_, err = out.Write(file)
		return err
	case "tax remit":
		res, err = svc.RecordRemittance(ctx, cmd.batch, cmd.prn, cmd.amount, cmd.operator)
	case "tax batches":
		res, err = svc.Batches(ctx, 50)
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

// complianceCmd shows the operating mode or stops money moving (ADR 0027). Resuming and going live are back-office
// actions: there staff are authenticated, which a typed -operator is not.
func complianceCmd(ctx context.Context, cfg config.Config, cmd command, out io.Writer) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	g := compliance.NewGate(pool, clock.Postgres{}, audit.NewWriter(), compliance.Config{Mode: compliance.Mode(cfg.Operating.Mode),
		LicenceReference: cfg.Operating.LicenceReference})
	var res any
	switch cmd.name {
	case "compliance status":
		st, err := g.Current(ctx)
		if err != nil {
			return err
		}
		reqs, err := g.OpenLiveRequests(ctx)
		if err != nil {
			return err
		}
		hist, err := g.History(ctx, 20)
		if err != nil {
			return err
		}
		// the configured mode is this shell's OPERATING_MODE, not necessarily the api's
		res = map[string]any{"status_in_this_shell": st, "recorded": st.Latest, "live_requests": reqs, "history": hist}
	case "compliance stop":
		if res, err = g.Stop(ctx, cmd.operator, cmd.reason); err != nil {
			return err
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

// alertsCmd lists and acknowledges alerts, or runs the checks now (sending to ALERT_WEBHOOK_URL when set).
func alertsCmd(ctx context.Context, cfg config.Config, cmd command, out io.Writer) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	clk := clock.Postgres{}
	hc := &http.Client{Timeout: 20 * time.Second}
	ds := draws.NewService(pool, clk, nil, drand.New(drand.Quicknet, drand.DefaultRelays, hc), ledger.NewStore())
	svc := alerts.NewService(pool, clk, app.AlertNotifier(cfg, hc), audit.NewWriter(), alerts.Config{}, app.AlertChecks(pool, clk, ds, cfg)...)
	var res any
	switch cmd.name {
	case "alerts list":
		state, ok := alerts.ParseState(cmd.state)
		if !ok {
			return errors.New("-state must be open, acknowledged, resolved or all")
		}
		if cmd.all && cmd.state == "" {
			state = alerts.All
		}
		var l alerts.Listing
		l, err = svc.List(ctx, alerts.Filter{Kind: cmd.kind, State: state, Limit: 100})
		res = l.Alerts
	case "alerts ack":
		res, err = svc.Acknowledge(ctx, cmd.alert, cmd.operator, cmd.note)
	case "alerts watch":
		if err = svc.Watch(ctx); err == nil {
			var l alerts.Listing
			l, err = svc.List(ctx, alerts.Filter{Limit: 100})
			res = l.Alerts
		}
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}
