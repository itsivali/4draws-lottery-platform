import { Bell } from 'lucide-react'
import { useId, useState } from 'react'
import { useSession } from '../context'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, PageHeader, Pill } from '../components/ui'
import { eat } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { Alert } from '../types'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

export function AlertsPage() {
  const { api, me } = useSession()
  const [all, setAll] = useState(false)
  const id = useId()
  const load = useLoad((signal) => api.get<{ alerts: Alert[] }>('/v1/staff/alerts', all ? { all: 'true' } : {}, { signal }), [all])
  const { ask, busy, view } = useConfirmedAction(load.reload)
  return (
    <section>
      <PageHeader
        title="Alerts"
        actions={
          <div className="flex items-center gap-2">
            <Switch id={id} checked={all} onCheckedChange={setAll} />
            <Label htmlFor={id} className="font-normal">Show resolved alerts too</Label>
          </div>
        }
      >
        Acknowledging stops reminders; acknowledging a failed reconciliation lifts the payout freeze it caused.
      </PageHeader>
      {view}
      <Loaded load={load} what="alerts">
        {({ alerts }) =>
          alerts.length === 0 ? <Empty icon={Bell}>{all ? 'No alerts.' : 'No open alerts.'}</Empty> : (
            <ul className="m-0 grid list-none gap-4 p-0 lg:grid-cols-2">
              {alerts.map((a) => (
                <li key={a.id}>
                  <Card className="h-full shadow-card">
                    <CardHeader>
                      <CardTitle className="text-base leading-snug">{a.summary}</CardTitle>
                      <CardDescription>{a.kind} · {a.subject} · {a.severity} · seen {a.occurrences}× since {eat(a.first_seen_at)}</CardDescription>
                      {a.freezes_payouts && !a.acknowledged_at ? <CardAction><Pill tone="error">Payouts frozen</Pill></CardAction> : null}
                    </CardHeader>
                    <CardContent>
                      {a.acknowledged_at ? (
                        <p className="m-0 flex flex-wrap items-center gap-2 text-sm"><Pill tone="success">Acknowledged</Pill> <span className="text-muted-foreground">{eat(a.acknowledged_at)}</span> — {a.ack_note}</p>
                      ) : a.resolved_at ? (
                        <p className="m-0 flex flex-wrap items-center gap-2 text-sm"><Pill tone="neutral">Resolved</Pill> <span className="text-muted-foreground">{eat(a.resolved_at)}, before anyone acknowledged it</span></p>
                      ) : can(me.roles, 'alerts_operator') ? (
                        <Button type="button" size="sm" disabled={busy} onClick={() => {
                          ask({ title: 'Acknowledge this alert?', body: <p>{a.summary}</p>, fields: [{ name: 'note', label: 'What you did or found', required: true }],
                            confirmLabel: 'Acknowledge', success: 'Acknowledged.', act: (v) => api.post(`/v1/staff/alerts/${a.id}/acknowledge`, { note: v.note }) })
                        }}>Acknowledge</Button>
                      ) : <p className="m-0 text-sm text-muted-foreground">Not yet acknowledged.</p>}
                    </CardContent>
                  </Card>
                </li>
              ))}
            </ul>
          )
        }
      </Loaded>
    </section>
  )
}
