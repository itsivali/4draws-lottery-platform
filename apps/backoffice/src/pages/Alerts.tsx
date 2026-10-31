import type { ColumnDef } from '@tanstack/react-table'
import { Bell, ExternalLink } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useSession } from '../context'
import { DataTable, Stack } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, Mono, PageHeader, Pill, type Tone } from '../components/ui'
import { eat } from '../format'
import { useLoad, usePages } from '../hooks'
import { can } from '../roles'
import type { Alert, AlertDetail, AlertKindCount, AlertState, AlertsPage } from '../types'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

const states: [AlertState, string][] = [['open', 'Open'], ['acknowledged', 'Acknowledged'], ['resolved', 'Resolved'], ['all', 'All']]
const severities: [string, string][] = [['', 'Any severity'], ['page', 'Pages'], ['warn', 'Warnings']]

/** What to do about each kind of alert, and where (docs/runbooks/alerts.md has the long form). */
interface Hint { what: string; to: (a: Alert) => string; label: string }
const hints: Record<string, Hint> = {
  'topup.unconfirmed': { what: 'M-Pesa may have taken the money but never confirmed it. Find the receipt on the M-Pesa statement, then credit the top-up from it or fail it.', to: (a) => `/topups?topup=${a.subject}`, label: 'Open the top-up' },
  'topup.overdue_pending': { what: 'The verify job that settles every pending top-up did not run. Check the background jobs, then the top-up: it settles on its own once the worker is back.', to: (a) => `/topups?topup=${a.subject}`, label: 'Open the top-up' },
  'topup.receipt_claim': { what: 'A player gave the receipt from their M-Pesa message and M-Pesa did not confirm it (or has not answered). Look for the receipt on the M-Pesa statement, then credit the top-up from it or close it as not on the statement.', to: (a) => `/topups?topup=${a.subject}`, label: 'Open the top-up' },
  'topup.orphan_callbacks': { what: 'Daraja sent callbacks for a checkout we never recorded: a push lost before it was saved, or a forgery. Compare with the M-Pesa statement; nothing was credited.', to: () => '/topups', label: 'Top-ups' },
  'paybill.suspense_open': { what: 'Paybill money arrived for an account that matches no player. Credit the right player or refund it; it stays in suspense until then.', to: () => '/payments', label: 'Suspense items' },
  'paybill.unverified': { what: 'Daraja could not confirm the paybill payment. Recheck it, or settle it from the M-Pesa statement.', to: () => '/payments', label: 'Paybill payments' },
  'payout.awaiting_approval': { what: 'A payout waits for an operator: approve or reject a held withdrawal, or settle a manual one from the M-Pesa statement.', to: () => '/payouts', label: 'Payouts' },
  'payout.failed': { what: 'The payout failed and the money is back in the wallet or the prize pool. Tell the player, and retry a prize once the cause is fixed.', to: () => '/payouts', label: 'Payouts' },
  'b2c.float_low': { what: 'The B2C float is below the floor: top it up with Safaricom before payouts stall.', to: () => '/finance', label: 'Finance' },
  'jobs.failing': { what: 'Background jobs are failing. Read the last error on the System page; a discarded job did not do its work and may need re-queuing by engineering.', to: () => '/system', label: 'System status' },
  'daraja.throttled': { what: 'Daraja is throttling the STK requests: top-ups confirm late until it eases. Nothing to fix here unless it persists; then ask Safaricom about the rate limit.', to: () => '/system', label: 'System status' },
  'kyc.queue_stale': { what: 'Identity submissions have waited too long: players cannot withdraw until someone decides. Review the queue.', to: () => '/kyc', label: 'Identity review' },
  'closure.waiting': { what: 'A player asked to close their account and nobody has. Refund what is left and close it.', to: () => '/closures', label: 'Closures' },
  'draw.overdue': { what: 'A draw is late for its next step: sign, approve or void it.', to: () => '/draws', label: 'Draws' },
  'reconciliation.failed': { what: 'The books do not balance, so no payout leaves. Find the mismatch in the run’s report, fix the cause, then acknowledge with what you found: that lifts the freeze.', to: () => '/system', label: 'System status' },
  'reconciliation.missing': { what: 'A daily reconciliation has not run: the worker is probably down. Check the background jobs.', to: () => '/system', label: 'System status' },
  'kra.remittance_due': { what: 'Withheld tax is nearing its remittance deadline: prepare the batch and remit it.', to: () => '/tax', label: 'Tax' },
  'kra.remittance_overdue': { what: 'Withheld tax is past its remittance deadline: remit it now.', to: () => '/tax', label: 'Tax' },
  'alerts.check_failed': { what: 'A check itself could not run, so nothing it watches is being watched. Read the error; it is usually the database or a missing table.', to: () => '/system', label: 'System status' },
}

export function hintFor(a: Alert): Hint {
  const h = hints[a.kind]
  if (h) return h
  const prefix = a.kind.split('.')[0] ?? ''
  const byPrefix: Record<string, Hint> = {
    topup: hints['topup.orphan_callbacks'] as Hint, paybill: hints['paybill.unverified'] as Hint, payout: hints['payout.awaiting_approval'] as Hint,
    jobs: hints['jobs.failing'] as Hint, kyc: hints['kyc.queue_stale'] as Hint, closure: hints['closure.waiting'] as Hint, draw: hints['draw.overdue'] as Hint,
  }
  return byPrefix[prefix] ?? { what: 'See the runbook (docs/runbooks/alerts.md) for this kind of alert.', to: () => '/system', label: 'System status' }
}

const severityTone: Record<string, Tone> = { page: 'error', warn: 'warning' }
const severityLabel: Record<string, string> = { page: 'Page', warn: 'Warning' }

function StatePill({ a }: { a: Alert }) {
  if (a.resolved_at) return <Pill tone="neutral">Resolved</Pill>
  if (a.acknowledged_at) return <Pill tone="success">Acknowledged</Pill>
  return <Pill tone={a.severity === 'page' ? 'error' : 'pending'}>Open</Pill>
}

function stringify(v: unknown): string {
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  if (v === null || v === undefined) return '—'
  return JSON.stringify(v)
}

/** Every alert, filtered by state, kind and severity, each opening in a sheet with what to do about it (ADR 0025). */
export function AlertsPage() {
  const { api, me } = useSession()
  const [params, setParams] = useSearchParams()
  const [state, setState] = useState<AlertState>('open')
  const [kind, setKind] = useState('')
  const [severity, setSeverity] = useState('')
  const [kinds, setKinds] = useState<AlertKindCount[]>([])
  const [selected, setSelected] = useState<string | null>(params.get('alert'))
  const load = usePages((before, signal) =>
    api.get<AlertsPage>('/v1/staff/alerts',
      { limit: '100', state, ...(kind ? { kind } : {}), ...(severity ? { severity } : {}), ...(before === null ? {} : { before }) }, { signal })
      .then((r) => { setKinds(r.kinds); return { rows: r.alerts, next: r.next_before } }), [state, kind, severity])

  const select = useCallback((id: string | null) => {
    setSelected(id)
    const next = new URLSearchParams(params)
    if (id) next.set('alert', id)
    else next.delete('alert')
    setParams(next, { replace: true })
  }, [params, setParams])

  const columns = useMemo<ColumnDef<Alert>[]>(() => [
    {
      id: 'what', header: 'Alert', accessorFn: (a) => `${a.summary} ${a.kind} ${a.subject}`,
      cell: ({ row }) => <Stack primary={<span className="max-w-xl [overflow-wrap:anywhere]">{row.original.summary}</span>} secondary={<span><Mono>{row.original.kind}</Mono> · <Mono>{row.original.subject}</Mono></span>} />,
    },
    {
      id: 'severity', header: 'Severity', accessorFn: (a) => a.severity,
      cell: ({ row }) => <div className="grid justify-items-start gap-1"><Pill tone={severityTone[row.original.severity] ?? 'neutral'}>{severityLabel[row.original.severity] ?? row.original.severity}</Pill>{row.original.freezes_payouts && !row.original.resolved_at ? <Pill tone="error">Payouts frozen</Pill> : null}</div>,
    },
    {
      id: 'state', header: 'State', accessorFn: (a) => a.resolved_at ? 'resolved' : a.acknowledged_at ? 'acknowledged' : 'open',
      cell: ({ row }) => {
        const a = row.original
        return (
          <div className="grid justify-items-start gap-1">
            <StatePill a={a} />
            {a.acknowledged_at ? <span className="text-sm text-muted-foreground">by {a.acknowledged_by ?? 'someone'} {eat(a.acknowledged_at)}</span> : null}
            {a.resolved_at ? <span className="text-sm text-muted-foreground">resolved by {a.resolved_by ?? 'system'} {eat(a.resolved_at)}</span> : null}
          </div>
        )
      },
    },
    {
      id: 'seen', header: 'Seen', accessorFn: (a) => a.first_seen_at,
      cell: ({ row }) => <Stack primary={<span className="tabular-nums font-normal">{eat(row.original.first_seen_at)}</span>} secondary={<span className="tabular-nums">last {eat(row.original.last_seen_at)} · {row.original.occurrences}×</span>} />,
    },
    {
      id: 'open', header: '', enableSorting: false,
      cell: ({ row }) => <div className="flex justify-end"><Button type="button" size="sm" variant="outline" onClick={() => { select(row.original.id) }}>Open</Button></div>,
    },
  ], [select])

  const openCount = kinds.reduce((n, k) => n + k.open, 0)
  return (
    <section>
      <PageHeader title="Alerts">
        What needs a person, newest first: money waiting for a decision and the platform’s health. Open one to see what
        was found and what to do. Acknowledging stops reminders; acknowledging a failed reconciliation lifts the payout
        freeze it caused.
      </PageHeader>
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <Tabs value={state} onValueChange={(v) => { setState(v as AlertState) }}>
          <TabsList aria-label="State" className="h-auto flex-wrap">
            {states.map(([s, l]) => <TabsTrigger key={s} value={s} className="cursor-pointer">{l}</TabsTrigger>)}
          </TabsList>
        </Tabs>
        <select aria-label="Kind" value={kind} onChange={(e) => { setKind(e.target.value) }}
          className="h-9 rounded-md border border-input bg-background px-3 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
          <option value="">Every kind{openCount > 0 ? ` (${openCount} open)` : ''}</option>
          {kinds.map((k) => <option key={k.kind} value={k.kind}>{k.kind}{k.open > 0 ? ` (${k.open} open)` : ''}</option>)}
        </select>
        <Tabs value={severity} onValueChange={setSeverity} className="ml-auto">
          <TabsList aria-label="Severity" className="h-auto flex-wrap">
            {severities.map(([s, l]) => <TabsTrigger key={s} value={s} className="cursor-pointer">{l}</TabsTrigger>)}
          </TabsList>
        </Tabs>
      </div>
      <Loaded load={load} what="alerts">
        {(alerts) =>
          alerts.length === 0 ? <Empty icon={Bell}>{kind || severity || state !== 'open' ? 'No alerts match.' : 'No open alerts.'}</Empty> : (
            <DataTable columns={columns} data={alerts} rowId={(a) => a.id} caption="Alerts, newest first" filter="Filter these rows" highlight={selected ?? undefined}
              more={{ has: load.more, loading: load.loadingMore, load: load.loadMore }} />
          )
        }
      </Loaded>
      <AlertSheet id={selected} onClose={() => { select(null) }} canAct={can(me.roles, 'staff')} onChanged={load.reload} />
    </section>
  )
}

/** One alert in a side sheet: what was found, what to do, who did what, and the acknowledge action. */
function AlertSheet({ id, onClose, canAct, onChanged }: { id: string | null; onClose: () => void; canAct: boolean; onChanged: () => void }) {
  const { api } = useSession()
  const [shown, setShown] = useState<string | null>(id) // keeps the content while the sheet slides shut
  if (id !== null && id !== shown) setShown(id)
  const load = useLoad((signal) => shown ? api.get<AlertDetail>(`/v1/staff/alerts/${shown}`, undefined, { signal }) : Promise.resolve(null), [shown])
  const { ask, busy, view } = useConfirmedAction(() => { load.reload(); onChanged() })
  return (
    <Sheet open={id !== null} onOpenChange={(open) => { if (!open) onClose() }}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
        <Loaded load={load} what="the alert">
          {(a) => a === null ? null : (() => {
            const hint = hintFor(a)
            const details = Object.entries(a.details ?? {})
            return (
              <>
                <SheetHeader>
                  <div className="flex flex-wrap gap-1"><Pill tone={severityTone[a.severity] ?? 'neutral'}>{severityLabel[a.severity] ?? a.severity}</Pill><StatePill a={a} />{a.freezes_payouts && !a.resolved_at ? <Pill tone="error">Payouts frozen</Pill> : null}</div>
                  <SheetTitle className="text-base leading-snug [overflow-wrap:anywhere]">{a.summary}</SheetTitle>
                  <SheetDescription><Mono>{a.kind}</Mono> · <Mono>{a.subject}</Mono> · raised by the {a.check || 'watch'} check</SheetDescription>
                </SheetHeader>
                <div className="grid gap-5 px-4 pb-6">
                  {view}
                  <section aria-labelledby="alert-what-to-do" className="rounded-xl border bg-muted/40 p-4">
                    <h3 id="alert-what-to-do" className="m-0 mb-1 text-sm font-semibold">What to do</h3>
                    <p className="m-0 text-sm">{hint.what}</p>
                    <Button asChild size="sm" variant="outline" className="mt-3"><Link to={hint.to(a)}>{hint.label} <ExternalLink aria-hidden="true" className="size-3.5" /></Link></Button>
                  </section>
                  {canAct && !a.acknowledged_at && !a.resolved_at ? (
                    <Button type="button" disabled={busy} onClick={() => {
                      ask({ title: 'Acknowledge this alert?', body: <p>{a.summary}</p>, fields: [{ name: 'note', label: 'What you did or found', required: true }],
                        confirmLabel: 'Acknowledge', success: 'Acknowledged.', act: (v) => api.post(`/v1/staff/alerts/${a.id}/acknowledge`, { note: v.note }) })
                    }}>Acknowledge</Button>
                  ) : null}
                  <section aria-labelledby="alert-details">
                    <h3 id="alert-details" className="m-0 mb-2 text-sm font-semibold">Details</h3>
                    {details.length === 0 ? <p className="m-0 text-sm text-muted-foreground">Nothing beyond the summary.</p> : (
                      <dl className="m-0 grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-sm">
                        {details.map(([k, v]) => <div key={k} className="contents"><dt className="text-muted-foreground">{k.replaceAll('_', ' ')}</dt><dd className="m-0 [overflow-wrap:anywhere]"><Mono>{stringify(v)}</Mono></dd></div>)}
                      </dl>
                    )}
                    <dl className="m-0 mt-3 grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-sm">
                      <dt className="text-muted-foreground">first seen</dt><dd className="m-0 tabular-nums">{eat(a.first_seen_at)}</dd>
                      <dt className="text-muted-foreground">last seen</dt><dd className="m-0 tabular-nums">{eat(a.last_seen_at)} ({a.occurrences}×, sent {a.notifications ?? 0}×)</dd>
                    </dl>
                  </section>
                  <section aria-labelledby="alert-history">
                    <h3 id="alert-history" className="m-0 mb-2 text-sm font-semibold">History</h3>
                    <ol className="m-0 grid list-none gap-2 p-0 text-sm">
                      {a.history.map((h, i) => (
                        <li key={i} className="grid gap-0.5">
                          <span><span className="font-medium">{h.event === 'alert.acknowledged' ? 'acknowledged' : h.event}</span> by {h.actor || 'system'} <span className="tabular-nums text-muted-foreground">{eat(h.at)}</span></span>
                          {h.note ? <span className="text-muted-foreground [overflow-wrap:anywhere]">{h.note}</span> : null}
                        </li>
                      ))}
                    </ol>
                  </section>
                </div>
              </>
            )
          })()}
        </Loaded>
      </SheetContent>
    </Sheet>
  )
}
