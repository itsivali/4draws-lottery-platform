import { CircleAlert, CircleCheck, Clock, RotateCw, TriangleAlert } from 'lucide-react'
import type { ReactNode } from 'react'
import { eat } from '../format'
import { infoOf, jobAreas, jobInfo, relative, stateWords } from '../jobs'
import type { SystemState } from '../types'
import { Pill } from './ui'
import { cn } from '@/lib/utils'

type Failed = SystemState['failed_jobs'][number]

/**
 * The background jobs, grouped by what they look after: each kind by name, what it does, how often it runs, when it
 * last finished and next runs, and its jobs by state. Failing jobs follow with their last error.
 */
export function JobsPanel({ s, actions }: { s: SystemState; actions?: ((job: Failed) => ReactNode) | undefined }) {
  const now = new Date(s.now)
  const kinds = [...new Set([...Object.keys(jobInfo), ...s.jobs.map((j) => j.kind)])]
  const runs = new Map((s.job_runs ?? []).map((r) => [r.kind, r]))
  const counts = (kind: string) => s.jobs.filter((j) => j.kind === kind && j.count > 0)
  return (
    <div className="grid gap-5">
      {jobAreas.map((area) => {
        const here = kinds.filter((k) => infoOf(k).area === area)
        if (here.length === 0) return null
        return (
          <section key={area} aria-labelledby={`jobs-${area}`} className="overflow-hidden rounded-3xl border bg-card shadow-card animate-rise-in">
            <h3 id={`jobs-${area}`} className="m-0 border-b bg-muted/40 px-5 py-3 text-[12px] font-bold uppercase tracking-[0.12em] text-muted-foreground">{area}</h3>
            <ul className="m-0 grid list-none p-0">
              {here.map((kind) => {
                const info = infoOf(kind)
                const c = counts(kind)
                const run = runs.get(kind)
                const bad = c.some((j) => j.state === 'discarded')
                const warn = c.some((j) => j.state === 'retryable')
                const Icon = bad ? CircleAlert : warn ? TriangleAlert : CircleCheck
                return (
                  <li key={kind} className="grid gap-3 border-t px-5 py-4 first:border-t-0 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
                    <div className="flex min-w-0 gap-3">
                      <Icon aria-hidden="true" className={cn('mt-0.5 size-5 shrink-0', bad ? 'text-destructive' : warn ? 'text-warning' : 'text-success')} />
                      <div className="min-w-0">
                        <p className="m-0 flex flex-wrap items-center gap-x-2 gap-y-1">
                          <span className="text-[15px] font-semibold">{info.name}</span>
                          <span className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-[12px] font-medium text-muted-foreground">
                            {info.every ? <><RotateCw aria-hidden="true" className="size-3" /> {info.every}</> : 'When needed'}
                          </span>
                          <span className="sr-only">{bad ? '(has jobs that gave up)' : warn ? '(retrying)' : '(healthy)'}</span>
                        </p>
                        <p className="m-0 mt-0.5 text-[14px] leading-relaxed text-muted-foreground">{info.does}</p>
                        <p className="m-0 mt-1.5 flex flex-wrap gap-x-4 gap-y-1 text-[13px] text-muted-foreground">
                          <span className="inline-flex items-center gap-1"><Clock aria-hidden="true" className="size-3.5" />
                            {run?.last_completed_at ? <>Last finished <time dateTime={run.last_completed_at} title={eat(run.last_completed_at)} className="font-medium text-foreground">{relative(run.last_completed_at, now)}</time></> : 'Not run in the last day'}
                          </span>
                          {run?.next_at ? <span>Next <time dateTime={run.next_at} title={eat(run.next_at)} className="font-medium text-foreground">{relative(run.next_at, now)}</time></span> : null}
                          <span className="font-mono text-[12px] opacity-80">{kind}</span>
                        </p>
                      </div>
                    </div>
                    <div className="flex flex-wrap gap-1.5 md:justify-end">
                      {c.length === 0 ? <span className="text-[13px] text-muted-foreground">Nothing in the last day</span> : c.map((j) => {
                        const w = stateWords[j.state] ?? { label: j.state, tone: 'neutral' as const }
                        return <Pill key={j.state} tone={w.tone}><span className="tabular-nums font-bold">{j.count.toLocaleString('en-KE')}</span> {w.label}</Pill>
                      })}
                    </div>
                  </li>
                )
              })}
            </ul>
          </section>
        )
      })}
      {s.failed_jobs.length > 0 ? (
        <section aria-labelledby="jobs-failing" className="overflow-hidden rounded-3xl border border-destructive/30 bg-card shadow-card">
          <div className="px-5 pb-3 pt-5">
            <h3 id="jobs-failing" className="m-0 font-display text-lg font-bold tracking-tight">Failing jobs</h3>
            <p className="m-0 mt-1 text-[14px] text-muted-foreground">Jobs that are retrying or gave up, newest first, with the last error they hit.</p>
          </div>
          <ul className="m-0 grid list-none p-0">
            {s.failed_jobs.map((j) => {
              const w = stateWords[j.state] ?? { label: j.state, tone: 'neutral' as const }
              return (
                <li key={j.id} className="grid gap-2 border-t px-5 py-4">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-[15px] font-semibold">{infoOf(j.kind).name}</span>
                    <Pill tone={w.tone}>{w.label}</Pill>
                    <span className="text-[13px] text-muted-foreground">attempt {j.attempt} · {eat(j.at)} · job {j.id}</span>
                    {actions ? <span className="ml-auto flex gap-2">{actions(j)}</span> : null}
                  </div>
                  {j.last_error ? <pre className="m-0 max-h-40 overflow-auto whitespace-pre-wrap rounded-xl bg-muted/60 px-3 py-2 font-mono text-[12.5px] leading-relaxed text-foreground [overflow-wrap:anywhere]">{j.last_error}</pre> : null}
                </li>
              )
            })}
          </ul>
        </section>
      ) : null}
    </div>
  )
}
