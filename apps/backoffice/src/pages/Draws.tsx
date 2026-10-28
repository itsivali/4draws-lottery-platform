import type { ColumnDef } from '@tanstack/react-table'
import { Ticket, TriangleAlert } from 'lucide-react'
import { useMemo } from 'react'
import { useSession } from '../context'
import { Actions, DataTable, Stack } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, PageHeader, Pill, type Tone } from '../components/ui'
import { eat, kes } from '../format'
import { useLoad, usePages } from '../hooks'
import { can } from '../roles'
import type { Draw, Late } from '../types'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

const statusTone: Record<Draw['status'], [Tone, string]> = {
  open: ['neutral', 'Open'],
  frozen: ['pending', 'Frozen · needs 2 signatures'],
  signed: ['pending', 'Signed · awaiting the round'],
  conducted: ['pending', 'Conducted · needs 2 approvals'],
  approved: ['success', 'Approved'],
  paid: ['success', 'Paid'],
  voided: ['error', 'Voided'],
}

const period: Record<Draw['draw_type'], string> = { daily: 'day', weekly: 'week', monthly: 'month', quarterly: 'quarter' }

function count(d: Draw, step: 'sign' | 'approve') {
  return d.signoffs.filter((s) => s.step === step).length
}

export function DrawsPage() {
  const { api, me } = useSession()
  // newest period first, 50 a page; "Load more" follows the server's cursor back in time
  const draws = usePages((before, signal) =>
    api.get<{ draws: Draw[]; next_before: string | null }>('/v1/staff/draws', { limit: '50', ...(before === null ? {} : { before }) }, { signal })
      .then((r) => ({ rows: r.draws, next: r.next_before })))
  const overdue = useLoad((signal) => api.get<{ overdue: Late[] }>('/v1/staff/draws/overdue', undefined, { signal }))
  const reload = () => { draws.reload(); overdue.reload() }
  const { ask, busy, view } = useConfirmedAction(reload)
  const official = can(me.roles, 'staff')

  const columns = useMemo<ColumnDef<Draw>[]>(() => [
    {
      id: 'draw',
      header: 'Draw',
      accessorFn: (d) => `${d.draw_type} ${d.period_start} ${d.period_end}`,
      cell: ({ row }) => {
        const d = row.original
        return (
          <Stack primary={<span className="capitalize">{d.draw_type}</span>} secondary={<span className="tabular-nums">{d.period_start}{d.period_end !== d.period_start ? ` – ${d.period_end}` : ''}</span>}>
            {d.basis === 'personal' ? <span className="text-sm text-muted-foreground">players whose own {period[d.draw_type]} ends {d.period_end}</span> : null}
          </Stack>
        )
      },
    },
    {
      id: 'status',
      header: 'Status',
      accessorFn: (d) => statusTone[d.status][1],
      cell: ({ row }) => {
        const d = row.original
        const [tone, label] = statusTone[d.status]
        return (
          <div className="grid justify-items-start gap-1">
            <Pill tone={tone}>{label}</Pill>
            {d.status === 'frozen' ? <span className="text-sm text-muted-foreground">{count(d, 'sign')} of 2 signatures</span> : null}
            {d.status === 'conducted' ? <span className="text-sm text-muted-foreground">{count(d, 'approve')} of 2 approvals</span> : null}
            {d.voided_reason ? <span className="text-sm text-muted-foreground">{d.voided_reason}</span> : null}
          </div>
        )
      },
    },
    { id: 'entries', header: 'Entries', accessorFn: (d) => d.entries, meta: { className: 'text-right tabular-nums' }, cell: ({ row }) => (row.original.status === 'open' ? '—' : row.original.entries.toLocaleString('en-KE')) },
    {
      id: 'results',
      header: 'Results',
      enableSorting: false,
      accessorFn: (d) => d.results.map((r) => `#${r.rank} entry ${r.entry_no}: ${kes(r.amount_kes)}`).join('; '),
      cell: ({ row }) => (row.original.results.length === 0 ? '—' : <ul className="m-0 grid list-none gap-0.5 p-0 text-sm">{row.original.results.map((r) => <li key={r.rank} className="tabular-nums">#{r.rank} entry {r.entry_no}: <span className="font-medium">{kes(r.amount_kes)}</span></li>)}</ul>),
    },
    {
      id: 'actions',
      header: () => <span className="sr-only">Actions</span>,
      enableSorting: false,
      cell: ({ row }) => {
        const d = row.original
        if (!official) return null
        return (
          <Actions>
            {d.status === 'frozen' ? (
              <Button type="button" size="sm" disabled={busy} onClick={() => {
                ask({
                  title: `Sign the ${d.draw_type} draw of ${d.period_start}?`,
                  body: <p>You attest to the entry list hash, the seed commitment and the drand round fixed at freeze. A second official must also sign.</p>,
                  confirmLabel: 'Sign',
                  success: 'Signature recorded.',
                  act: () => api.post(`/v1/staff/draws/${d.id}/sign`),
                })
              }}>Sign</Button>
            ) : null}
            {d.status === 'conducted' ? (
              <Button type="button" size="sm" disabled={busy} onClick={() => {
                ask({
                  title: `Approve the results of the ${d.draw_type} draw of ${d.period_start}?`,
                  body: <p>Approval by two officials makes the results final and funds the prizes.</p>,
                  confirmLabel: 'Approve',
                  success: 'Approval recorded.',
                  act: () => api.post(`/v1/staff/draws/${d.id}/approve`),
                })
              }}>Approve</Button>
            ) : null}
            {d.status === 'frozen' || d.status === 'signed' ? (
              <Button type="button" size="sm" variant="outline" className="border-destructive/50 text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={busy} onClick={() => {
                ask({
                  title: `Void the ${d.draw_type} draw of ${d.period_start}?`,
                  body: <p>The draw is cancelled before its round and re-run as a new draw with a new seed.</p>,
                  fields: [{ name: 'reason', label: 'Reason', required: true }],
                  confirmLabel: 'Void and re-run',
                  danger: true,
                  success: 'The draw was voided and re-run.',
                  act: (v) => api.post(`/v1/staff/draws/${d.id}/void`, { reason: v.reason }),
                })
              }}>Void</Button>
            ) : null}
          </Actions>
        )
      },
    },
  ], [api, ask, busy, official])

  return (
    <section>
      <PageHeader title="Draws">
        A frozen draw needs two different officials to sign before its drand round; a conducted draw needs two to approve.
        You cannot sign the same step twice.
      </PageHeader>
      {view}
      <Loaded load={overdue} what="late draws" skeleton={null}>
        {({ overdue: late }) =>
          late.length === 0 ? null : (
            <Alert variant="destructive" className="my-4">
              <TriangleAlert />
              <AlertTitle>{late.length} draw step{late.length > 1 ? 's are' : ' is'} late</AlertTitle>
              <AlertDescription>
                <ul className="m-0 grid list-disc gap-0.5 pl-4">{late.map((l) => <li key={l.draw_id + l.problem}>{l.draw_type} {l.period_start}: {l.problem} (due {eat(l.due)})</li>)}</ul>
              </AlertDescription>
            </Alert>
          )
        }
      </Loaded>
      <Loaded load={draws} what="draws">
        {(list) =>
          list.length === 0 ? <Empty icon={Ticket}>No draws yet.</Empty> : (
            <DataTable columns={columns} data={list} rowId={(d) => d.id} caption="Draws, newest first" filter="Search draws"
              more={{ has: draws.more, loading: draws.loadingMore, load: draws.loadMore }} />
          )
        }
      </Loaded>
    </section>
  )
}
