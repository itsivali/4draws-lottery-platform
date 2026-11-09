package aml

import (
	"context"
	"time"

	"github.com/riverqueue/river"
)

// ScanEvery is how often the rules run.
const ScanEvery = time.Hour

// ScanArgs runs the monitoring rules.
type ScanArgs struct{}

// Kind implements river.JobArgs.
func (ScanArgs) Kind() string { return "aml.scan" }

type scanWorker struct {
	river.WorkerDefaults[ScanArgs]
	svc *Service
}

func (w *scanWorker) Work(ctx context.Context, _ *river.Job[ScanArgs]) error {
	_, err := w.svc.Scan(ctx)
	return err
}

// AddWorkers registers the monitoring worker.
func AddWorkers(ws *river.Workers, svc *Service) {
	river.AddWorker(ws, &scanWorker{svc: svc})
}

// PeriodicJobs are the scheduled monitoring jobs.
func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(ScanEvery),
			func() (river.JobArgs, *river.InsertOpts) { return ScanArgs{}, nil }, &river.PeriodicJobOpts{ID: "aml.scan"}),
	}
}
