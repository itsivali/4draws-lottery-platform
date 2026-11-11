import type { ColumnDef } from '@tanstack/react-table'
import { Download, Receipt } from 'lucide-react'
import { useMemo, useState } from 'react'
import { ProblemError } from '../api'
import { useSession } from '../context'
import { Actions, DataTable, Stack } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, Mono, PageHeader, Pill } from '../components/ui'
import { eat, kes } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { Batch } from '../types'
import { Alert, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ExportReceipt, type SavedFile } from '../components/polish'

export function TaxPage() {
  const { api, me } = useSession()
  const load = useLoad((signal) => api.get<{ batches: Batch[] }>('/v1/staff/tax/batches', undefined, { signal }))
  const { ask, busy, view } = useConfirmedAction(load.reload)
  const officer = can(me.roles, 'staff')
  const [downloading, setDownloading] = useState<number | null>(null)
  const [downloadError, setDownloadError] = useState<string | null>(null)
  const [saved, setSaved] = useState<SavedFile | null>(null)
  const download = async (b: Batch) => {
    setDownloading(b.batch_no)
    setDownloadError(null)
    setSaved(null)
    try {
      const { blob, sha256, rows } = await api.download(`/v1/staff/tax/batches/${b.batch_no}/file`)
      if (sha256 && sha256 !== b.file_sha256) {
        setDownloadError(`The file of batch #${b.batch_no} did not match its recorded hash; it was not saved. Try again.`)
        return
      }
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = `kra-batch-${b.batch_no}.csv`
      document.body.appendChild(a)
      a.click()
      a.remove()
      setTimeout(() => { URL.revokeObjectURL(a.href) }, 1000)
      setSaved({ name: `kra-batch-${b.batch_no}.csv`, sha256: sha256 ?? b.file_sha256, rows, at: new Date(),
        note: sha256 ? 'The file’s hash matches the one recorded when the batch was prepared.' : 'This is the hash recorded when the batch was prepared; check the saved file against it.' })
    } catch (e) {
      setDownloadError(e instanceof ProblemError ? e.title : 'The file could not be downloaded. Try again.')
    } finally {
      setDownloading(null)
    }
  }

  const columns = useMemo<ColumnDef<Batch>[]>(() => [
    { id: 'batch', header: 'Batch', accessorFn: (b) => b.batch_no, cell: ({ row }) => <Stack primary={`#${row.original.batch_no}`} secondary={`${row.original.events} payouts`} /> },
    { id: 'tax', header: 'Tax', accessorFn: (b) => b.tax_kes, meta: { className: 'tabular-nums' }, cell: ({ row }) => <Stack primary={kes(row.original.tax_kes)} secondary={`on ${kes(row.original.gross_kes)}`} /> },
    { id: 'prepared', header: 'Prepared', accessorFn: (b) => b.prepared_at, cell: ({ row }) => <Stack primary={<span className="font-normal text-muted-foreground">{eat(row.original.prepared_at)}</span>} secondary={<Mono>{row.original.prepared_by}</Mono>} /> },
    {
      id: 'remitted', header: 'Remitted', accessorFn: (b) => b.remitted_at ?? '',
      cell: ({ row }) => (row.original.remitted_at ? <div className="grid justify-items-start gap-1"><Pill tone="success">Remitted</Pill><span className="text-sm text-muted-foreground">PRN {row.original.prn}</span></div> : <Pill tone="pending">Not yet</Pill>),
    },
    {
      id: 'actions', header: () => <span className="sr-only">Actions</span>, enableSorting: false,
      cell: ({ row }) => {
        const b = row.original
        return (
          <Actions>
            <Button type="button" size="sm" variant="outline" disabled={downloading === b.batch_no} onClick={() => void download(b)}>
              <Download aria-hidden="true" />{downloading === b.batch_no ? 'Downloading…' : 'Download file'}
            </Button>
            {officer && !b.remitted_at && b.prepared_by !== me.id ? (
              <Button type="button" size="sm" disabled={busy} onClick={() => {
                ask({ title: `Record remittance of batch #${b.batch_no}?`, body: <p>Must be recorded by someone other than who prepared it. The amount must equal the batch’s tax, {kes(b.tax_kes)}.</p>,
                  fields: [{ name: 'prn', label: 'KRA payment registration number', required: true }, { name: 'amount', label: 'Amount remitted (KES)', type: 'number', required: true }],
                  confirmLabel: 'Record remittance', success: 'Remittance recorded.',
                  act: (v) => api.post(`/v1/staff/tax/batches/${b.batch_no}/remit`, { prn: v.prn, amount_kes: Number(v.amount) }) })
              }}>Record remittance</Button>
            ) : null}
          </Actions>
        )
      },
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [api, ask, busy, officer, downloading, me.id])

  return (
    <section>
      <PageHeader
        title="KRA withholding tax"
        actions={officer ? (
          <Button type="button" disabled={busy} onClick={() => {
            ask({ title: 'Prepare the next batch?', body: <p>All withheld tax not yet batched goes into a numbered batch with a hashed file.</p>, confirmLabel: 'Prepare batch', success: 'Batch prepared.',
              act: () => api.post('/v1/staff/tax/batches') })
          }}><Receipt aria-hidden="true" /> Prepare batch</Button>
        ) : undefined}
      >
        One person prepares a batch and files it with KRA; a different person records the remittance with its payment registration number.
      </PageHeader>
      {view}
      {downloadError ? <Alert variant="destructive" className="my-4"><AlertTitle>{downloadError}</AlertTitle></Alert> : null}
      {saved ? <ExportReceipt file={saved} className="max-w-xl" /> : null}
      <Loaded load={load} what="batches">
        {({ batches }) =>
          batches.length === 0 ? <Empty icon={Receipt}>No batches yet.</Empty> : (
            <DataTable columns={columns} data={batches} rowId={(b) => String(b.batch_no)} caption="KRA batches" description="Withholding tax grouped into numbered batches for KRA. One person prepares a batch; someone else records its remittance." />
          )
        }
      </Loaded>
    </section>
  )
}
