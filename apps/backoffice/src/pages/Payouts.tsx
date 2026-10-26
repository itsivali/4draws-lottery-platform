import { useState } from 'react'
import { useSession } from '../context'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, PageHeader, Pill } from '../components/ui'
import { eat, kes } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { Payout } from '../types'

const states = [
  ['held', 'Held for approval'],
  ['manual', 'Needs manual settlement'],
  ['timed_out', 'Timed out'],
  ['failed', 'Failed'],
  ['succeeded', 'Paid'],
] as const

export function PayoutsPage() {
  const { api, me } = useSession()
  const [state, setState] = useState<string>('held')
  const load = useLoad(() => api.get<{ payouts: Payout[] }>('/v1/staff/payouts', { state }), [state])
  const { ask, busy, view } = useConfirmedAction(load.reload)
  const operator = can(me.roles, 'payout_operator')
  return (
    <section>
      <PageHeader title="Payouts">
        Large withdrawals wait for approval. A payout whose M-Pesa answer was unclear is settled from the M-Pesa statement.
      </PageHeader>
      <div className="tabs" role="tablist" aria-label="Payout state">
        {states.map(([s, label]) => (
          <button key={s} type="button" role="tab" aria-selected={state === s} className={state === s ? 'tab active' : 'tab'} onClick={() => { setState(s) }}>
            {label}
          </button>
        ))}
      </div>
      {view}
      <Loaded load={load} what="payouts">
        {({ payouts }) =>
          payouts.length === 0 ? <Empty>Nothing here.</Empty> : (
            <table>
              <caption className="sr-only">Payouts</caption>
              <thead><tr><th scope="col">Payout</th><th scope="col">Amount</th><th scope="col">State</th><th scope="col">Created</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead>
              <tbody>
                {payouts.map((p) => (
                  <tr key={p.id}>
                    <td>{p.kind}<br /><span className="mono muted">{p.id}</span></td>
                    <td>{kes(p.net_kes)} to M-Pesa{p.tax_kes > 0 ? <div className="muted">tax {kes(p.tax_kes)}</div> : null}</td>
                    <td><Pill tone={p.status === 'paid' ? 'success' : p.status === 'failed' ? 'error' : 'pending'}>{p.state}</Pill>{p.reason ? <div className="muted">{p.reason}</div> : null}</td>
                    <td>{eat(p.created_at)}</td>
                    <td className="actions">
                      {operator && p.state === 'held' ? (
                        <>
                          <button type="button" className="primary" disabled={busy} onClick={() => {
                            ask({ title: `Approve ${kes(p.net_kes)}?`, body: <p>It is sent through M-Pesa now.</p>, confirmLabel: 'Approve and send', success: 'Approved; it is being sent.',
                              act: () => api.post(`/v1/staff/payouts/${p.id}/approve`) })
                          }}>Approve</button>
                          <button type="button" className="danger-outline" disabled={busy} onClick={() => {
                            ask({ title: `Reject ${kes(p.net_kes)}?`, body: <p>A withdrawal’s amount returns to the player’s wallet.</p>,
                              fields: [{ name: 'reason', label: 'Reason', required: true }], confirmLabel: 'Reject', danger: true, success: 'Rejected.',
                              act: (v) => api.post(`/v1/staff/payouts/${p.id}/reject`, { reason: v.reason }) })
                          }}>Reject</button>
                        </>
                      ) : null}
                      {operator && p.state === 'manual' ? (
                        <>
                          <button type="button" className="primary" disabled={busy} onClick={() => {
                            ask({ title: 'Paid according to the statement?', body: <p>Record the M-Pesa receipt from the statement.</p>,
                              fields: [{ name: 'receipt', label: 'M-Pesa receipt', required: true }], confirmLabel: 'Record as paid', success: 'Recorded as paid.',
                              act: (v) => api.post(`/v1/staff/payouts/${p.id}/settle`, { paid: true, receipt: v.receipt }) })
                          }}>Paid</button>
                          <button type="button" className="danger-outline" disabled={busy} onClick={() => {
                            ask({ title: 'Not paid according to the statement?', body: <p>A withdrawal’s amount returns to the wallet; a prize returns to its pool.</p>,
                              fields: [{ name: 'reason', label: 'What the statement shows', required: true }], confirmLabel: 'Record as not paid', danger: true, success: 'Recorded as not paid.',
                              act: (v) => api.post(`/v1/staff/payouts/${p.id}/settle`, { paid: false, reason: v.reason }) })
                          }}>Not paid</button>
                        </>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )
        }
      </Loaded>
    </section>
  )
}
