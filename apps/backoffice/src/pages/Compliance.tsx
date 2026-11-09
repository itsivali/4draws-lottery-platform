import type { ColumnDef } from '@tanstack/react-table'
import { ShieldAlert } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useSession } from '../context'
import { Actions, DataTable, Stack } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, Mono, PageHeader, Pill } from '../components/ui'
import { eat, msisdn } from '../format'
import { useLoad } from '../hooks'
import type { AmlCase, AmlRule } from '../types'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

export const ruleLabels: Record<AmlCase['rule'], string> = {
  deposit_velocity: 'Much money in, quickly',
  in_out_little_play: 'Money in and out, little play',
  failed_topups: 'Many failed top-ups',
  shared_id_number: 'ID number on several accounts',
  shared_device: 'Device on several accounts',
  manual: 'Opened by staff',
}

const views = [['open', 'Open'], ['held', 'Money held'], ['closed', 'Closed']] as const

/** What a rule saw, in a line. */
function evidence(c: AmlCase): string {
  const e = c.evidence
  const n = (k: string) => (typeof e[k] === 'number' ? e[k].toLocaleString('en-KE') : '')
  switch (c.rule) {
    case 'deposit_velocity': return `KES ${n('deposited_kes')} in ${n('deposits')} top-ups`
    case 'in_out_little_play': return `KES ${n('deposited_kes')} in, KES ${n('withdrawn_kes')} out, KES ${n('staked_kes')} played`
    case 'failed_topups': return `${n('failed_topups')} failed top-ups`
    case 'shared_id_number':
    case 'shared_device': return `${Array.isArray(e.other_accounts) ? e.other_accounts.length : 0} other account(s)`
    default: return typeof e.reason === 'string' ? e.reason : ''
  }
}

/**
 * Transaction monitoring (ADR 0033): cases the rules raised or staff opened, the holds on players' money, and the
 * rules as they stand. A foundation for the compliance owner's AML programme, not a claim that it satisfies the law.
 */
export function CompliancePage() {
  const { api, me } = useSession()
  const [view, setView] = useState<string>('open')
  const load = useLoad((signal) => api.get<{ cases: AmlCase[]; next_after: string | null }>('/v1/staff/aml/cases', { view, limit: '200' }, { signal }), [view])
  const rules = useLoad((signal) => api.get<{ rules: AmlRule[] }>('/v1/staff/aml/rules', undefined, { signal }))
  const { ask, busy, view: dialog } = useConfirmedAction(load.reload)
  const mine = `staff:${me.id}`

  const columns = useMemo<ColumnDef<AmlCase>[]>(() => [
    {
      id: 'player', header: 'Player', accessorFn: (c) => `${c.msisdn ?? ''} ${c.user_id}`,
      cell: ({ row }) => <Stack primary={<Link className="tabular-nums underline-offset-4 hover:underline" to={`/players?player=${row.original.user_id}`}>{row.original.msisdn ? msisdn(row.original.msisdn) : row.original.user_id}</Link>} secondary={<Mono>{row.original.id}</Mono>} />,
    },
    { id: 'rule', header: 'Why', accessorFn: (c) => ruleLabels[c.rule], cell: ({ row }) => <Stack primary={ruleLabels[row.original.rule]} secondary={evidence(row.original)} /> },
    {
      id: 'state', header: 'State', accessorFn: (c) => (c.hold ? 'held' : c.status),
      cell: ({ row }) => {
        const c = row.original
        if (c.status === 'closed') return <Stack primary={<Pill tone="neutral">closed</Pill>} secondary={c.outcome} />
        return c.hold ? <Stack primary={<Pill tone="error">money held</Pill>} secondary={<>by <Mono>{c.hold_by}</Mono></>} /> : <Pill tone="pending">open</Pill>
      },
    },
    { id: 'opened', header: 'Opened', accessorFn: (c) => c.opened_at, cell: ({ row }) => <span className="text-muted-foreground">{eat(row.original.opened_at)}</span> },
    {
      id: 'actions', header: () => <span className="sr-only">Actions</span>, enableSorting: false,
      cell: ({ row }) => {
        const c = row.original
        if (c.status === 'closed') return null
        const note = (label: string) => [{ name: 'note', label, required: true }]
        const heldByMe = c.hold && c.hold_by === mine
        return (
          <Actions>
            {c.hold ? (
              <Button type="button" size="sm" variant="outline" disabled={busy || heldByMe} title={heldByMe ? 'Someone else lifts a hold you placed' : undefined} onClick={() => {
                ask({ title: 'Lift the hold?', body: <p>The player can top up and withdraw again.</p>, fields: note('Why (goes in the case and the audit log)'),
                  confirmLabel: 'Lift the hold', success: 'Hold lifted.', act: (v) => api.post(`/v1/staff/aml/cases/${c.id}/release`, { note: v.note }) })
              }}>Lift hold</Button>
            ) : (
              <Button type="button" size="sm" variant="outline" className="border-destructive/50 text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={busy} onClick={() => {
                ask({ title: 'Hold this player’s money?', body: <p>No top-ups, paybill payments, withdrawals or payouts until someone else lifts it. The player is only told it is not available right now: never tell them why.</p>,
                  fields: note('Why (goes in the case and the audit log)'), confirmLabel: 'Hold the money', danger: true, success: 'Money held.',
                  act: (v) => api.post(`/v1/staff/aml/cases/${c.id}/hold`, { note: v.note }) })
              }}>Hold money</Button>
            )}
            <Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => {
              ask({ title: 'Add a note', body: <p>What was found or done.</p>, fields: note('Note'), confirmLabel: 'Add note', success: 'Note added.',
                act: (v) => api.post(`/v1/staff/aml/cases/${c.id}/note`, { note: v.note }) })
            }}>Note</Button>
            <Button type="button" size="sm" disabled={busy || heldByMe} title={heldByMe ? 'Someone else closes a case whose money you held' : undefined} onClick={() => {
              ask({ title: 'Close the case?', body: <p>Record what was decided, and whether it was reported to the Financial Reporting Centre. Closing lifts any hold. A closed case is final.</p>,
                fields: note('Outcome'), confirmLabel: 'Close case', success: 'Case closed.', act: (v) => api.post(`/v1/staff/aml/cases/${c.id}/close`, { note: v.note }) })
            }}>Close</Button>
          </Actions>
        )
      },
    },
  ], [api, ask, busy, mine])

  return (
    <section>
      <PageHeader title="Compliance">
        Transaction monitoring raises cases about players for you to look into. Hold a player’s money while you do; someone else lifts the hold.
        The rules and their thresholds are the compliance owner’s decision: until approved they are placeholders.
      </PageHeader>
      {dialog}
      <Tabs value={view} onValueChange={setView}>
        <TabsList aria-label="Cases" className="h-auto flex-wrap">
          {views.map(([v, label]) => <TabsTrigger key={v} value={v} className="cursor-pointer">{label}</TabsTrigger>)}
        </TabsList>
      </Tabs>
      <div className="mt-3">
        <Loaded load={load} what="cases">
          {({ cases }) => cases.length === 0 ? <Empty icon={ShieldAlert}>No cases here.</Empty> : (
            <DataTable columns={columns} data={cases} rowId={(c) => c.id} caption="Monitoring cases, oldest first" filter="Search by phone, id or reason" />
          )}
        </Loaded>
      </div>
      <h2 className="mt-8 text-base font-semibold">Rules</h2>
      <Loaded load={rules} what="rules">
        {({ rules: list }) => (
          <ul className="mt-2 grid gap-2">
            {list.map((r) => (
              <li key={r.rule} className="flex flex-wrap items-center gap-2 rounded-xl border px-3 py-2 text-sm">
                <span className="font-medium">{ruleLabels[r.rule]}</span>
                <Pill tone={r.enabled ? 'success' : 'neutral'}>{r.enabled ? 'on' : 'off'}</Pill>
                {r.pending_approval ? <Pill tone="warning">pending compliance-owner approval</Pill> : <Pill tone="success">approved: {r.approval_reference}</Pill>}
                <Mono>{Object.entries(r.params).map(([k, v]) => `${k}=${String(v)}`).join(' ')}</Mono>
              </li>
            ))}
          </ul>
        )}
      </Loaded>
    </section>
  )
}
