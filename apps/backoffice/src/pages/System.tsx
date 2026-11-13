import { CircleCheck, CircleX, RefreshCw } from 'lucide-react'
import { Link } from 'react-router-dom'
import { JobsPanel } from '../components/Jobs'
import { OperatingMode } from '../components/OperatingMode'
import { Switches } from '../components/Switches'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { infoOf } from '../jobs'
import { can } from '../roles'
import { Loaded, PageHeader, Pill, SectionTitle } from '../components/ui'
import { useSession } from '../context'
import { eat, kes } from '../format'
import { useLoad } from '../hooks'
import type { SystemState } from '../types'

const channelNames: Record<string, string> = { webhook: 'Webhook', email: 'Email' }
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

function On({ on, yes = 'On', no = 'Off' }: { on: boolean; yes?: string; no?: string }) {
  return on ? <Pill tone="success">{yes}</Pill> : <Pill tone="neutral">{no}</Pill>
}

function Row({ label, children, help }: { label: string; children: React.ReactNode; help?: string | undefined }) {
  return (
    <div className="grid gap-1 border-t py-3 first:border-t-0 first:pt-0 sm:grid-cols-[14rem_1fr] sm:items-center">
      <dt className="text-sm font-medium">{label}</dt>
      <dd className="m-0 flex flex-wrap items-center gap-2 text-sm">{children}{help ? <span className="text-muted-foreground">{help}</span> : null}</dd>
    </div>
  )
}

function minutes(s: number): string {
  if (s === 0) return 'none'
  return s % 60 === 0 ? `${s / 60} min` : `${s} s`
}

/** The system's state: what is configured, the background jobs, the reconciliations, and the counts that matter. */
export function SystemPage() {
  const { api, me } = useSession()
  const load = useLoad((signal) => api.get<SystemState>('/v1/staff/system', {}, { signal }), [])
  const { ask, busy, view } = useConfirmedAction(load.reload)
  const admin = can(me.roles, 'admin')
  return (
    <section>
      <PageHeader title="System status" actions={<Button type="button" variant="outline" size="sm" onClick={load.reload} disabled={load.loading}><RefreshCw aria-hidden="true" /> Refresh</Button>}>
        How the platform is set up and running right now. Settings change in the hosting dashboard, not here; no secret
        is shown.
      </PageHeader>
      <div className="mb-6 grid gap-5"><OperatingMode /><Switches /></div>
      <Loaded load={load} what="the system status">
        {(s) => {
          const st = s.settings
          const failing = s.failed_jobs.length
          const reconBad = s.reconciliations.filter((r) => r.ok === false).length
          return (
            <>
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                {[
                  { label: 'Open alerts', value: s.open_alerts, tone: s.open_alerts > 0 ? 'text-destructive' : '', to: '/alerts' },
                  { label: 'Top-ups in progress', value: s.top_ups_pending, tone: '', to: '/topups' },
                  { label: 'Unconfirmed top-ups', value: s.top_ups_unconfirmed, tone: s.top_ups_unconfirmed > 0 ? 'text-warning' : '', to: '/topups?status=unconfirmed' },
                  { label: 'Receipts from players to check', value: s.receipt_claims, tone: s.receipt_claims > 0 ? 'text-warning' : '', to: '/topups?claimed=true' },
                  { label: 'Plays today', value: s.plays_today, tone: '', to: '/plays' },
                  { label: 'Players', value: s.players, tone: '', to: '/players' },
                ].map((t) => (
                  <Card key={t.label} className="shadow-card">
                    <CardHeader className="pb-2"><CardDescription>{t.label}</CardDescription></CardHeader>
                    <CardContent><Link to={t.to} className={`font-display text-3xl font-bold tabular-nums no-underline hover:underline ${t.tone}`}>{t.value.toLocaleString('en-KE')}</Link></CardContent>
                  </Card>
                ))}
              </div>

              <SectionTitle>Health</SectionTitle>
              <Card className="shadow-card">
                <CardContent className="pt-6">
                  <dl className="m-0">
                    <Row label="Background jobs">{failing === 0 ? <Pill tone="success"><CircleCheck aria-hidden="true" className="size-3" /> No failing jobs</Pill> : <Pill tone="error"><CircleX aria-hidden="true" className="size-3" /> {failing} failing</Pill>}</Row>
                    <Row label="Reconciliations">{reconBad === 0 ? <Pill tone="success">All balanced at their last run</Pill> : <Pill tone="error">{reconBad} found mismatches</Pill>}</Row>
                    {s.reconciliations.map((r) => (
                      <Row key={r.area} label={`  ${r.area[0]?.toUpperCase() ?? ''}${r.area.slice(1)}`}>
                        {r.ran_at === null ? <Pill tone="neutral">Never run</Pill> : <><Pill tone={r.ok ? 'success' : 'error'}>{r.ok ? 'Balanced' : 'Mismatch'}</Pill><span className="text-muted-foreground">{eat(r.ran_at)}</span></>}
                      </Row>
                    ))}
                    <Row label="Server time"><span className="tabular-nums">{eat(s.now)}</span></Row>
                    <Row label="Running since"><span className="tabular-nums">{eat(st.started_at)}</span>{st.version ? <span className="font-mono text-xs text-muted-foreground">{st.version.slice(0, 7)}</span> : null}</Row>
                  </dl>
                </CardContent>
              </Card>

              <SectionTitle>Settings</SectionTitle>
              <div className="grid gap-4 lg:grid-cols-2">
                <Card className="shadow-card">
                  <CardHeader><CardTitle className="text-lg">Money</CardTitle></CardHeader>
                  <CardContent>
                    <dl className="m-0">
                      <Row label="M-Pesa (Daraja)">
                        {st.daraja === '' ? <Pill tone="error">Top-ups off</Pill> : st.daraja === 'production' ? <Pill tone="success">Production</Pill> : <Pill tone="warning">{st.daraja === 'sandbox' ? 'Sandbox' : st.daraja}</Pill>}
                      </Row>
                      <Row label="Sandbox settlement" help={st.sandbox_settlement ? 'settled from Safaricom’s callback; only real payments credit' : undefined}><On on={st.sandbox_settlement} /></Row>
                      <Row label="Smallest top-up">{st.top_up_min_kes > 0 ? kes(st.top_up_min_kes) : 'default'}</Row>
                      <Row label="Top-up cooldown">{minutes(st.top_up_cooldown_s)}</Row>
                      <Row label="Prompt pays">{st.buy_goods ? 'a till (Buy Goods)' : 'the paybill'}</Row>
                      <Row label="Paybill payments"><On on={st.paybill} /></Row>
                      <Row label="Withdrawals (B2C)"><On on={st.payouts} /></Row>
                    </dl>
                  </CardContent>
                </Card>
                <Card className="shadow-card">
                  <CardHeader><CardTitle className="text-lg">Game and players</CardTitle></CardHeader>
                  <CardContent>
                    <dl className="m-0">
                      <Row label="Price of a play">{st.play_price_kes === null ? '—' : kes(st.play_price_kes)}</Row>
                      <Row label="Wheel">{st.wheel_slots === null ? '—' : `${st.wheel_slots} slots, ${st.wheel_lucky_slots ?? 0} lucky`}</Row>
                      <Row label="Google sign-in"><On on={st.google_sign_in} /></Row>
                      <Row label="Identity checks">{st.kyc_registry ? <Pill tone="success">Automatic registry</Pill> : <Pill tone="neutral">Manual review</Pill>}</Row>
                      <Row label="Device attestation"><span className="capitalize">{st.attestation || 'record'}</span></Row>
                    </dl>
                  </CardContent>
                </Card>
                <Card className="shadow-card">
                  <CardHeader><CardTitle className="text-lg">Operations</CardTitle></CardHeader>
                  <CardContent>
                    <dl className="m-0">
                      <Row label="Email">{st.email ? <Pill tone="success">Sending</Pill> : <Pill tone="error">Not configured</Pill>}</Row>
                      <Row label="Alerts go to">{st.alerts_delivery === 'log' || !st.alerts_delivery ? <Pill tone="warning">Logs only</Pill> : (
                        <span className="flex flex-wrap gap-1">{st.alerts_delivery.split(',').map((c) => c.trim()).filter(Boolean).map((c) => <Pill key={c} tone="success">{channelNames[c] ?? c}</Pill>)}</span>
                      )}</Row>
                      <Row label="Heartbeat"><On on={st.heartbeat} yes="Pinged" no="Not set" /></Row>
                      <Row label="Workers"><span className="capitalize">{st.workers}</span></Row>
                    </dl>
                  </CardContent>
                </Card>
              </div>

              <SectionTitle>Background jobs</SectionTitle>
              <p className="-mt-1 mb-4 max-w-[75ch] text-[14px] leading-relaxed text-muted-foreground">The work the platform does on its own: sending M-Pesa prompts and payouts, checking payments, running draws and the monitoring rules. Counts cover jobs waiting now and those finished in the last day.</p>
              {view}
              <JobsPanel s={s} actions={admin ? (j) => (
                <>
                  {j.state === 'retryable' ? (
                    <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => {
                      ask({ title: `Cancel this “${infoOf(j.kind).name}” job?`, confirmLabel: 'Cancel the job', success: 'Cancelled.', danger: true,
                        body: <p>It stops retrying. Its work is not lost: the area’s sweep picks it up again, or retry it here.</p>,
                        act: () => api.post(`/v1/staff/jobs/${String(j.id)}/cancel`) })
                    }}>Cancel</Button>
                  ) : null}
                  <Button type="button" size="sm" disabled={busy} onClick={() => {
                    ask({ title: `Run “${infoOf(j.kind).name}” again now?`, confirmLabel: 'Retry now', success: 'Queued to run again.',
                      body: <p>The job runs again at once. It checks its record first, so nothing is paid or credited twice.</p>,
                      act: () => api.post(`/v1/staff/jobs/${String(j.id)}/retry`) })
                  }}>Retry now</Button>
                </>
              ) : undefined} />
            </>
          )
        }}
      </Loaded>
    </section>
  )
}
