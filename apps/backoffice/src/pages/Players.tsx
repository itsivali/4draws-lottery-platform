import type { ColumnDef } from '@tanstack/react-table'
import { Search, UserRound, UserRoundCheck, UserRoundX } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Actions, DataTable, Stack } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, Mono, PageHeader, Pill, type Tone } from '../components/ui'
import { useSession } from '../context'
import { eat, kes, msisdn } from '../format'
import { useLoad } from '../hooks'
import type { Player } from '../types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

const statuses = [['', 'All'], ['active', 'Active'], ['suspended', 'Suspended'], ['closed', 'Closed']] as const

const statusTone: Record<Player['status'], Tone> = { active: 'success', suspended: 'warning', closed: 'neutral' }

const kycLabel: Record<Player['kyc_status'], [Tone, string]> = {
  none: ['neutral', 'No ID yet'],
  pending: ['pending', 'ID pending'],
  manual_review: ['pending', 'ID in review'],
  verified: ['success', 'ID verified'],
  rejected: ['error', 'ID rejected'],
}

/** Every registered player, newest first, with search, a status filter, and suspension (ADR 0019). */
export function PlayersPage() {
  const { api } = useSession()
  const [typed, setTyped] = useState('')
  const [q, setQ] = useState('')
  const [status, setStatus] = useState<string>('')
  const load = useLoad((signal) => api.get<{ players: Player[]; next_before: string | null }>('/v1/staff/players',
    { limit: '100', ...(q ? { q } : {}), ...(status ? { status } : {}) }, { signal }), [q, status])
  const { ask, busy, view } = useConfirmedAction(load.reload)

  const columns = useMemo<ColumnDef<Player>[]>(() => [
    {
      id: 'player', header: 'Player', accessorFn: (p) => `${p.full_name} ${msisdn(p.msisdn)} ${p.user_id}`,
      cell: ({ row }) => (
        <Stack primary={row.original.full_name || <span className="font-normal text-muted-foreground">No name yet</span>} secondary={<span className="tabular-nums">{msisdn(row.original.msisdn)}</span>}>
          <Mono className="text-muted-foreground">{row.original.user_id}</Mono>
        </Stack>
      ),
    },
    {
      id: 'id', header: 'National ID', accessorFn: (p) => p.national_id,
      cell: ({ row }) => (row.original.national_id ? <Mono>{row.original.national_id}</Mono> : <span className="text-sm text-muted-foreground">—</span>),
    },
    {
      id: 'status', header: 'Status', accessorFn: (p) => `${p.status} ${p.kyc_status}`,
      cell: ({ row }) => {
        const p = row.original
        const [tone, label] = kycLabel[p.kyc_status]
        return (
          <div className="flex flex-wrap gap-1">
            <Pill tone={statusTone[p.status]}><span className="capitalize">{p.status}</span></Pill>
            <Pill tone={tone}>{label}</Pill>
            {p.phone_verified ? null : <Pill tone="neutral">Phone not proved</Pill>}
            {p.closure_requested_at ? <Pill tone="pending">Asked to close</Pill> : null}
          </div>
        )
      },
    },
    { id: 'balance', header: 'Balance', accessorFn: (p) => p.balance_kes, meta: { className: 'text-right tabular-nums' }, cell: ({ row }) => kes(row.original.balance_kes) },
    { id: 'plays', header: 'Plays', accessorFn: (p) => p.plays, meta: { className: 'text-right tabular-nums' }, cell: ({ row }) => row.original.plays.toLocaleString('en-KE') },
    {
      id: 'seen', header: 'Joined · last sign-in', accessorFn: (p) => p.created_at,
      cell: ({ row }) => <Stack primary={<span className="font-normal">{eat(row.original.created_at)}</span>} secondary={row.original.last_sign_in_at ? `last ${eat(row.original.last_sign_in_at)}` : 'never signed in'} />,
    },
    {
      id: 'actions', header: () => <span className="sr-only">Actions</span>, enableSorting: false,
      cell: ({ row }) => {
        const p = row.original
        if (p.status === 'closed') return null
        return (
          <Actions>
            {p.status === 'active' ? (
              <Button type="button" size="sm" variant="outline" className="border-destructive/50 text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={busy} onClick={() => {
                ask({
                  title: `Suspend ${msisdn(p.msisdn)}?`,
                  body: <p>Sign-in, play, top-ups and withdrawals stop and every session ends. The balance stays in the wallet. You can reinstate the account later.</p>,
                  fields: [{ name: 'reason', label: 'Reason (goes in the audit log)', required: true }],
                  confirmLabel: 'Suspend', danger: true, success: `${msisdn(p.msisdn)} is suspended.`,
                  act: (v) => api.post(`/v1/staff/players/${p.user_id}/suspend`, { reason: v.reason }),
                })
              }}><UserRoundX aria-hidden="true" /> Suspend</Button>
            ) : (
              <Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => {
                ask({
                  title: `Reinstate ${msisdn(p.msisdn)}?`,
                  body: <p>The player can sign in, play, top up and withdraw again.</p>,
                  fields: [{ name: 'reason', label: 'Reason (goes in the audit log)', required: true }],
                  confirmLabel: 'Reinstate', success: `${msisdn(p.msisdn)} is active again.`,
                  act: (v) => api.post(`/v1/staff/players/${p.user_id}/reinstate`, { reason: v.reason }),
                })
              }}><UserRoundCheck aria-hidden="true" /> Reinstate</Button>
            )}
          </Actions>
        )
      },
    },
  ], [api, ask, busy])

  return (
    <section>
      <PageHeader title="Players">
        Every registered account, newest first. Identity decisions are made on the Identity review page and closures on
        Account closures; here you can look a player up and suspend or reinstate them. Names and ID numbers come from
        the player’s latest identity submission. Each view is recorded in the audit log, because it shows personal data.
      </PageHeader>
      <form className="mb-4 flex flex-wrap items-center gap-3" onSubmit={(e) => { e.preventDefault(); setQ(typed.trim()) }}>
        <div className="relative w-full max-w-sm">
          <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label="Phone number or user id" placeholder="Phone number or user id" value={typed} onChange={(e) => { setTyped(e.target.value) }} className="pl-8" />
        </div>
        <Button type="submit" variant="outline">Search</Button>
        {q ? <Button type="button" variant="ghost" onClick={() => { setTyped(''); setQ('') }}>Clear</Button> : null}
        <Tabs value={status} onValueChange={setStatus} className="ml-auto">
          <TabsList aria-label="Account status" className="h-auto flex-wrap">
            {statuses.map(([s, label]) => <TabsTrigger key={s} value={s} className="cursor-pointer">{label}</TabsTrigger>)}
          </TabsList>
        </Tabs>
      </form>
      {view}
      <Loaded load={load} what="players">
        {({ players, next_before }) =>
          players.length === 0 ? <Empty icon={UserRound}>{q || status ? 'No player matches.' : 'No players yet.'}</Empty> : (
            <>
              <DataTable columns={columns} data={players} rowId={(p) => p.user_id} caption="Players, newest first" filter="Filter these rows" pageSize={25} />
              {next_before ? <p className="mt-3 text-sm text-muted-foreground">Showing the newest 100. Search by phone number or user id to find an older player.</p> : null}
            </>
          )
        }
      </Loaded>
    </section>
  )
}
