import type { ColumnDef } from '@tanstack/react-table'
import { IdCard } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useSession } from '../context'
import { Actions, DataTable } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, Mono, PageHeader } from '../components/ui'
import { eat, msisdn } from '../format'
import { useLoad } from '../hooks'
import { useReviewQueue } from '../reviewQueue'
import type { KycSubmission } from '../types'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

export function KycPage() {
  const { api } = useSession()
  const load = useLoad((signal) => api.get<{ submissions: KycSubmission[] }>('/v1/staff/kyc/pending', undefined, { signal }))
  const { queue, dismiss, refresh } = useReviewQueue()
  // after a decision the list and the count are re-read together, so they never disagree for a poll interval
  const { ask, busy, view } = useConfirmedAction(() => { load.reload(); refresh() })
  useEffect(dismiss, [dismiss, queue])
  // the list is reloaded only when asked: every submission it shows is audited as unmasked
  const fresh = (queue?.waiting ?? 0) - (load.data?.submissions.length ?? Infinity)

  const columns = useMemo<ColumnDef<KycSubmission>[]>(() => [
    { id: 'name', header: 'Name', accessorFn: (s) => s.full_name, cell: ({ row }) => <span className="font-medium">{row.original.full_name}</span> },
    { id: 'id', header: 'ID number', accessorFn: (s) => s.national_id, cell: ({ row }) => <Mono>{row.original.national_id}</Mono> },
    { id: 'born', header: 'Born', accessorFn: (s) => s.date_of_birth },
    { id: 'phone', header: 'Phone', accessorFn: (s) => msisdn(s.msisdn), cell: ({ row }) => <span className="tabular-nums">{msisdn(row.original.msisdn)}</span> },
    { id: 'submitted', header: 'Submitted', accessorFn: (s) => s.submitted_at, cell: ({ row }) => <span className="text-muted-foreground">{eat(row.original.submitted_at)}</span> },
    {
      id: 'decision',
      header: () => <span className="sr-only">Decision</span>,
      enableSorting: false,
      cell: ({ row }) => {
        const s = row.original
        return (
          <Actions>
            <Button
              type="button"
              size="sm"
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
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="border-destructive/50 text-destructive hover:bg-destructive/10 hover:text-destructive"
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
            </Button>
          </Actions>
        )
      },
    },
  ], [api, ask, busy])

  return (
    <section>
      <PageHeader title="Identity review">
        Compare each submission with the person’s national ID. Opening this list is recorded in the audit log.
      </PageHeader>
      {view}
      {fresh > 0 ? (
        <Alert className="my-4 border-pending/40 bg-pending/10 [&>svg]:text-pending">
          <IdCard />
          <AlertTitle>{fresh === 1 ? 'A new submission arrived.' : `${fresh} new submissions arrived.`}</AlertTitle>
          <AlertDescription>
            <Button type="button" size="sm" variant="outline" onClick={load.reload}>Show {fresh} new {fresh === 1 ? 'submission' : 'submissions'}</Button>
          </AlertDescription>
        </Alert>
      ) : null}
      <Loaded load={load} what="submissions">
        {({ submissions }) =>
          submissions.length === 0 ? (
            <Empty icon={IdCard}>No submission is waiting for review.</Empty>
          ) : (
            <DataTable columns={columns} data={submissions} rowId={(s) => s.user_id} caption="Submissions awaiting review, oldest first" filter="Search by name, ID or phone" />
          )
        }
      </Loaded>
    </section>
  )
}
