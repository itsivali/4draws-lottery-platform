import { Activity, CircleCheck, CircleX, RefreshCw } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Loaded, PageHeader, Pill, SectionTitle, type Tone } from '../components/ui'
import { useSession } from '../context'
import { eat, kes } from '../format'
import { useLoad } from '../hooks'
import type { SystemState } from '../types'
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

const jobTone: Record<string, Tone> = { available: 'pending', running: 'pending', scheduled: 'neutral', retryable: 'warning', discarded: 'error', completed: 'success', cancelled: 'neutral' }

function minutes(s: number): string {
  if (s === 0) return 'none'
  return s % 60 === 0 ? `${s / 60} min` : `${s} s`
}

/** The system's state: what is configured, the background jobs, the reconciliations, and the counts that matter. */
export function SystemPage() {
  const { api } = useSession()
  const load = useLoad((signal) => api.get<SystemState>('/v1/staff/system', {}, { signal }), [])
  return (
    <section>
      <PageHeader title="System status" actions={<Button type="button" variant="outline" size="sm" onClick={load.reload} disabled={load.loading}><RefreshCw aria-hidden="true" /> Refresh</Button>}>
        How the platform is set up and running right now. Settings change in the hosting dashboard, not here; no secret
        is shown.
      </PageHeader>
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
                      <Row label="Sandbox settlement" help={st.sandbox_settlement ? 'top-ups credit without M-Pesa’s confirmation' : undefined}><On on={st.sandbox_settlement} /></Row>
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
                      <Row label="Apple sign-in"><On on={st.apple_sign_in} /></Row>
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
                      <Row label="Alerts go to">{st.alerts_delivery === 'webhook' ? <Pill tone="success">Webhook</Pill> : <Pill tone="warning">Logs only</Pill>}</Row>
                      <Row label="Heartbeat"><On on={st.heartbeat} yes="Pinged" no="Not set" /></Row>
                      <Row label="Workers"><span className="capitalize">{st.workers}</span></Row>
                    </dl>
                  </CardContent>
                </Card>
              </div>

              <SectionTitle>Background jobs</SectionTitle>
              <Card className="shadow-card">
                <CardContent className="pt-6">
                  {s.jobs.length === 0 ? <p className="m-0 flex items-center gap-2 text-sm text-muted-foreground"><Activity aria-hidden="true" className="size-4" /> No jobs in the last day.</p> : (
                    <table className="w-full text-sm">
                      <caption className="sr-only">Background jobs by kind and state (the last day for completed ones)</caption>
                      <thead><tr className="text-left text-muted-foreground"><th className="pb-2 font-medium">Kind</th><th className="pb-2 font-medium">State</th><th className="pb-2 text-right font-medium">Jobs</th></tr></thead>
                      <tbody>
                        {s.jobs.map((j) => (
                          <tr key={`${j.kind}:${j.state}`} className="border-t">
                            <td className="py-2 font-mono text-xs">{j.kind}</td>
                            <td className="py-2"><Pill tone={jobTone[j.state] ?? 'neutral'}>{j.state}</Pill></td>
                            <td className="py-2 text-right tabular-nums">{j.count.toLocaleString('en-KE')}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  )}
                  {failing > 0 ? (
                    <>
                      <h3 className="mb-2 mt-6 text-sm font-semibold">Failing jobs</h3>
                      <ul className="m-0 grid list-none gap-2 p-0">
                        {s.failed_jobs.map((j) => (
                          <li key={j.id} className="rounded-lg border p-3 text-sm">
                            <div className="flex flex-wrap items-center gap-2"><span className="font-mono text-xs">{j.kind}</span><Pill tone={jobTone[j.state] ?? 'neutral'}>{j.state}</Pill><span className="text-muted-foreground">attempt {j.attempt} · {eat(j.at)}</span></div>
                            {j.last_error ? <p className="m-0 mt-1 text-muted-foreground [overflow-wrap:anywhere]">{j.last_error}</p> : null}
                          </li>
                        ))}
                      </ul>
                    </>
                  ) : null}
                </CardContent>
              </Card>
            </>
          )
        }}
      </Loaded>
    </section>
  )
}
