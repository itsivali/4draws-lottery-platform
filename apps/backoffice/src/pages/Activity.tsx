import type { ColumnDef } from '@tanstack/react-table'
import { Download, History, Search } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { ProblemError } from '../api'
import { DataTable, Stack } from '../components/DataTable'
import { Empty, Loaded, Mono, PageHeader, Pill, type Tone } from '../components/ui'
import { useSession } from '../context'
import { saveBlob } from '../download'
import { eat } from '../format'
import { useLoad, usePages } from '../hooks'
import type { AuditEvent, StaffMember } from '../types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ExportReceipt, type SavedFile } from '../components/polish'

const actors = [['', 'Everyone'], ['staff', 'Staff'], ['player', 'Players'], ['system', 'System']] as const
const actorTone: Record<AuditEvent['actor_type'], Tone> = { staff: 'pending', player: 'neutral', system: 'success' }

/** The families of actions, by the prefix every action of the family starts with. */
const actionFamilies: [string, string][] = [
  ['identity.', 'Identity (accounts, sign-ins, closures)'], ['auth.', 'Authentication'], ['sso.', 'Google and Apple sign-in'], ['user.', 'Player self-service'],
  ['kyc.', 'Identity verification'], ['payment.', 'Payments and top-ups'], ['wallet.', 'Wallet'], ['payout.', 'Payouts'], ['draw.', 'Draws'], ['game.', 'Game'],
  ['kra.', 'KRA tax'], ['alert.', 'Alerts'], ['rg.', 'Responsible gaming'], ['staff.', 'Staff members'], ['finance.', 'Finance exports'],
  ['ledger.', 'Ledger exports'], ['activity.', 'Activity exports'],
]

/** The filters as the API takes them; empty strings mean "any". */
interface Filters { q: string; actor: string; from: string; to: string; action: string; entityType: string; entityId: string; actorId: string }
const none: Filters = { q: '', actor: '', from: '', to: '', action: '', entityType: '', entityId: '', actorId: '' }

function queryOf(f: Filters): Record<string, string> {
  return {
    ...(f.q ? { q: f.q } : {}), ...(f.actor ? { actor_type: f.actor } : {}), ...(f.from ? { from: f.from } : {}), ...(f.to ? { to: f.to } : {}),
    ...(f.action ? { action: f.action } : {}), ...(f.entityType ? { entity_type: f.entityType } : {}), ...(f.entityId ? { entity_id: f.entityId } : {}),
    ...(f.actorId ? { actor_id: f.actorId } : {}),
  }
}

/** Where an entity id leads: the page that shows that thing. */
export function entityPath(entityType: string, id: string): string | null {
  switch (entityType) {
    case 'payment': case 'topup': case 'top_up': return `/topups?topup=${encodeURIComponent(id)}`
    case 'user': case 'player': case 'account': return `/players?player=${encodeURIComponent(id)}`
    case 'payout': case 'withdrawal': case 'prize': return '/payouts'
    case 'draw': return '/draws'
    case 'alert': return '/alerts'
    default: return null
  }
}

function Ref({ entityType, id, className = '' }: { entityType: string; id: string; className?: string }) {
  const to = entityPath(entityType, id)
  return to ? <Link to={to} className={className}><Mono>{id}</Mono></Link> : <Mono className={className}>{id}</Mono>
}

/** The details as readable "key: value" chips; ids that lead somewhere are links. */
function Details({ d }: { d: Record<string, unknown> }) {
  const entries = Object.entries(d)
  if (entries.length === 0) return null
  return (
    <ul className="flex max-w-2xl flex-wrap gap-1" aria-label="Details">
      {entries.map(([k, v]) => {
        const text = typeof v === 'string' ? v : typeof v === 'number' || typeof v === 'boolean' ? `${v}` : v === null || v === undefined ? '—' : JSON.stringify(v)
        const link = k === 'user_id' && typeof v === 'string' ? entityPath('user', v) : k === 'payment_id' && typeof v === 'string' ? entityPath('payment', v) : null
        return (
          <li key={k} className="rounded-md border bg-muted/50 px-1.5 py-0.5 text-xs [overflow-wrap:anywhere]">
            {link ? <Link to={link}>{k.replaceAll('_', ' ')}: {text}</Link> : <>{k.replaceAll('_', ' ')}: {text}</>}
          </li>
        )
      })}
    </ul>
  )
}

/** The audit trail, newest first: every action by staff, players and the system (administrators). */
export function ActivityPage() {
  const { api } = useSession()
  const [params] = useSearchParams() // a link from elsewhere (a player's timeline) arrives with its filters
  const [initial] = useState<Filters>(() => ({
    q: params.get('q') ?? '', actor: params.get('actor_type') ?? '', from: params.get('from') ?? '', to: params.get('to') ?? '', action: params.get('action') ?? '',
    entityType: params.get('entity_type') ?? '', entityId: params.get('entity_id') ?? '', actorId: params.get('actor_id') ?? '',
  }))
  const [draft, setDraft] = useState<Filters>(initial)
  const [applied, setApplied] = useState<Filters>(initial)
  const [exported, setExported] = useState<{ tone: 'error'; text: string } | { tone: 'success'; file: SavedFile } | null>(null)
  const [exporting, setExporting] = useState(false)
  const set = (k: keyof Filters) => (v: string) => { setDraft((d) => ({ ...d, [k]: v })) }
  const members = useLoad((signal) => api.get<{ members: StaffMember[] }>('/v1/staff/members', undefined, { signal }))
  const names = useMemo(() => new Map((members.data?.members ?? []).map((m) => [`staff:${m.id}`, m.name || m.email])), [members.data])
  const query = useMemo(() => queryOf(applied), [applied])
  const load = usePages((before, signal) =>
    api.get<{ events: AuditEvent[]; next_before: string | null }>('/v1/staff/activity', { limit: '100', ...query, ...(before === null ? {} : { before }) }, { signal })
      .then((r) => ({ rows: r.events, next: r.next_before })), [query])
  const filtered = Object.keys(query).length > 0

  const download = async () => {
    setExporting(true)
    setExported(null)
    const name = applied.from && applied.to ? `activity-${applied.from}-${applied.to}.csv` : applied.from ? `activity-from-${applied.from}.csv` : applied.to ? `activity-to-${applied.to}.csv` : 'activity-all.csv'
    try {
      const qs = new URLSearchParams(query).toString()
      const { blob, sha256, rows } = await api.download(`/v1/staff/activity/export${qs ? `?${qs}` : ''}`)
      saveBlob(blob, name)
      setExported({ tone: 'success', file: { name, sha256, rows, at: new Date(), note: 'The export itself is now in this log.' } })
    } catch (e) {
      const text = e instanceof ProblemError ? `${e.title}${e.detail ? `: ${e.detail}` : ''}` : e instanceof Error ? e.message : 'The export failed'
      setExported({ tone: 'error', text })
    } finally {
      setExporting(false)
    }
  }

  const columns = useMemo<ColumnDef<AuditEvent>[]>(() => [
    { id: 'at', header: 'When', accessorFn: (e) => e.at, cell: ({ row }) => <span className="tabular-nums text-muted-foreground">{eat(row.original.at)}</span> },
    {
      id: 'who', header: 'Who', accessorFn: (e) => `${e.actor_type} ${e.actor_id ?? ''} ${(e.actor_id && names.get(e.actor_id)) ?? ''}`,
      cell: ({ row }) => {
        const e = row.original
        const name = e.actor_id ? names.get(e.actor_id) : undefined
        let who: ReactNode = null
        if (e.actor_id) {
          who = e.actor_type === 'player' ? <Ref entityType="user" id={e.actor_id} className="text-muted-foreground" /> : <Mono className="text-muted-foreground">{e.actor_id}</Mono>
        }
        return (
          <div className="grid justify-items-start gap-1">
            <Pill tone={actorTone[e.actor_type]}><span className="capitalize">{e.actor_type}</span></Pill>
            {name ? <span className="font-medium">{name}</span> : null}
            {who}
          </div>
        )
      },
    },
    {
      id: 'what', header: 'What', accessorFn: (e) => `${e.action} ${e.entity_type} ${e.entity_id ?? ''} ${JSON.stringify(e.details)}`,
      cell: ({ row }) => {
        const e = row.original
        return (
          <Stack primary={<Mono>{e.action}</Mono>} secondary={<span>{e.entity_type}{e.entity_id ? <> <Ref entityType={e.entity_type} id={e.entity_id} /></> : null}</span>}>
            <Details d={e.details} />
          </Stack>
        )
      },
    },
  ], [names])

  const apply = () => { setApplied({ ...draft, q: draft.q.trim(), entityType: draft.entityType.trim(), entityId: draft.entityId.trim(), actorId: draft.actorId.trim() }) }

  return (
    <section>
      <PageHeader title="Activity log" actions={
        <Button type="button" variant="outline" disabled={exporting} onClick={() => void download()}>
          <Download aria-hidden="true" />{exporting ? 'Preparing…' : 'Download CSV'}
        </Button>
      }>
        Everything the platform recorded, newest first: staff decisions, player actions and the system’s own work. The
        log cannot be changed or deleted. Narrow it, then download what you see as a file; the download is recorded here too.
      </PageHeader>
      <form className="mb-4 grid gap-3" onSubmit={(e) => { e.preventDefault(); apply() }}>
        <div className="flex flex-wrap items-end gap-3">
          <div className="relative w-full max-w-sm">
            <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input aria-label="Action, id or actor" placeholder="Action (e.g. payment.topup), id or actor" value={draft.q} onChange={(e) => { set('q')(e.target.value) }} className="pl-8" />
          </div>
          <Tabs value={draft.actor} onValueChange={(v) => { setDraft((d) => ({ ...d, actor: v })); setApplied((a) => ({ ...a, actor: v })) }} className="ml-auto">
            <TabsList aria-label="Who" className="h-auto flex-wrap">
              {actors.map(([s, l]) => <TabsTrigger key={s} value={s} className="cursor-pointer">{l}</TabsTrigger>)}
            </TabsList>
          </Tabs>
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="activity-from">From</Label>
            <Input id="activity-from" type="date" value={draft.from} onChange={(e) => { set('from')(e.target.value) }} className="w-40" />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="activity-to">To</Label>
            <Input id="activity-to" type="date" value={draft.to} onChange={(e) => { set('to')(e.target.value) }} className="w-40" />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="activity-action">Action family</Label>
            <Select value={draft.action || 'any'} onValueChange={(v) => { set('action')(v === 'any' ? '' : v) }}>
              <SelectTrigger id="activity-action" className="w-64"><SelectValue placeholder="Any" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="any">Any</SelectItem>
                {actionFamilies.map(([v, l]) => <SelectItem key={v} value={v}>{l} <Mono className="text-muted-foreground">{v}</Mono></SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="activity-entity-type">Entity type</Label>
            <Input id="activity-entity-type" placeholder="payment, user, draw…" value={draft.entityType} onChange={(e) => { set('entityType')(e.target.value) }} className="w-40" />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="activity-entity-id">Entity id</Label>
            <Input id="activity-entity-id" placeholder="exact id" value={draft.entityId} onChange={(e) => { set('entityId')(e.target.value) }} className="w-72 font-mono text-[13px]" />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="activity-actor-id">Actor id</Label>
            <Input id="activity-actor-id" placeholder="player id or staff:<id>" value={draft.actorId} onChange={(e) => { set('actorId')(e.target.value) }} className="w-72 font-mono text-[13px]" />
          </div>
          <Button type="submit" variant="outline">Search</Button>
          {filtered || draft !== none ? <Button type="button" variant="ghost" onClick={() => { setDraft(none); setApplied(none) }}>Clear</Button> : null}
        </div>
      </form>
      {exported?.tone === 'success' ? <ExportReceipt file={exported.file} className="mb-6 max-w-xl" /> : null}
      {exported?.tone === 'error' ? <p role="status" className="mb-4 rounded-2xl border border-destructive/40 bg-destructive/10 px-4 py-3 text-[14.5px] font-medium text-destructive">{exported.text}</p> : null}
      <Loaded load={load} what="activity">
        {(events) =>
          events.length === 0 ? <Empty icon={History}>{filtered ? 'Nothing matches.' : 'Nothing recorded yet.'}</Empty> : (
            <DataTable columns={columns} data={events} rowId={(e) => e.id} caption="Activity, newest first" description="Every action recorded in the audit log, by staff, players and the system, with what it was about." filter="Filter these rows"
              more={{ has: load.more, loading: load.loadingMore, load: load.loadMore }} />
          )
        }
      </Loaded>
    </section>
  )
}
