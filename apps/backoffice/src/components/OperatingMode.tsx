import { CirclePause, CirclePlay, ShieldCheck } from 'lucide-react'
import { useSession } from '../context'
import { eat } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { OperatingState } from '../types'
import { useConfirmedAction } from './useConfirmedAction'
import { Loaded, Mono, Pill } from './ui'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

const modeWords = {
  disabled: { label: 'Stopped', tone: 'error', what: 'No top-up, play, withdrawal or payout starts. Money already moving is still accounted for.' },
  sandbox: { label: 'Sandbox', tone: 'warning', what: 'A demonstration with sandbox money: no real stakes or prizes.' },
  live: { label: 'Live', tone: 'success', what: 'Real money, under the licence recorded below.' },
} as const

/**
 * The operating mode (ADR 0027): whether money moves and whether it is real. Anyone on staff can stop it at once;
 * an admin resumes. Going live needs a request by one admin and an approval by another.
 */
export function OperatingMode() {
  const { api, me } = useSession()
  const load = useLoad((signal) => api.get<OperatingState>('/v1/staff/operating', {}, { signal }), [])
  const { ask, busy, view } = useConfirmedAction(load.reload)
  const admin = can(me.roles, 'admin')
  return (
    <Card className="shadow-card">
      <CardHeader>
        <CardTitle className="text-lg">Operating mode</CardTitle>
        <CardDescription>Licensing is required before real money moves. Stopping takes one person; going live takes two admins and the licence evidence.</CardDescription>
      </CardHeader>
      <CardContent>
        {view}
        <Loaded load={load} what="the operating mode">
          {(o) => {
            const m = modeWords[o.status.mode]
            const last = o.status.last_change
            return (
              <div className="grid gap-4">
                <div className="flex flex-wrap items-center gap-3">
                  <Pill tone={m.tone}>{m.label}</Pill>
                  <span className="text-sm text-muted-foreground">{m.what}</span>
                </div>
                {o.status.configured !== o.status.mode ? (
                  <p className="m-0 text-sm">The deployment allows <strong>{modeWords[o.status.configured].label.toLowerCase()}</strong>; what is recorded here keeps it {m.label.toLowerCase()}.</p>
                ) : null}
                {last ? (
                  <p className="m-0 text-sm text-muted-foreground">
                    Last change: {modeWords[last.mode].label.toLowerCase()} by <Mono>{last.changed_by}</Mono>, {eat(last.at)} — “{last.reason}”
                  </p>
                ) : null}
                <div className="flex flex-wrap gap-2">
                  {o.status.mode !== 'disabled' ? (
                    <Button type="button" variant="destructive" disabled={busy} onClick={() => {
                      ask({ title: 'Stop all money operations?', body: <p>Top-ups, plays, withdrawals and payouts are refused until an admin resumes them. Money already moving is still accounted for.</p>,
                        fields: [{ name: 'reason', label: 'Why', required: true }], confirmLabel: 'Stop now', danger: true, success: 'Stopped.',
                        act: (v) => api.post('/v1/staff/operating/stop', { reason: v.reason }) })
                    }}><CirclePause aria-hidden="true" /> Stop money operations</Button>
                  ) : null}
                  {o.status.mode === 'disabled' && admin && o.status.configured === 'sandbox' ? (
                    <Button type="button" disabled={busy} onClick={() => {
                      ask({ title: 'Resume in sandbox?', body: <p>Top-ups, plays and withdrawals start again, with sandbox money.</p>,
                        fields: [{ name: 'reason', label: 'What was found', required: true }], confirmLabel: 'Resume', success: 'Resumed in sandbox.',
                        act: (v) => api.post('/v1/staff/operating/resume', { mode: 'sandbox', reason: v.reason }) })
                    }}><CirclePlay aria-hidden="true" /> Resume (sandbox)</Button>
                  ) : null}
                  {admin && o.status.configured === 'live' && o.status.mode !== 'live' ? (
                    <Button type="button" variant="outline" disabled={busy} onClick={() => {
                      ask({ title: 'Request to operate live', body: <p>A second admin must approve. Record where the licence and the regulator’s approval are kept, and the SHA-256 of the evidence file.</p>,
                        fields: [
                          { name: 'licence_reference', label: 'Licence reference (as configured)', required: true },
                          { name: 'approval_reference', label: 'Regulator approval reference', required: true },
                          { name: 'evidence_sha256', label: 'SHA-256 of the evidence', required: true },
                          { name: 'evidence_location', label: 'Where the evidence is kept', required: true },
                          { name: 'reason', label: 'Reason', required: true },
                        ],
                        confirmLabel: 'Record the request', success: 'Recorded; a second admin must approve it.',
                        act: (v) => api.post('/v1/staff/operating/live-requests', v) })
                    }}><ShieldCheck aria-hidden="true" /> Request to go live</Button>
                  ) : null}
                </div>
                {o.live_requests.length > 0 ? (
                  <div>
                    <h3 className="mb-2 text-sm font-semibold">Waiting for a second admin</h3>
                    <ul className="m-0 grid list-none gap-2 p-0">
                      {o.live_requests.map((r) => {
                        const own = r.requested_by === `staff:${me.id}`
                        return (
                          <li key={r.id} className="rounded-lg border p-3 text-sm">
                            <div>Licence <strong>{r.licence_reference}</strong>, approval {r.approval_reference}</div>
                            <div className="text-muted-foreground">Requested by <Mono>{r.requested_by}</Mono>, {eat(r.at)}; evidence at {r.evidence_location} (<Mono>{r.evidence_sha256.slice(0, 12)}…</Mono>)</div>
                            {admin && !own ? (
                              <Button type="button" size="sm" className="mt-2" disabled={busy} onClick={() => {
                                ask({ title: `Go live under licence ${r.licence_reference}?`, body: <p>Real money moves from now on. Check the evidence yourself before approving.</p>,
                                  confirmLabel: 'Approve and go live', danger: true, success: 'Live.',
                                  act: () => api.post(`/v1/staff/operating/live-requests/${r.id}/approve`) })
                              }}>Approve</Button>
                            ) : own ? <p className="m-0 mt-1 text-muted-foreground">Another admin must approve your request.</p> : null}
                          </li>
                        )
                      })}
                    </ul>
                  </div>
                ) : null}
              </div>
            )
          }}
        </Loaded>
      </CardContent>
    </Card>
  )
}
