package winnotice

import (
	"context"
	"time"

	"github.com/riverqueue/river"
)

// NotifyEvery is how often winners waiting for their notice are looked for.
const NotifyEvery = 5 * time.Minute

// NotifyArgs sends the prize notices that are due.
type NotifyArgs struct{}

// Kind implements river.JobArgs.
func (NotifyArgs) Kind() string { return "winnotice.notify" }

type notifyWorker struct {
	river.WorkerDefaults[NotifyArgs]
	svc *Service
}

func (w *notifyWorker) Work(ctx context.Context, _ *river.Job[NotifyArgs]) error {
	_, err := w.svc.Notify(ctx)
	return err
}

// AddWorkers registers the notice worker.
func AddWorkers(workers *river.Workers, svc *Service) {
	river.AddWorker(workers, &notifyWorker{svc: svc})
}

// PeriodicJobs are the scheduled notice jobs.
func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(NotifyEvery),
			func() (river.JobArgs, *river.InsertOpts) { return NotifyArgs{}, &river.InsertOpts{MaxAttempts: 1} },
			&river.PeriodicJobOpts{ID: "winnotice.notify"}),
	}
}
