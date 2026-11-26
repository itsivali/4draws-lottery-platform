package draws

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/itsivali/4draws-lottery-platform/internal/draws/store"
)

// How late a step may be before it is reported.
const (
	freezeGrace   = 30 * time.Minute // the schedule runs every 5 minutes
	signWarning   = time.Hour        // before the round: officials still have time to sign
	conductGrace  = 30 * time.Minute // after the round: drand or the worker is down
	approvalGrace = 24 * time.Hour   // after the round: the result waits for the officials
)

// Late is a draw whose next step is overdue or about to be missed.
type Late struct {
	DrawID      string    `json:"draw_id"`
	DrawType    string    `json:"draw_type"`
	PeriodStart string    `json:"period_start"`
	Status      string    `json:"status"`
	Due         time.Time `json:"due"`
	Problem     string    `json:"problem"`
}

// Overdue lists draws whose step is late: open past their freeze time, frozen with under an hour of signing left,
// signed past their round without a result, or conducted and unapproved for a day.
func (s *Service) Overdue(ctx context.Context) ([]Late, error) {
	now, err := s.clk.Now(ctx, s.pool)
	if err != nil {
		return nil, err
	}
	q := store.New(s.pool)
	out := []Late{}
	for _, status := range []string{"open", "frozen", "signed", "conducted"} {
		ids, err := q.DrawsInStatus(ctx, status)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			d, err := q.GetDraw(ctx, id)
			if err != nil {
				return nil, err
			}
			l := Late{DrawID: d.ID, DrawType: d.DrawType, PeriodStart: d.PeriodStart.Format(time.DateOnly), Status: d.Status}
			switch status {
			case "open":
				l.Due = FreezeTime(d.PeriodEnd)
				if now.Before(l.Due.Add(freezeGrace)) {
					continue
				}
				l.Problem = "not frozen after its freeze time: is the worker running?"
			case "frozen":
				l.Due = s.roundTime(d)
				if now.Before(l.Due.Add(-signWarning)) {
					continue
				}
				n, err := q.SignoffCount(ctx, store.SignoffCountParams{DrawID: d.ID, Step: "sign"})
				if err != nil {
					return nil, err
				}
				l.Problem = strconv.Itoa(int(n)) + " of " + strconv.Itoa(Officials) + " officials have signed; it is voided and re-run if not signed by the round"
			case "signed":
				l.Due = s.roundTime(d)
				if now.Before(l.Due.Add(conductGrace)) {
					continue
				}
				l.Problem = "its drand round is out but it has not been conducted: is drand reachable?"
			case "conducted":
				l.Due = s.roundTime(d).Add(approvalGrace)
				if now.Before(l.Due) {
					continue
				}
				l.Problem = "the result awaits approval by two officials"
			}
			out = append(out, l)
		}
	}
	return out, nil
}

// reportOverdue logs every late draw under one message, "draw overdue", for a log alert to match.
func (s *Service) reportOverdue(ctx context.Context) error {
	late, err := s.Overdue(ctx)
	if err != nil {
		return err
	}
	for _, l := range late {
		slog.ErrorContext(ctx, "draw overdue", "draw", l.DrawID, "type", l.DrawType, "period_start", l.PeriodStart, "status", l.Status,
			"due", l.Due, "problem", l.Problem)
	}
	return nil
}
