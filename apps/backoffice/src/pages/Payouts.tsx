import type { ColumnDef } from '@tanstack/react-table'
import { Banknote } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useSession } from '../context'
import { Actions, DataTable, Stack } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, Mono, PageHeader, Pill, type Tone } from '../components/ui'
import { eat, kes } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { Payout } from '../types'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

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
  const load = useLoad(() => api.get<{ payouts: Payout[] }>('/v1/staff/payouts', { state }), [state])
  const { ask, busy, view } = useConfirmedAction(load.reload)
  const operator = can(me.roles, 'payout_operator')

  const columns = useMemo<ColumnDef<Payout>[]>(() => [
    { id: 'payout', header: 'Payout', accessorFn: (p) => `${p.kind} ${p.id}`, cell: ({ row }) => <Stack primary={<span className="capitalize">{row.original.kind}</span>} secondary={<Mono>{row.original.id}</Mono>} /> },
    {
      id: 'amount', header: 'Amount', accessorFn: (p) => p.net_kes, meta: { className: 'tabular-nums' },
      cell: ({ row }) => <Stack primary={`${kes(row.original.net_kes)} to M-Pesa`} secondary={row.original.tax_kes > 0 ? `tax ${kes(row.original.tax_kes)}` : undefined} />,
    },
    {
      id: 'state', header: 'State', accessorFn: (p) => p.state,
      cell: ({ row }) => <div className="grid justify-items-start gap-1"><Pill tone={tone(row.original.state)}>{row.original.state}</Pill>{row.original.reason ? <span className="text-sm text-muted-foreground">{row.original.reason}</span> : null}</div>,
    },
    { id: 'created', header: 'Created', accessorFn: (p) => p.created_at, cell: ({ row }) => <span className="text-muted-foreground">{eat(row.original.created_at)}</span> },
    {
      id: 'actions', header: () => <span className="sr-only">Actions</span>, enableSorting: false,
      cell: ({ row }) => {
        const p = row.original
        if (!operator) return null
        const dangerOutline = 'border-destructive/50 text-destructive hover:bg-destructive/10 hover:text-destructive'
        return (
          <Actions>
            {p.state === 'held' ? (
              <>
                <Button type="button" size="sm" disabled={busy} onClick={() => {
                  ask({ title: `Approve ${kes(p.net_kes)}?`, body: <p>It is sent through M-Pesa now.</p>, confirmLabel: 'Approve and send', success: 'Approved; it is being sent.',
                    act: () => api.post(`/v1/staff/payouts/${p.id}/approve`) })
                }}>Approve</Button>
                <Button type="button" size="sm" variant="outline" className={dangerOutline} disabled={busy} onClick={() => {
                  ask({ title: `Reject ${kes(p.net_kes)}?`, body: <p>A withdrawal’s amount returns to the player’s wallet.</p>,
                    fields: [{ name: 'reason', label: 'Reason', required: true }], confirmLabel: 'Reject', danger: true, success: 'Rejected.',
                    act: (v) => api.post(`/v1/staff/payouts/${p.id}/reject`, { reason: v.reason }) })
                }}>Reject</Button>
              </>
            ) : null}
            {p.state === 'manual' ? (
              <>
                <Button type="button" size="sm" disabled={busy} onClick={() => {
                  ask({ title: 'Paid according to the statement?', body: <p>Record the M-Pesa receipt from the statement.</p>,
                    fields: [{ name: 'receipt', label: 'M-Pesa receipt', required: true }], confirmLabel: 'Record as paid', success: 'Recorded as paid.',
                    act: (v) => api.post(`/v1/staff/payouts/${p.id}/settle`, { paid: true, receipt: v.receipt }) })
                }}>Paid</Button>
                <Button type="button" size="sm" variant="outline" className={dangerOutline} disabled={busy} onClick={() => {
                  ask({ title: 'Not paid according to the statement?', body: <p>A withdrawal’s amount returns to the wallet; a prize returns to its pool.</p>,
                    fields: [{ name: 'reason', label: 'What the statement shows', required: true }], confirmLabel: 'Record as not paid', danger: true, success: 'Recorded as not paid.',
                    act: (v) => api.post(`/v1/staff/payouts/${p.id}/settle`, { paid: false, reason: v.reason }) })
                }}>Not paid</Button>
              </>
            ) : null}
          </Actions>
        )
      },
    },
  ], [api, ask, busy, operator])

  return (
    <section>
      <PageHeader title="Payouts">
        Large withdrawals wait for approval. A payout whose M-Pesa answer was unclear is settled from the M-Pesa statement.
      </PageHeader>
      <Tabs value={state} onValueChange={setState}>
        <TabsList aria-label="Payout state" className="h-auto flex-wrap">
          {states.map(([s, label]) => <TabsTrigger key={s} value={s} className="cursor-pointer">{label}</TabsTrigger>)}
        </TabsList>
      </Tabs>
      {view}
      <Loaded load={load} what="payouts">
        {({ payouts }) =>
          payouts.length === 0 ? <Empty icon={Banknote}>Nothing here.</Empty> : (
            <DataTable columns={columns} data={payouts} rowId={(p) => p.id} caption="Payouts" filter="Search payouts" />
          )
        }
      </Loaded>
    </section>
  )
}
