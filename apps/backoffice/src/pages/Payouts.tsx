import type { ColumnDef } from '@tanstack/react-table'
import { Banknote } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useSession } from '../context'
import { Actions, DataTable, Stack } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, Mono, PageHeader, Pill, type Tone } from '../components/ui'
import { eat, kes, msisdn } from '../format'
import { usePages } from '../hooks'
import { can } from '../roles'
import type { Payout } from '../types'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ReceiptCode } from '../components/polish'

const states = [
  ['held', 'Held for approval'],
  ['manual', 'Needs manual settlement'],
  ['timed_out', 'Timed out'],
  ['failed', 'Failed'],
  ['succeeded', 'Paid'],
] as const

const tone = (state: string): Tone => (state === 'succeeded' ? 'success' : state === 'failed' ? 'error' : 'pending')

export function PayoutsPage() {
  const { api, me } = useSession()
  const [state, setState] = useState<string>('held')
  // the operator's queue, oldest first, 50 a page; "Load more" follows the server's cursor forward
  const load = usePages((after, signal) =>
    api.get<{ payouts: Payout[]; next_after: string | null }>('/v1/staff/payouts', { state, limit: '50', ...(after === null ? {} : { after }) }, { signal })
      .then((r) => ({ rows: r.payouts, next: r.next_after })), [state])
  const { ask, busy, view } = useConfirmedAction(load.reload)
  const operator = can(me.roles, 'staff')

  const columns = useMemo<ColumnDef<Payout>[]>(() => [
    {
      id: 'payout', header: 'Payout', accessorFn: (p) => `${p.refund ? 'refund' : p.kind} ${p.id}`,
      cell: ({ row }) => {
        const r = row.original.refund
        return <Stack primary={<span className="capitalize">{r ? 'Refund to source' : row.original.kind}</span>}
          secondary={r ? <>{r.reason} · asked by <Mono>{r.requested_by}</Mono> · to {msisdn(r.msisdn)} · someone else approves</> : <Mono>{row.original.id}</Mono>} />
      },
    },
    {
      id: 'amount', header: 'Amount', accessorFn: (p) => p.net_kes, meta: { className: 'tabular-nums' },
      cell: ({ row }) => <Stack primary={`${kes(row.original.net_kes)} to M-Pesa`} secondary={row.original.tax_kes > 0 ? `tax ${kes(row.original.tax_kes)}` : undefined} />,
    },
    {
      id: 'state', header: 'State', accessorFn: (p) => p.state,
      cell: ({ row }) => {
        const p = row.original
        const approvers = [...(p.refund ? [p.refund.requested_by] : []), ...(p.approvals ?? [])]
        return (
          <div className="grid justify-items-start gap-1">
            <Pill tone={tone(p.state)}>{p.state}</Pill>
            {p.reason ? <span className="text-sm text-muted-foreground">{p.reason}</span> : null}
            {p.state === 'held' && approvers.length > 0 ? <span className="text-sm">{p.approvals?.length ? <>Approved by {p.approvals.map((a) => <Mono key={a}>{a}</Mono>)}</> : 'Asked for by the admin'}; one more person approves it</span> : null}
            {p.proposal ? <span className="text-sm">Proposed by <Mono>{p.proposal.by}</Mono>: not paid ({p.proposal.reason}); a second person confirms it</span> : null}
            {p.check && p.state === 'manual' ? (
              <span className="text-sm">
                Receipt <ReceiptCode value={p.check.receipt} />{' '}
                {p.check.outcome === '' ? 'is being checked with M-Pesa' : p.check.outcome === 'refuted' ? <>was not confirmed by M-Pesa{p.check.detail ? `: ${p.check.detail}` : ''}</> : 'got no answer from M-Pesa; record it again'}
              </span>
            ) : null}
          </div>
        )
      },
    },
    { id: 'created', header: 'Created', accessorFn: (p) => p.created_at, cell: ({ row }) => <span className="text-muted-foreground">{eat(row.original.created_at)}</span> },
    {
      id: 'actions', header: () => <span className="sr-only">Actions</span>, enableSorting: false,
      cell: ({ row }) => {
        const p = row.original
        if (!operator) return null
        const mine = `staff:${me.id}`
        const approvers = [...(p.refund ? [p.refund.requested_by] : []), ...(p.approvals ?? [])]
        const approvedByMe = approvers.includes(mine)
        const last = approvers.length >= 1
        const dangerOutline = 'border-destructive/50 text-destructive hover:bg-destructive/10 hover:text-destructive'
        return (
          <Actions>
            {p.state === 'held' ? (
              <>
                <Button type="button" size="sm" disabled={busy || approvedByMe} title={approvedByMe ? 'A second person must approve it' : undefined} onClick={() => {
                  ask(last
                    ? { title: `Approve ${kes(p.net_kes)}?`, body: <p>The second approval: it is sent through M-Pesa now.</p>, confirmLabel: 'Approve and send', success: 'Approved; it is being sent.',
                      act: () => api.post(`/v1/staff/payouts/${p.id}/approve`) }
                    : { title: `Approve ${kes(p.net_kes)}?`, body: <p>The first of two approvals: nothing is sent until a second staff member approves it too.</p>, confirmLabel: 'Approve', success: 'Approved. A second person approves it before it is sent.',
                      act: () => api.post(`/v1/staff/payouts/${p.id}/approve`) })
                }}>{approvedByMe ? 'You approved' : 'Approve'}</Button>
                <Button type="button" size="sm" variant="outline" className={dangerOutline} disabled={busy} onClick={() => {
                  ask({ title: `Reject ${kes(p.net_kes)}?`, body: <p>A withdrawal’s amount returns to the player’s wallet.</p>,
                    fields: [{ name: 'reason', label: 'Reason', required: true }], confirmLabel: 'Reject', danger: true, success: 'Rejected.',
                    act: (v) => api.post(`/v1/staff/payouts/${p.id}/reject`, { reason: v.reason }) })
                }}>Reject</Button>
              </>
            ) : null}
            {p.state === 'manual' ? (
              <>
                <Button type="button" size="sm" disabled={busy || p.check?.outcome === ''} onClick={() => {
                  ask({ title: 'Paid according to the statement?', body: <p>Record the M-Pesa receipt from the statement. M-Pesa is asked about it, and the payout is settled as paid only when M-Pesa confirms that receipt paid this amount to this number.</p>,
                    fields: [{ name: 'receipt', label: 'M-Pesa receipt', required: true }], confirmLabel: 'Check with M-Pesa', success: 'Receipt recorded; M-Pesa is checking it.',
                    act: (v) => api.post(`/v1/staff/payouts/${p.id}/settle`, { paid: true, receipt: v.receipt }) })
                }}>Paid</Button>
                <Button type="button" size="sm" variant="outline" className={dangerOutline} disabled={busy || p.check?.outcome === ''} onClick={() => {
                  ask({ title: 'Not paid according to the statement?', body: <p>A withdrawal’s amount returns to the wallet; a prize returns to its pool.</p>,
                    fields: [{ name: 'reason', label: 'What the statement shows', required: true }], confirmLabel: 'Record as not paid', danger: true, success: 'Recorded. It is settled once two people agree.',
                    act: (v) => api.post(`/v1/staff/payouts/${p.id}/settle`, { paid: false, reason: v.reason }) })
                }}>Not paid</Button>
              </>
            ) : null}
          </Actions>
        )
      },
    },
  ], [api, ask, busy, operator, me.id])

  return (
    <section>
      <PageHeader title="Payouts">
        Large withdrawals wait for two different people to approve them. A payout whose M-Pesa answer was unclear is settled from
        the M-Pesa statement: paid when M-Pesa confirms the receipt found there, not paid when two people agree.
      </PageHeader>
      <Tabs value={state} onValueChange={setState}>
        <TabsList aria-label="Payout state" className="h-auto flex-wrap">
          {states.map(([s, label]) => <TabsTrigger key={s} value={s} className="cursor-pointer">{label}</TabsTrigger>)}
        </TabsList>
      </Tabs>
      {view}
      <Loaded load={load} what="payouts">
        {(payouts) =>
          payouts.length === 0 ? <Empty icon={Banknote}>Nothing here.</Empty> : (
            <DataTable columns={columns} data={payouts} rowId={(p) => p.id} caption="Payouts" description="Withdrawals, prizes and refunds in the chosen state. Held payouts wait for two people; unclear ones are settled from the M-Pesa statement." filter="Search payouts"
              more={{ has: load.more, loading: load.loadingMore, load: load.loadMore }} />
          )
        }
      </Loaded>
    </section>
  )
}
