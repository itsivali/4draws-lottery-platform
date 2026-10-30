import type { ColumnDef } from '@tanstack/react-table'
import { Search, Smartphone } from 'lucide-react'
import { useMemo, useState } from 'react'
import { DataTable, Stack } from '../components/DataTable'
import { Empty, Loaded, Mono, PageHeader, Pill, type Tone } from '../components/ui'
import { useSession } from '../context'
import { eat, kes, msisdn } from '../format'
import { usePages } from '../hooks'
import type { TopUpRow } from '../types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

const filters = [['', 'All'], ['credited', 'Credited'], ['stk_sent', 'Waiting'], ['failed', 'Failed'], ['cancelled', 'Cancelled'], ['unconfirmed', 'Unconfirmed'], ['reversed', 'Reversed']] as const

const label: Record<string, [Tone, string]> = {
  initiated: ['pending', 'Sending prompt'],
  stk_sent: ['pending', 'Waiting for M-Pesa'],
  callback_received: ['pending', 'Checking'],
  verified: ['pending', 'Verified'],
  credited: ['success', 'Credited'],
  failed: ['error', 'Failed'],
  cancelled: ['neutral', 'Cancelled'],
  expired: ['neutral', 'Expired'],
  unconfirmed: ['warning', 'Unconfirmed'],
  reversed: ['error', 'Reversed'],
}

/** M-Pesa's result codes the team meets most, in words. */
const resultCodes: Record<number, string> = {
  0: 'paid', 1: 'insufficient balance', 1032: 'cancelled by the customer', 1037: 'phone did not answer (timeout)', 2001: 'wrong PIN', 4999: 'still processing',
}

/** Every top-up, newest first: who, how much, how it ended, and the receipt. */
export function TopUpsPage() {
  const { api } = useSession()
  const [typed, setTyped] = useState('')
  const [q, setQ] = useState('')
  const [status, setStatus] = useState('')
  const load = usePages((before, signal) =>
    api.get<{ topups: TopUpRow[]; next_before: string | null }>('/v1/staff/topups',
      { limit: '50', ...(q ? { q } : {}), ...(status ? { status } : {}), ...(before === null ? {} : { before }) }, { signal })
      .then((r) => ({ rows: r.topups, next: r.next_before })), [q, status])

  const columns = useMemo<ColumnDef<TopUpRow>[]>(() => [
    {
      id: 'when', header: 'When', accessorFn: (t) => t.created_at,
      cell: ({ row }) => <Stack primary={<span className="font-normal tabular-nums">{eat(row.original.created_at)}</span>} secondary={row.original.channel === 'stk' ? 'M-Pesa prompt' : 'Paybill'} />,
    },
    {
      id: 'player', header: 'Player', accessorFn: (t) => `${t.full_name ?? ''} ${msisdn(t.msisdn)} ${t.user_id}`,
      cell: ({ row }) => (
        <Stack primary={row.original.full_name ?? <span className="font-normal text-muted-foreground">No name yet</span>}
          secondary={<span className="tabular-nums">{msisdn(row.original.msisdn)}</span>} />
      ),
    },
    {
      id: 'amount', header: 'Amount', accessorFn: (t) => t.amount_confirmed_kes ?? t.amount_requested_kes, meta: { className: 'text-right tabular-nums' },
      cell: ({ row }) => {
        const t = row.original
        return <Stack primary={kes(t.amount_confirmed_kes ?? t.amount_requested_kes)} secondary={t.amount_confirmed_kes !== null && t.amount_confirmed_kes !== t.amount_requested_kes ? `asked ${kes(t.amount_requested_kes)}` : undefined} />
      },
    },
    {
      id: 'status', header: 'Outcome', accessorFn: (t) => t.status,
      cell: ({ row }) => {
        const t = row.original
        const [tone, text] = label[t.status] ?? ['neutral', t.status]
        // a sandbox placeholder credit was never paid: M-Pesa's answer was not a payment, whatever code was stored
        const code = t.sandbox ? 'credited without M-Pesa’s confirmation (sandbox, before 30 Oct)' : t.result_code !== null ? (resultCodes[t.result_code] ?? `M-Pesa code ${t.result_code}`) : null
        const why = [code, t.failure_reason?.replaceAll('_', ' ')].filter(Boolean).join(' · ')
        return (
          <div className="grid justify-items-start gap-1">
            <div className="flex flex-wrap gap-1"><Pill tone={tone}>{text}</Pill>{t.sandbox ? <Pill tone="pending">Sandbox</Pill> : null}</div>
            {why ? <span className="text-sm text-muted-foreground">{why}</span> : null}
          </div>
        )
      },
    },
    {
      id: 'receipt', header: 'Receipt', accessorFn: (t) => t.receipt ?? '',
      cell: ({ row }) => <Stack primary={row.original.receipt ? <Mono>{row.original.receipt}</Mono> : <span className="font-normal text-muted-foreground">—</span>} secondary={<Mono className="text-muted-foreground">{row.original.id}</Mono>} />,
    },
  ], [])

  return (
    <section>
      <PageHeader title="Top-ups">
        Every top-up, newest first, with how it ended. “Sandbox” marks a top-up credited without a payment while the
        Daraja sandbox was being tested (30 October); since then only real M-Pesa payments are credited.
      </PageHeader>
      <form className="mb-4 flex flex-wrap items-center gap-3" onSubmit={(e) => { e.preventDefault(); setQ(typed.trim()) }}>
        <div className="relative w-full max-w-sm">
          <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label="Phone number, player id or receipt" placeholder="Phone number, player id or receipt" value={typed} onChange={(e) => { setTyped(e.target.value) }} className="pl-8" />
        </div>
        <Button type="submit" variant="outline">Search</Button>
        {q ? <Button type="button" variant="ghost" onClick={() => { setTyped(''); setQ('') }}>Clear</Button> : null}
        <Tabs value={status} onValueChange={setStatus} className="ml-auto">
          <TabsList aria-label="Outcome" className="h-auto flex-wrap">
            {filters.map(([s, l]) => <TabsTrigger key={s} value={s} className="cursor-pointer">{l}</TabsTrigger>)}
          </TabsList>
        </Tabs>
      </form>
      <Loaded load={load} what="top-ups">
        {(topups) =>
          topups.length === 0 ? <Empty icon={Smartphone}>{q || status ? 'No top-up matches.' : 'No top-ups yet.'}</Empty> : (
            <DataTable columns={columns} data={topups} rowId={(t) => t.id} caption="Top-ups, newest first" filter="Filter these rows"
              more={{ has: load.more, loading: load.loadingMore, load: load.loadMore }} />
          )
        }
      </Loaded>
    </section>
  )
}
