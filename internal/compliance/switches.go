package compliance

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/compliance/store"
)

// Area is one money area a switch can pause on its own (ADR 0036).
type Area string

const (
	AreaTopUps      Area = "top-ups"
	AreaPlays       Area = "plays"
	AreaWithdrawals Area = "withdrawals"
	AreaPayouts     Area = "payouts"
)

// Areas are the switchable areas, in the order they are shown.
var Areas = []Area{AreaTopUps, AreaPlays, AreaWithdrawals, AreaPayouts}

// opArea is the area each operation belongs to: pausing top-ups refuses new top-ups, their prompts and paybill
// payments; pausing payouts holds the B2C sends while withdrawals may still be requested.
var opArea = map[Op]Area{
	OpTopUp: AreaTopUps, OpPush: AreaTopUps, OpPaybill: AreaTopUps,
	OpPlay:       AreaPlays,
	OpWithdrawal: AreaWithdrawals,
	OpPayout:     AreaPayouts,
}

// Switch is an area's state: running, or paused by someone for a reason.
type Switch struct {
	Area      Area       `json:"area"`
	Paused    bool       `json:"paused"`
	Reason    string     `json:"reason,omitempty"`
	ChangedBy string     `json:"changed_by,omitempty"`
	At        *time.Time `json:"at,omitempty"` // nil: never switched
}

// Switches reads every area's state.
func (g *Gate) Switches(ctx context.Context) ([]Switch, error) {
	if g == nil {
		return nil, nil
	}
	rows, err := store.New(g.pool).LatestAreaSwitches(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]store.ComplianceAreaSwitch{}
	for _, r := range rows {
		seen[r.Area] = r
	}
	out := make([]Switch, 0, len(Areas))
	for _, a := range Areas {
		sw := Switch{Area: a}
		if r, ok := seen[string(a)]; ok {
			at := r.CreatedAt
			sw = Switch{Area: a, Paused: r.Paused, Reason: r.Reason, ChangedBy: r.ChangedBy, At: &at}
		}
		out = append(out, sw)
	}
	return out, nil
}

// SetSwitch pauses or resumes one area, with the reason, audited. Who may do which is the API's to decide (anyone
// on staff pauses; an admin resumes), as for the operating mode.
func (g *Gate) SetSwitch(ctx context.Context, area Area, paused bool, actor, reason string) (Switch, error) {
	known := false
	for _, a := range Areas {
		known = known || a == area
	}
	if !known || !validActor(actor) || !text(reason, 1000) {
		return Switch{}, ErrInvalid
	}
	reason = strings.TrimSpace(reason)
	var sw Switch
	err := pgx.BeginFunc(ctx, g.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, changeLock); err != nil {
			return err
		}
		now, err := g.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		id, err := store.New(tx).InsertAreaSwitch(ctx, store.InsertAreaSwitchParams{Area: string(area), Paused: paused, Reason: reason, ChangedBy: actor, At: now})
		if err != nil {
			return err
		}
		sw = Switch{Area: area, Paused: paused, Reason: reason, ChangedBy: actor, At: &now}
		action := "compliance.area_resumed"
		if paused {
			action = "compliance.area_paused"
		}
		return g.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: actor, Action: action, EntityType: "area_switch",
			EntityID: id, Details: map[string]any{"area": string(area), "reason": reason}})
	})
	return sw, err
}
