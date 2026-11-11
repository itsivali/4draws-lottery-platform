import type { ColumnDef } from '@tanstack/react-table'
import { UserX } from 'lucide-react'
import { useMemo } from 'react'
import { useSession } from '../context'
import { Actions, DataTable, Stack } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, Mono, PageHeader, Pill } from '../components/ui'
import { eat, msisdn } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { Closure } from '../types'
import { Button } from '@/components/ui/button'

export function ClosuresPage() {
  const { api, me } = useSession()
  const load = useLoad((signal) => api.get<{ closures: Closure[] }>('/v1/staff/closures', undefined, { signal }))
  const { ask, busy, view } = useConfirmedAction(load.reload)
  const support = can(me.roles, 'staff')

  const columns = useMemo<ColumnDef<Closure>[]>(() => [
    { id: 'phone', header: 'Phone', accessorFn: (c) => `${msisdn(c.msisdn)} ${c.user_id}`, cell: ({ row }) => <Stack primary={<span className="tabular-nums">{msisdn(row.original.msisdn)}</span>} secondary={<Mono>{row.original.user_id}</Mono>} /> },
    { id: 'status', header: 'Account', accessorFn: (c) => c.status, cell: ({ row }) => <Pill tone={row.original.status === 'active' ? 'success' : 'neutral'}><span className="capitalize">{row.original.status.replace(/_/g, ' ')}</span></Pill> },
    { id: 'requested', header: 'Requested', accessorFn: (c) => c.requested_at, cell: ({ row }) => <span className="text-muted-foreground">{eat(row.original.requested_at)}</span> },
    {
      id: 'actions', header: () => <span className="sr-only">Actions</span>, enableSorting: false,
      cell: ({ row }) => {
        const c = row.original
        if (!support) return null
        return (
          <Actions>
            <Button type="button" size="sm" variant="outline" className="border-destructive/50 text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={busy} onClick={() => {
              ask({ title: `Close the account of ${msisdn(c.msisdn)}?`, body: <p>Sign-in, play, top-ups and withdrawals stop. The account’s password, sessions, devices, Google or Apple links, photo and email address are erased; the records the law requires (identity checks, payments, plays, prizes) are kept. This cannot be undone.</p>,
                confirmLabel: 'Close account', danger: true, success: 'Account closed.', act: () => api.post(`/v1/staff/closures/${c.user_id}/close`) })
            }}>Close account</Button>
          </Actions>
        )
      },
    },
  ], [api, ask, busy, support])

  return (
    <section>
      <PageHeader title="Account closures">
        An account closes only when no money is in it or moving: the balance must be paid out, and every top-up and payout must be final.
        The player’s records are kept as the law requires.
      </PageHeader>
      {view}
      <Loaded load={load} what="closure requests">
        {({ closures }) =>
          closures.length === 0 ? <Empty icon={UserX}>No open requests.</Empty> : (
            <DataTable columns={columns} data={closures} rowId={(c) => c.user_id} caption="Open closure requests, oldest first" description="Players who asked to close their account. An account closes only once its balance is paid out and every top-up and payout is final." filter="Search by phone or id" />
          )
        }
      </Loaded>
    </section>
  )
}
