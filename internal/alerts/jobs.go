package alerts

import (
	"context"
	"time"

	"github.com/riverqueue/river"
)

// WatchEvery is how often the checks run and alerts are sent.
const WatchEvery = 5 * time.Minute

// WatchArgs runs the checks and sends what is due.
type WatchArgs struct{}

// Kind implements river.JobArgs.
func (WatchArgs) Kind() string { return "alerts.watch" }

type watchWorker struct {
	river.WorkerDefaults[WatchArgs]
	svc *Service
}

func (w *watchWorker) Work(ctx context.Context, _ *river.Job[WatchArgs]) error {
	return w.svc.Watch(ctx)
}

// AddWorkers registers the alert worker.
func AddWorkers(ws *river.Workers, svc *Service) {
	river.AddWorker(ws, &watchWorker{svc: svc})
}

// PeriodicJobs are the scheduled alert jobs. The first watch waits one interval, so the reconciliations that run on
// start have run before it looks for them.
func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(WatchEvery),
			func() (river.JobArgs, *river.InsertOpts) { return WatchArgs{}, nil }, &river.PeriodicJobOpts{ID: "alerts.watch"}),
	}
}
