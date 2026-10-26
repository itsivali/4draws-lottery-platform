import { useState } from 'react'
import { useSession } from '../context'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, PageHeader, Pill } from '../components/ui'
import { eat } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { Alert } from '../types'

export function AlertsPage() {
  const { api, me } = useSession()
  const [all, setAll] = useState(false)
  const load = useLoad(() => api.get<{ alerts: Alert[] }>('/v1/staff/alerts', all ? { all: 'true' } : {}), [all])
  const { ask, busy, view } = useConfirmedAction(load.reload)
  return (
    <section>
      <PageHeader title="Alerts">Acknowledging stops reminders; acknowledging a failed reconciliation lifts the payout freeze it caused.</PageHeader>
      <label className="check"><input type="checkbox" checked={all} onChange={(e) => { setAll(e.target.checked) }} /> <span>Show resolved alerts too</span></label>
      {view}
      <Loaded load={load} what="alerts">
        {({ alerts }) =>
          alerts.length === 0 ? <Empty>No open alerts.</Empty> : (
            <ul className="cards">
              {alerts.map((a) => (
                <li key={a.id} className="card">
                  <div className="card-head">
                    <h2>{a.summary}</h2>
                    {a.freezes_payouts && !a.acknowledged_at ? <Pill tone="error">Payouts frozen</Pill> : null}
                  </div>
                  <p className="muted">{a.kind} · {a.subject} · {a.severity} · seen {a.occurrences}× since {eat(a.first_seen_at)}</p>
                  {a.acknowledged_at ? (
                    <p><Pill tone="success">Acknowledged</Pill> {eat(a.acknowledged_at)} — {a.ack_note}</p>
                  ) : can(me.roles, 'alerts_operator') ? (
                    <button type="button" className="primary" disabled={busy} onClick={() => {
                      ask({ title: 'Acknowledge this alert?', body: <p>{a.summary}</p>, fields: [{ name: 'note', label: 'What you did or found', required: true }],
                        confirmLabel: 'Acknowledge', success: 'Acknowledged.', act: (v) => api.post(`/v1/staff/alerts/${a.id}/acknowledge`, { note: v.note }) })
                    }}>Acknowledge</button>
                  ) : null}
                </li>
              ))}
            </ul>
          )
        }
      </Loaded>
    </section>
  )
}
