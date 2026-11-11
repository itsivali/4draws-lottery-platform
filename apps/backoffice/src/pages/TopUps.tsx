import type { ColumnDef } from '@tanstack/react-table'
import { Download, Search, Smartphone } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { DataTable, Stack } from '../components/DataTable'
import { claimLabel, resultCodes, statusLabel, TopUpDetailSheet } from '../components/TopUpDetail'
import { Empty, Loaded, Mono, PageHeader, Pill } from '../components/ui'
import { useSession } from '../context'
import { eat, kes, msisdn } from '../format'
import { usePages } from '../hooks'
import type { TopUpRow } from '../types'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ReceiptCode } from '../components/polish'

const filters = [['', 'All'], ['credited', 'Credited'], ['stk_sent', 'Waiting'], ['failed', 'Failed'], ['cancelled', 'Cancelled'], ['unconfirmed', 'Unconfirmed'], ['reversed', 'Reversed']] as const

/** failure reasons the platform writes, in words */
const reasons: [string, string][] = [
  ['paid_without_callback', 'paid, callback never came'],
  ['still_pending_at_expiry', 'never answered'],
  ['query_rejected', 'M-Pesa refused the status query'],
  ['receipt_reused', 'receipt already used'],
  ['not_on_statement', 'closed: not on the statement'],
  ['push_throttled', 'M-Pesa throttled the prompt'],
  ['push_rejected', 'M-Pesa refused the prompt'],
  ['push_outcome_unknown', 'the prompt’s outcome is unknown'],
]

/** Every top-up, newest first: who, how much, how it ended, the receipt; each opens in full with what to do about it. */
export function TopUpsPage() {
  const { api } = useSession()
  const [params, setParams] = useSearchParams()
  const [typed, setTyped] = useState(params.get('q') ?? '')
  const [q, setQ] = useState(params.get('q') ?? '')
  const [status, setStatus] = useState(params.get('status') ?? '')
  const [reason, setReason] = useState(params.get('reason') ?? '')
  const [channel, setChannel] = useState(params.get('channel') ?? '')
  const [from, setFrom] = useState(params.get('from') ?? '')
  const [to, setTo] = useState(params.get('to') ?? '')
  const [claimed, setClaimed] = useState(params.get('claimed') === 'true')
  // the open top-up: one picked from the list, else one linked to (?topup= from the reconciliation or the statement)
  // that the operator has not closed; the URL mirrors it so the link can be shared
  const [picked, setPicked] = useState<string | null>(null)
  const [dismissed, setDismissed] = useState<string | null>(null)
  const linked = params.get('topup')
  const open = picked ?? (linked && linked !== dismissed ? linked : null)
  const query = useMemo(() => ({
    limit: '50', ...(q ? { q } : {}), ...(status ? { status } : {}), ...(reason ? { reason } : {}), ...(channel ? { channel } : {}),
    ...(from ? { from } : {}), ...(to ? { to } : {}), ...(claimed ? { claimed: 'true' } : {}),
  }), [q, status, reason, channel, from, to, claimed])
  const load = usePages((before, signal) =>
    api.get<{ topups: TopUpRow[]; next_before: string | null }>('/v1/staff/topups', { ...query, ...(before === null ? {} : { before }) }, { signal })
      .then((r) => ({ rows: r.topups, next: r.next_before })), [query])

  const show = useCallback((id: string | null) => {
    setPicked(id)
    if (id === null) setDismissed(params.get('topup'))
    setParams((p) => {
      const n = new URLSearchParams(p)
      if (id) n.set('topup', id)
      else n.delete('topup')
      return n
    }, { replace: true })
  }, [params, setParams])

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
      id: 'status', header: 'Outcome', accessorFn: (t) => `${t.status} ${t.failure_reason ?? ''} ${t.claim_status ?? ''}`,
      cell: ({ row }) => {
        const t = row.original
        const [tone, text] = statusLabel[t.status] ?? ['neutral', t.status]
        // a sandbox placeholder credit was never paid: M-Pesa's answer was not a payment, whatever code was stored
        const code = t.sandbox ? 'credited without M-Pesa’s confirmation (sandbox, before 30 Oct)' : t.result_code !== null ? (resultCodes[t.result_code] ?? `M-Pesa code ${t.result_code}`) : null
        const why = [code, t.failure_reason?.replaceAll('_', ' ')].filter(Boolean).join(' · ')
        const claim = t.claim_status ? claimLabel[t.claim_status] : null
        return (
          <div className="grid justify-items-start gap-1">
            <div className="flex flex-wrap gap-1">
              <Pill tone={tone}>{text}</Pill>
              {t.sandbox ? <Pill tone="pending">Sandbox</Pill> : null}
              {claim ? <Pill tone={claim[0]}>Receipt from player</Pill> : null}
            </div>
            {why ? <span className="text-sm text-muted-foreground">{why}</span> : null}
            {claim && t.claimed_receipt ? <span className="text-sm text-muted-foreground">{claim[1].replace('Receipt from player: ', '')} · <ReceiptCode value={t.claimed_receipt} /></span> : null}
          </div>
        )
      },
    },
    {
      id: 'receipt', header: 'Receipt', accessorFn: (t) => t.receipt ?? '',
      cell: ({ row }) => <Stack primary={row.original.receipt ? <ReceiptCode value={row.original.receipt} /> : <span className="font-normal text-muted-foreground">—</span>} secondary={<Mono className="text-muted-foreground">{row.original.id}</Mono>} />,
    },
    {
      id: 'actions', header: () => <span className="sr-only">Actions</span>, enableSorting: false,
      cell: ({ row }) => <Button type="button" variant="outline" size="sm" onClick={() => { show(row.original.id) }} aria-label={`Open top-up ${row.original.id}`}>Open</Button>,
    },
  ], [show])

  return (
    <section>
      <PageHeader title="Top-ups" actions={
        <Button type="button" variant="outline" disabled={!load.data?.length} onClick={() => { downloadCSV(load.data ?? [], query) }}><Download aria-hidden="true" />CSV</Button>
      }>
        Every top-up, newest first, with how it ended. Open one for M-Pesa’s callback, the player’s receipt and what to do about it.
        “Sandbox” marks a top-up credited without a payment while the Daraja sandbox was being tested (30 October).
      </PageHeader>
      <form className="mb-4 grid gap-3" onSubmit={(e) => { e.preventDefault(); setQ(typed.trim()) }}>
        <div className="flex flex-wrap items-center gap-3">
          <div className="relative w-full max-w-sm">
            <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input aria-label="Phone number, player id, payment id, checkout id or receipt" placeholder="Phone, player, payment or checkout id, receipt" value={typed} onChange={(e) => { setTyped(e.target.value) }} className="pl-8" />
          </div>
          <Button type="submit" variant="outline">Search</Button>
          {q ? <Button type="button" variant="ghost" onClick={() => { setTyped(''); setQ('') }}>Clear</Button> : null}
          <Tabs value={status} onValueChange={setStatus} className="ml-auto">
            <TabsList aria-label="Outcome" className="h-auto flex-wrap">
              {filters.map(([s, l]) => <TabsTrigger key={s} value={s} className="cursor-pointer">{l}</TabsTrigger>)}
            </TabsList>
          </Tabs>
        </div>
        <div className="flex flex-wrap items-end gap-3 text-sm">
          <div className="grid gap-1">
            <Label htmlFor="topups-from">From</Label>
            <Input id="topups-from" type="date" value={from} onChange={(e) => { setFrom(e.target.value) }} className="w-40" />
          </div>
          <div className="grid gap-1">
            <Label htmlFor="topups-to">To</Label>
            <Input id="topups-to" type="date" value={to} onChange={(e) => { setTo(e.target.value) }} className="w-40" />
          </div>
          <div className="grid gap-1">
            <Label htmlFor="topups-reason">Reason</Label>
            <Select value={reason || 'any'} onValueChange={(v) => { setReason(v === 'any' ? '' : v) }}>
              <SelectTrigger id="topups-reason" className="w-64"><SelectValue placeholder="Any" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="any">Any</SelectItem>
                {reasons.map(([v, l]) => <SelectItem key={v} value={v}>{l}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1">
            <Label htmlFor="topups-channel">Channel</Label>
            <Select value={channel || 'any'} onValueChange={(v) => { setChannel(v === 'any' ? '' : v) }}>
              <SelectTrigger id="topups-channel" className="w-40"><SelectValue placeholder="Any" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="any">Any</SelectItem>
                <SelectItem value="stk">M-Pesa prompt</SelectItem>
                <SelectItem value="c2b">Paybill</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <label className="flex items-center gap-2 pb-2">
            <Checkbox checked={claimed} onCheckedChange={(v) => { setClaimed(v === true) }} aria-label="Receipt from player to check" />
            Receipt from player to check
          </label>
        </div>
      </form>
      <Loaded load={load} what="top-ups">
        {(topups) =>
          topups.length === 0 ? <Empty icon={Smartphone}>{q || status || reason || channel || from || to || claimed ? 'No top-up matches.' : 'No top-ups yet.'}</Empty> : (
            <DataTable columns={columns} data={topups} rowId={(t) => t.id} caption="Top-ups, newest first" description="M-Pesa top-ups players started, with what M-Pesa confirmed. Open one to see its receipt, its callbacks and what you can do about it." filter="Filter these rows"
              highlight={open ?? undefined} more={{ has: load.more, loading: load.loadingMore, load: load.loadMore }} />
          )
        }
      </Loaded>
      <TopUpDetailSheet id={open} onClose={() => { show(null) }} onChanged={load.reload} />
    </section>
  )
}

/** The loaded rows as CSV, for a spreadsheet or an auditor (what is on screen, not the whole table). */
function downloadCSV(rows: TopUpRow[], query: Record<string, string>) {
  const head = ['created_at', 'payment_id', 'user_id', 'msisdn', 'full_name', 'channel', 'status', 'failure_reason', 'result_code', 'amount_requested_kes', 'amount_confirmed_kes', 'receipt', 'claimed_receipt', 'claim_status', 'updated_at']
  const esc = (v: string | number | null | undefined) => { const s = v === null || v === undefined ? '' : String(v); return /[",\n]/.test(s) ? `"${s.replaceAll('"', '""')}"` : s }
  const lines = [head.join(','), ...rows.map((t) => [t.created_at, t.id, t.user_id, t.msisdn, t.full_name, t.channel, t.status, t.failure_reason, t.result_code, t.amount_requested_kes, t.amount_confirmed_kes, t.receipt, t.claimed_receipt, t.claim_status, t.updated_at].map(esc).join(','))]
  const blob = new Blob([lines.join('\n') + '\n'], { type: 'text/csv' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  const tag = [query.status, query.from, query.to].filter(Boolean).join('-')
  a.download = `topups${tag ? `-${tag}` : ''}.csv`
  a.click()
  URL.revokeObjectURL(a.href)
}
