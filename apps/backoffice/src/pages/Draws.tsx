import { useSession } from '../context'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, PageHeader, Pill, type Tone } from '../components/ui'
import { eat, kes } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { Draw, Late } from '../types'

const statusTone: Record<Draw['status'], [Tone, string]> = {
  open: ['neutral', 'Open'],
  frozen: ['pending', 'Frozen · needs 2 signatures'],
  signed: ['pending', 'Signed · awaiting the round'],
  conducted: ['pending', 'Conducted · needs 2 approvals'],
  approved: ['success', 'Approved'],
  paid: ['success', 'Paid'],
  voided: ['error', 'Voided'],
}

function count(d: Draw, step: 'sign' | 'approve') {
  return d.signoffs.filter((s) => s.step === step).length
}

export function DrawsPage() {
  const { api, me } = useSession()
  const draws = useLoad(() => api.get<{ draws: Draw[] }>('/v1/staff/draws', { limit: '40' }))
  const overdue = useLoad(() => api.get<{ overdue: Late[] }>('/v1/staff/draws/overdue'))
  const reload = () => { draws.reload(); overdue.reload() }
  const { ask, busy, view } = useConfirmedAction(reload)
  const official = can(me.roles, 'draw_official')
  return (
    <section>
      <PageHeader title="Draws">
        A frozen draw needs two different officials to sign before its drand round; a conducted draw needs two to approve.
        You cannot sign the same step twice.
      </PageHeader>
      {view}
      <Loaded load={overdue} what="late draws">
        {({ overdue: late }) =>
          late.length === 0 ? null : (
            <div className="notice notice-error" role="alert">
              <p><strong>{late.length} draw step{late.length > 1 ? 's are' : ' is'} late</strong></p>
              <ul>{late.map((l) => <li key={l.draw_id + l.problem}>{l.draw_type} {l.period_start}: {l.problem} (due {eat(l.due)})</li>)}</ul>
            </div>
          )
        }
      </Loaded>
      <Loaded load={draws} what="draws">
        {({ draws: list }) =>
          list.length === 0 ? <Empty>No draws yet.</Empty> : (
            <table>
              <caption className="sr-only">Draws, newest first</caption>
              <thead><tr><th scope="col">Draw</th><th scope="col">Status</th><th scope="col">Entries</th><th scope="col">Results</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead>
              <tbody>
                {list.map((d) => {
                  const [tone, label] = statusTone[d.status]
                  return (
                    <tr key={d.id}>
                      <td><strong>{d.draw_type}</strong><br /><span className="muted">{d.period_start}{d.period_end !== d.period_start ? ` – ${d.period_end}` : ''}</span>{d.basis === 'personal' ? <><br /><span className="muted">players whose own {d.draw_type.replace('ly', '')} ends {d.period_end}</span></> : null}</td>
                      <td>
                        <Pill tone={tone}>{label}</Pill>
                        {d.status === 'frozen' ? <div className="muted">{count(d, 'sign')} of 2 signatures</div> : null}
                        {d.status === 'conducted' ? <div className="muted">{count(d, 'approve')} of 2 approvals</div> : null}
                        {d.voided_reason ? <div className="muted">{d.voided_reason}</div> : null}
                      </td>
                      <td>{d.status === 'open' ? '—' : d.entries}</td>
                      <td>{d.results.length === 0 ? '—' : d.results.map((r) => `#${r.rank} entry ${r.entry_no}: ${kes(r.amount_kes)}`).join('; ')}</td>
                      <td className="actions">
                        {official && d.status === 'frozen' ? (
                          <button type="button" className="primary" disabled={busy} onClick={() => {
                            ask({
                              title: `Sign the ${d.draw_type} draw of ${d.period_start}?`,
                              body: <p>You attest to the entry list hash, the seed commitment and the drand round fixed at freeze. A second official must also sign.</p>,
                              confirmLabel: 'Sign',
                              success: 'Signature recorded.',
                              act: () => api.post(`/v1/staff/draws/${d.id}/sign`),
                            })
                          }}>Sign</button>
                        ) : null}
                        {official && d.status === 'conducted' ? (
                          <button type="button" className="primary" disabled={busy} onClick={() => {
                            ask({
                              title: `Approve the results of the ${d.draw_type} draw of ${d.period_start}?`,
                              body: <p>Approval by two officials makes the results final and funds the prizes.</p>,
                              confirmLabel: 'Approve',
                              success: 'Approval recorded.',
                              act: () => api.post(`/v1/staff/draws/${d.id}/approve`),
                            })
                          }}>Approve</button>
                        ) : null}
                        {official && (d.status === 'frozen' || d.status === 'signed') ? (
                          <button type="button" className="danger-outline" disabled={busy} onClick={() => {
                            ask({
                              title: `Void the ${d.draw_type} draw of ${d.period_start}?`,
                              body: <p>The draw is cancelled before its round and re-run as a new draw with a new seed.</p>,
                              fields: [{ name: 'reason', label: 'Reason', required: true }],
                              confirmLabel: 'Void and re-run',
                              danger: true,
                              success: 'The draw was voided and re-run.',
                              act: (v) => api.post(`/v1/staff/draws/${d.id}/void`, { reason: v.reason }),
                            })
                          }}>Void</button>
                        ) : null}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )
        }
      </Loaded>
    </section>
  )
}
