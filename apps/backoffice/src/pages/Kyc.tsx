import { useEffect } from 'react'
import { useSession } from '../context'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, PageHeader } from '../components/ui'
import { eat, msisdn } from '../format'
import { useLoad } from '../hooks'
import { useReviewQueue } from '../reviewQueue'
import type { KycSubmission } from '../types'

export function KycPage() {
  const { api } = useSession()
  const load = useLoad(() => api.get<{ submissions: KycSubmission[] }>('/v1/staff/kyc/pending'))
  const { ask, busy, view } = useConfirmedAction(load.reload)
  const { queue, dismiss } = useReviewQueue()
  useEffect(dismiss, [dismiss, queue])
  // the list is reloaded only when asked: every submission it shows is audited as unmasked
  const fresh = (queue?.waiting ?? 0) - (load.data?.submissions.length ?? Infinity)
  return (
    <section>
      <PageHeader title="Identity review">
        Compare each submission with the person’s national ID. Opening this list is recorded in the audit log.
      </PageHeader>
      {view}
      {fresh > 0 ? (
        <div className="notice notice-pending">
          <p>{fresh === 1 ? 'A new submission arrived.' : `${fresh} new submissions arrived.`}</p>
          <button type="button" onClick={load.reload}>Show {fresh} new {fresh === 1 ? 'submission' : 'submissions'}</button>
        </div>
      ) : null}
      <Loaded load={load} what="submissions">
        {({ submissions }) =>
          submissions.length === 0 ? (
            <Empty>No submission is waiting for review.</Empty>
          ) : (
            <table>
              <caption className="sr-only">Submissions awaiting review, oldest first</caption>
              <thead>
                <tr><th scope="col">Name</th><th scope="col">ID number</th><th scope="col">Born</th><th scope="col">Phone</th><th scope="col">Submitted</th><th scope="col"><span className="sr-only">Decision</span></th></tr>
              </thead>
              <tbody>
                {submissions.map((s) => (
                  <tr key={s.user_id}>
                    <td>{s.full_name}</td>
                    <td className="mono">{s.national_id}</td>
                    <td>{s.date_of_birth}</td>
                    <td>{msisdn(s.msisdn)}</td>
                    <td>{eat(s.submitted_at)}</td>
                    <td className="actions">
                      <button
                        type="button"
                        className="primary"
                        disabled={busy}
                        onClick={() => {
                          ask({
                            title: `Approve ${s.full_name}?`,
                            body: <p>The player becomes verified and can top up and play.</p>,
                            fields: [{ name: 'reason', label: 'What you checked', required: true, help: 'e.g. ID card seen, details match' }],
                            confirmLabel: 'Approve',
                            success: `${s.full_name} is verified.`,
                            act: (v) => api.post(`/v1/staff/kyc/${s.user_id}/approve`, { reason: v.reason }),
                          })
                        }}
                      >
                        Approve
                      </button>
                      <button
                        type="button"
                        className="danger-outline"
                        disabled={busy}
                        onClick={() => {
                          ask({
                            title: `Reject ${s.full_name}?`,
                            body: <p>The player cannot top up or play with this ID.</p>,
                            fields: [{ name: 'reason', label: 'Why', required: true }],
                            confirmLabel: 'Reject',
                            danger: true,
                            success: `${s.full_name} was rejected.`,
                            act: (v) => api.post(`/v1/staff/kyc/${s.user_id}/reject`, { reason: v.reason }),
                          })
                        }}
                      >
                        Reject
                      </button>
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
