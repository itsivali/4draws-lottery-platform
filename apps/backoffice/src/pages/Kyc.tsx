import type { ColumnDef } from '@tanstack/react-table'
import { IdCard, Mail } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useSession } from '../context'
import { Actions, DataTable } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, Mono, PageHeader } from '../components/ui'
import { eat, msisdn } from '../format'
import { useLoad } from '../hooks'
import { useReviewQueue } from '../reviewQueue'
import type { KycSubmission } from '../types'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

export function KycPage() {
  const { api } = useSession()
  // the email sent when a submission arrives links here with the account: that row is marked, or its absence explained
  const [params] = useSearchParams()
  const linked = params.get('user')
  const load = useLoad((signal) => api.get<{ submissions: KycSubmission[] }>('/v1/staff/kyc/pending', undefined, { signal }))
  const { queue, dismiss, refresh } = useReviewQueue()
  // after a decision the list and the count are re-read together, so they never disagree for a poll interval
  const { ask, busy, view } = useConfirmedAction(() => { load.reload(); refresh() })
  useEffect(dismiss, [dismiss, queue])
  // the list is reloaded only when asked: every submission it shows is audited as unmasked
  const fresh = (queue?.waiting ?? 0) - (load.data?.submissions.length ?? Infinity)

  const columns = useMemo<ColumnDef<KycSubmission>[]>(() => [
    {
      id: 'name',
      header: 'Name',
      accessorFn: (s) => s.full_name,
      cell: ({ row }) => (
        <span className="font-medium">
          {row.original.full_name}
          {row.original.user_id === linked ? <Badge variant="outline" className="ml-2 border-pending/50 text-pending"><Mail aria-hidden="true" className="size-3" /> From your email</Badge> : null}
        </span>
      ),
    },
    { id: 'id', header: 'ID number', accessorFn: (s) => s.national_id, cell: ({ row }) => <Mono>{row.original.national_id}</Mono> },
    {
      id: 'born', header: 'Born', accessorFn: (s) => s.date_of_birth,
      // not asked since 2026-10-30: the M-Pesa line is registered to an adult; check the name against the ID instead
      cell: ({ row }) => row.original.date_of_birth || <span className="text-sm text-muted-foreground">Not asked</span>,
    },
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
                  body: <p>The player becomes verified and can withdraw and be paid prizes.</p>,
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
                  body: <p>The player cannot withdraw or be paid with this ID; they may still play.</p>,
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
  ], [api, ask, busy, linked])
  const linkedGone = linked !== null && load.data !== undefined && !load.data.submissions.some((s) => s.user_id === linked)

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
      {linkedGone ? (
        <Alert className="my-4">
          <Mail />
          <AlertTitle>The submission from your email is no longer waiting.</AlertTitle>
          <AlertDescription>It was decided already, or the player submitted again. Player <Mono>{linked}</Mono></AlertDescription>
        </Alert>
      ) : null}
      <Loaded load={load} what="submissions">
        {({ submissions }) =>
          submissions.length === 0 ? (
            <Empty icon={IdCard}>No submission is waiting for review.</Empty>
          ) : (
            <DataTable columns={columns} data={submissions} rowId={(s) => s.user_id} caption="Submissions awaiting review, oldest first" description="Identity submissions to compare with the national ID. Opening one shows the personal details, and that is recorded under your name." filter="Search by name, ID or phone" highlight={linked ?? undefined} />
          )
        }
      </Loaded>
    </section>
  )
}
