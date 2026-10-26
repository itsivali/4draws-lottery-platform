import { useSession } from '../context'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, PageHeader } from '../components/ui'
import { eat, msisdn } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { Closure } from '../types'

export function ClosuresPage() {
  const { api, me } = useSession()
  const load = useLoad(() => api.get<{ closures: Closure[] }>('/v1/staff/closures'))
  const { ask, busy, view } = useConfirmedAction(load.reload)
  return (
    <section>
      <PageHeader title="Account closures">
        An account closes only when no money is in it or moving: the balance must be paid out, and every top-up and payout must be final.
        The player’s records are kept as the law requires.
      </PageHeader>
      {view}
      <Loaded load={load} what="closure requests">
        {({ closures }) =>
          closures.length === 0 ? <Empty>No open requests.</Empty> : (
            <table>
              <caption className="sr-only">Open closure requests, oldest first</caption>
              <thead><tr><th scope="col">Phone</th><th scope="col">Account</th><th scope="col">Requested</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead>
              <tbody>
                {closures.map((c) => (
                  <tr key={c.user_id}>
                    <td>{msisdn(c.msisdn)}<div className="muted mono">{c.user_id}</div></td>
                    <td>{c.status}</td>
                    <td>{eat(c.requested_at)}</td>
                    <td className="actions">
                      {can(me.roles, 'support') ? (
                        <button type="button" className="danger-outline" disabled={busy} onClick={() => {
                          ask({ title: `Close the account of ${msisdn(c.msisdn)}?`, body: <p>Sign-in, play, top-ups and withdrawals stop and every session ends. This cannot be undone here.</p>,
                            confirmLabel: 'Close account', danger: true, success: 'Account closed.', act: () => api.post(`/v1/staff/closures/${c.user_id}/close`) })
                        }}>Close account</button>
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
