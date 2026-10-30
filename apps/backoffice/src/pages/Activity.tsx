import type { ColumnDef } from '@tanstack/react-table'
import { History, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { DataTable, Stack } from '../components/DataTable'
import { Empty, Loaded, Mono, PageHeader, Pill, type Tone } from '../components/ui'
import { useSession } from '../context'
import { eat } from '../format'
import { usePages } from '../hooks'
import type { AuditEvent } from '../types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

const actors = [['', 'Everyone'], ['staff', 'Staff'], ['player', 'Players'], ['system', 'System']] as const
const actorTone: Record<AuditEvent['actor_type'], Tone> = { staff: 'pending', player: 'neutral', system: 'success' }

function details(d: Record<string, unknown>): string {
  const parts = Object.entries(d).map(([k, v]) => `${k.replaceAll('_', ' ')}: ${typeof v === 'string' ? v : JSON.stringify(v)}`)
  return parts.join(' · ')
}

/** The audit trail, newest first: every action by staff, players and the system (administrators). */
export function ActivityPage() {
  const { api } = useSession()
  const [typed, setTyped] = useState('')
  const [q, setQ] = useState('')
  const [actor, setActor] = useState('')
  const load = usePages((before, signal) =>
    api.get<{ events: AuditEvent[]; next_before: string | null }>('/v1/staff/activity',
      { limit: '100', ...(q ? { q } : {}), ...(actor ? { actor_type: actor } : {}), ...(before === null ? {} : { before }) }, { signal })
      .then((r) => ({ rows: r.events, next: r.next_before })), [q, actor])

  const columns = useMemo<ColumnDef<AuditEvent>[]>(() => [
    { id: 'at', header: 'When', accessorFn: (e) => e.at, cell: ({ row }) => <span className="tabular-nums text-muted-foreground">{eat(row.original.at)}</span> },
    {
      id: 'who', header: 'Who', accessorFn: (e) => `${e.actor_type} ${e.actor_id ?? ''}`,
      cell: ({ row }) => <div className="grid justify-items-start gap-1"><Pill tone={actorTone[row.original.actor_type]}><span className="capitalize">{row.original.actor_type}</span></Pill>{row.original.actor_id ? <Mono className="text-muted-foreground">{row.original.actor_id}</Mono> : null}</div>,
    },
    {
      id: 'what', header: 'What', accessorFn: (e) => `${e.action} ${e.entity_type} ${e.entity_id ?? ''} ${JSON.stringify(e.details)}`,
      cell: ({ row }) => {
        const e = row.original
        const d = details(e.details)
        return (
          <Stack primary={<Mono>{e.action}</Mono>} secondary={<span>{e.entity_type}{e.entity_id ? <> <Mono>{e.entity_id}</Mono></> : null}</span>}>
            {d ? <span className="block max-w-xl text-sm text-muted-foreground [overflow-wrap:anywhere]">{d}</span> : null}
          </Stack>
        )
      },
    },
  ], [])

  return (
    <section>
      <PageHeader title="Activity log">
        Everything the platform recorded, newest first: staff decisions, player actions and the system’s own work. The
        log cannot be changed or deleted.
      </PageHeader>
      <form className="mb-4 flex flex-wrap items-center gap-3" onSubmit={(e) => { e.preventDefault(); setQ(typed.trim()) }}>
        <div className="relative w-full max-w-sm">
          <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label="Action, id or actor" placeholder="Action (e.g. payment.topup), id or actor" value={typed} onChange={(e) => { setTyped(e.target.value) }} className="pl-8" />
        </div>
        <Button type="submit" variant="outline">Search</Button>
        {q ? <Button type="button" variant="ghost" onClick={() => { setTyped(''); setQ('') }}>Clear</Button> : null}
        <Tabs value={actor} onValueChange={setActor} className="ml-auto">
          <TabsList aria-label="Who" className="h-auto flex-wrap">
            {actors.map(([s, l]) => <TabsTrigger key={s} value={s} className="cursor-pointer">{l}</TabsTrigger>)}
          </TabsList>
        </Tabs>
      </form>
      <Loaded load={load} what="activity">
        {(events) =>
          events.length === 0 ? <Empty icon={History}>{q || actor ? 'Nothing matches.' : 'Nothing recorded yet.'}</Empty> : (
            <DataTable columns={columns} data={events} rowId={(e) => e.id} caption="Activity, newest first" filter="Filter these rows"
              more={{ has: load.more, loading: load.loadingMore, load: load.loadMore }} />
          )
        }
      </Loaded>
    </section>
  )
}
