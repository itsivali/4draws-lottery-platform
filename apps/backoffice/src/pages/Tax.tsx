import { useSession } from '../context'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, PageHeader, Pill } from '../components/ui'
import { eat, kes } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { Batch } from '../types'

export function TaxPage() {
  const { api, me } = useSession()
  const load = useLoad(() => api.get<{ batches: Batch[] }>('/v1/staff/tax/batches'))
  const { ask, busy, view } = useConfirmedAction(load.reload)
  const officer = can(me.roles, 'tax_officer')
  const download = async (b: Batch) => {
    const { blob } = await api.download(`/v1/staff/tax/batches/${b.batch_no}/file`)
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `kra-batch-${b.batch_no}.csv`
    a.click()
    URL.revokeObjectURL(a.href)
  }
  return (
    <section>
      <PageHeader title="KRA withholding tax">
        One person prepares a batch and files it with KRA; a different person records the remittance with its payment registration number.
      </PageHeader>
      {view}
      {officer ? (
        <button type="button" className="primary" disabled={busy} onClick={() => {
          ask({ title: 'Prepare the next batch?', body: <p>All withheld tax not yet batched goes into a numbered batch with a hashed file.</p>, confirmLabel: 'Prepare batch', success: 'Batch prepared.',
            act: () => api.post('/v1/staff/tax/batches') })
        }}>Prepare batch</button>
      ) : null}
      <Loaded load={load} what="batches">
        {({ batches }) =>
          batches.length === 0 ? <Empty>No batches yet.</Empty> : (
            <table>
              <caption className="sr-only">KRA batches</caption>
              <thead><tr><th scope="col">Batch</th><th scope="col">Tax</th><th scope="col">Prepared</th><th scope="col">Remitted</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead>
              <tbody>
                {batches.map((b) => (
                  <tr key={b.batch_no}>
                    <td>#{b.batch_no}<br /><span className="muted">{b.events} payouts</span></td>
                    <td>{kes(b.tax_kes)}<div className="muted">on {kes(b.gross_kes)}</div></td>
                    <td>{eat(b.prepared_at)}<div className="muted mono">{b.prepared_by}</div></td>
                    <td>{b.remitted_at ? <><Pill tone="success">Remitted</Pill><div className="muted">PRN {b.prn}</div></> : <Pill tone="pending">Not yet</Pill>}</td>
                    <td className="actions">
                      <button type="button" className="secondary" onClick={() => void download(b)}>Download file</button>
                      {officer && !b.remitted_at ? (
                        <button type="button" className="primary" disabled={busy} onClick={() => {
                          ask({ title: `Record remittance of batch #${b.batch_no}?`, body: <p>Must be recorded by someone other than who prepared it. The amount must equal the batch’s tax, {kes(b.tax_kes)}.</p>,
                            fields: [{ name: 'prn', label: 'KRA payment registration number', required: true }, { name: 'amount', label: 'Amount remitted (KES)', type: 'number', required: true }],
                            confirmLabel: 'Record remittance', success: 'Remittance recorded.',
                            act: (v) => api.post(`/v1/staff/tax/batches/${b.batch_no}/remit`, { prn: v.prn, amount_kes: Number(v.amount) }) })
                        }}>Record remittance</button>
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
