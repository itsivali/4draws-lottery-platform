import type { ColumnDef } from '@tanstack/react-table'
import { Download, KeyRound, Search, Undo2, UserRound, UserRoundCheck, UserRoundX } from 'lucide-react'
import { useCallback, useMemo, useState, type ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { ProblemError } from '../api'
import { Actions, DataTable, Stack } from '../components/DataTable'
import { useConfirmedAction, type ActionSpec } from '../components/useConfirmedAction'
import { Empty, Loaded, Mono, PageHeader, Pill, TableSkeleton, type Tone } from '../components/ui'
import { useSession } from '../context'
import { saveBlob } from '../download'
import { eat, kes, msisdn } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { AuditEvent, Player, PlayerTimeline } from '../types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

const statuses = [['', 'All'], ['active', 'Active'], ['suspended', 'Suspended'], ['closed', 'Closed']] as const

const statusTone: Record<Player['status'], Tone> = { active: 'success', suspended: 'warning', closed: 'neutral' }

const kycLabel: Record<Player['kyc_status'], [Tone, string]> = {
  none: ['neutral', 'No ID yet'],
  pending: ['pending', 'ID pending'],
  manual_review: ['pending', 'ID in review'],
  verified: ['success', 'ID verified'],
  rejected: ['error', 'ID rejected'],
}

function StatusPills({ p }: { p: Player }) {
  const [tone, label] = kycLabel[p.kyc_status]
  return (
    <div className="flex flex-wrap gap-1">
      <Pill tone={statusTone[p.status]}><span className="capitalize">{p.status}</span></Pill>
      <Pill tone={tone}>{label}</Pill>
      {p.phone_verified ? null : <Pill tone="neutral">Phone not proved</Pill>}
      {p.closure_requested_at ? <Pill tone="pending">Asked to close</Pill> : null}
    </div>
  )
}

/** The actions on a player (ADR 0019), asked for with a reason that goes in the audit log. */
function playerActions(p: Player, post: (path: string, body: unknown) => Promise<unknown>, ask: (s: ActionSpec) => void, busy: boolean): ReactNode {
  if (p.status === 'closed') return null
  const who = msisdn(p.msisdn)
  return (
    <>
      {p.status === 'active' ? (
        <Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => {
          ask({
            title: `Temporary password for ${who}`,
            body: <p>First check the caller against the name and ID number shown here. Tell them the password by phone, never by SMS or chat. Every session of theirs ends now, and the app makes them choose their own password when they next sign in.</p>,
            fields: [
              { name: 'reason', label: 'Reason (goes in the audit log)', required: true },
              { name: 'password', label: 'Temporary password', type: 'password', required: true, help: 'A short sentence works best. It is never stored in clear.' },
            ],
            confirmLabel: 'Set temporary password', success: `${who} has a temporary password.`,
            act: (v) => post(`/v1/staff/players/${p.user_id}/password`, { password: v.password, reason: v.reason }),
          })
        }}><KeyRound aria-hidden="true" /> Temporary password</Button>
      ) : null}
      {p.status === 'active' ? (
        <Button type="button" size="sm" variant="outline" className="border-destructive/50 text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={busy} onClick={() => {
          ask({
            title: `Suspend ${who}?`,
            body: <p>Sign-in, play, top-ups and withdrawals stop and every session ends. The balance stays in the wallet. You can reinstate the account later.</p>,
            fields: [{ name: 'reason', label: 'Reason (goes in the audit log)', required: true }],
            confirmLabel: 'Suspend', danger: true, success: `${who} is suspended.`,
            act: (v) => post(`/v1/staff/players/${p.user_id}/suspend`, { reason: v.reason }),
          })
        }}><UserRoundX aria-hidden="true" /> Suspend</Button>
      ) : (
        <Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => {
          ask({
            title: `Reinstate ${who}?`,
            body: <p>The player can sign in, play, top up and withdraw again.</p>,
            fields: [{ name: 'reason', label: 'Reason (goes in the audit log)', required: true }],
            confirmLabel: 'Reinstate', success: `${who} is active again.`,
            act: (v) => post(`/v1/staff/players/${p.user_id}/reinstate`, { reason: v.reason }),
          })
        }}><UserRoundCheck aria-hidden="true" /> Reinstate</Button>
      )}
    </>
  )
}

/**
 * Refund to source (ADR 0031): a balance the account cannot withdraw goes back to the number that paid it in. Admins
 * ask; the payout waits in Payouts for a second staff member to approve.
 */
function refundAction(p: PlayerTimeline['player'], balance: number, post: (path: string, body: unknown) => Promise<unknown>, ask: (s: ActionSpec) => void, busy: boolean): ReactNode {
  if (p.status === 'closed' || balance <= 0) return null
  return (
    <Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => {
      ask({
        title: `Refund ${kes(balance)} to source?`,
        body: <p>For a balance this account cannot withdraw (suspended, identity refused or never verified). The whole balance goes back to the number that paid the latest top-up, as a withdrawal (the withdrawal tax applies). It waits in Payouts until someone else approves it; rejecting it there puts the money back.</p>,
        fields: [{ name: 'reason', label: 'Reason (goes on the payout and in the audit log)', required: true }],
        confirmLabel: 'Ask for the refund', danger: true, success: 'Refund asked for. A second person approves it in Payouts.',
        act: (v) => post(`/v1/staff/players/${p.user_id}/refund`, { reason: v.reason }),
      })
    }}><Undo2 aria-hidden="true" /> Refund to source</Button>
  )
}

/** Every registered player, newest first, with search, a status filter, suspension (ADR 0019) and each one's timeline. */
export function PlayersPage() {
  const { api } = useSession()
  const [params, setParams] = useSearchParams()
  const [typed, setTyped] = useState('')
  const [q, setQ] = useState('')
  const [status, setStatus] = useState<string>('')
  const load = useLoad((signal) => api.get<{ players: Player[]; next_before: string | null }>('/v1/staff/players',
    { limit: '100', ...(q ? { q } : {}), ...(status ? { status } : {}) }, { signal }), [q, status])
  // the open player: one picked from the list, else one linked to (?player= from the activity log) that the operator
  // has not closed; the URL mirrors it so the link can be shared
  const [picked, setPicked] = useState<string | null>(null)
  const [dismissed, setDismissed] = useState<string | null>(null)
  const linked = params.get('player')
  const open = picked ?? (linked && linked !== dismissed ? linked : null)
  const show = useCallback((id: string | null) => {
    setPicked(id)
    if (id === null) setDismissed(params.get('player'))
    setParams((p) => {
      const n = new URLSearchParams(p)
      if (id) n.set('player', id)
      else n.delete('player')
      return n
    }, { replace: true })
  }, [params, setParams])
  const [detailTick, setDetailTick] = useState(0)
  const reloadList = load.reload
  const reload = useCallback(() => { reloadList(); setDetailTick((t) => t + 1) }, [reloadList])
  const { ask, busy, view } = useConfirmedAction(reload)
  const post = useCallback((path: string, body: unknown) => api.post(path, body), [api])

  const columns = useMemo<ColumnDef<Player>[]>(() => [
    {
      id: 'player', header: 'Player', accessorFn: (p) => `${p.full_name} ${msisdn(p.msisdn)} ${p.email ?? ''} ${p.user_id}`,
      cell: ({ row }) => (
        <Stack primary={row.original.full_name || <span className="font-normal text-muted-foreground">No name yet</span>}
          secondary={<span><span className="tabular-nums">{msisdn(row.original.msisdn)}</span>{row.original.email ? <span className="text-muted-foreground"> · {row.original.email}</span> : null}</span>}>
          <Mono className="text-muted-foreground">{row.original.user_id}</Mono>
        </Stack>
      ),
    },
    {
      id: 'id', header: 'National ID', accessorFn: (p) => p.national_id,
      cell: ({ row }) => (row.original.national_id ? <Mono>{row.original.national_id}</Mono> : <span className="text-sm text-muted-foreground">—</span>),
    },
    { id: 'status', header: 'Status', accessorFn: (p) => `${p.status} ${p.kyc_status}`, cell: ({ row }) => <StatusPills p={row.original} /> },
    { id: 'balance', header: 'Balance', accessorFn: (p) => p.balance_kes, meta: { className: 'text-right tabular-nums' }, cell: ({ row }) => kes(row.original.balance_kes) },
    { id: 'plays', header: 'Plays', accessorFn: (p) => p.plays, meta: { className: 'text-right tabular-nums' }, cell: ({ row }) => row.original.plays.toLocaleString('en-KE') },
    {
      id: 'seen', header: 'Joined · last sign-in', accessorFn: (p) => p.created_at,
      cell: ({ row }) => <Stack primary={<span className="font-normal">{eat(row.original.created_at)}</span>} secondary={row.original.last_sign_in_at ? `last ${eat(row.original.last_sign_in_at)}` : 'never signed in'} />,
    },
    {
      id: 'actions', header: () => <span className="sr-only">Actions</span>, enableSorting: false,
      cell: ({ row }) => (
        <Actions>
          <Button type="button" size="sm" variant="secondary" onClick={() => { show(row.original.user_id) }} aria-label={`Open ${msisdn(row.original.msisdn)}`}>Open</Button>
          {playerActions(row.original, post, ask, busy)}
        </Actions>
      ),
    },
  ], [post, ask, busy, show])

  return (
    <section>
      <PageHeader title="Players">
        Every registered account, newest first. Identity decisions are made on the Identity review page and closures on
        Account closures; here you can look a player up, open their whole story (wallet, top-ups, plays, payouts,
        identity, limits and the audit records about them), and suspend or reinstate them. Names and ID numbers come
        from the player’s latest identity submission. Each view is recorded in the audit log, because it shows personal data.
      </PageHeader>
      <form className="mb-4 flex flex-wrap items-center gap-3" onSubmit={(e) => { e.preventDefault(); setQ(typed.trim()) }}>
        <div className="relative w-full max-w-sm">
          <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label="Phone number, email or user id" placeholder="Phone number, email or user id" value={typed} onChange={(e) => { setTyped(e.target.value) }} className="pl-8" />
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
              <DataTable columns={columns} data={players} rowId={(p) => p.user_id} caption="Players, newest first" filter="Filter these rows" pageSize={25} highlight={open ?? undefined} />
              {next_before ? <p className="mt-3 text-sm text-muted-foreground">Showing the newest 100. Search by phone number, email or user id to find an older player.</p> : null}
            </>
          )
        }
      </Loaded>
      <Sheet open={open !== null} onOpenChange={(o) => { if (!o) show(null) }}>
        <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-3xl">
          {open ? <PlayerSheet id={open} tick={detailTick} post={post} ask={ask} busy={busy} /> : null}
        </SheetContent>
      </Sheet>
    </section>
  )
}

const txKind: Record<string, string> = {
  deposit: 'Top-up', deposit_excise: 'Excise on top-up', stake: 'Play', withdrawal: 'Withdrawal', prize: 'Prize', refund: 'Refund', adjustment: 'Adjustment', reversal: 'Reversal',
}
const payoutTone = (state: string): Tone => state === 'succeeded' || state === 'paid' ? 'success' : state === 'failed' || state === 'rejected' ? 'error' : 'pending'
const topUpTone = (status: string): Tone => status === 'credited' ? 'success' : status === 'failed' || status === 'cancelled' || status === 'expired' || status === 'reversed' ? 'error' : 'pending'

function When({ at }: { at: string }) { return <span className="tabular-nums text-muted-foreground">{eat(at)}</span> }

function Rows({ children, caption }: { children: ReactNode; caption: string }) {
  return (
    <div className="overflow-hidden rounded-xl border bg-card">
      <table className="w-full text-sm">
        <caption className="sr-only">{caption}</caption>
        <tbody className="divide-y">{children}</tbody>
      </table>
    </div>
  )
}

function Nothing({ children }: { children: ReactNode }) { return <p className="py-6 text-center text-sm text-muted-foreground">{children}</p> }

/** One player's timeline, loaded when the sheet opens; tick reloads it after an action. */
function PlayerSheet({ id, tick, post, ask, busy }: { id: string; tick: number; post: (path: string, body: unknown) => Promise<unknown>; ask: (s: ActionSpec) => void; busy: boolean }) {
  const { api, me } = useSession()
  const admin = can(me.roles, 'admin')
  const load = useLoad((signal) => api.get<PlayerTimeline>(`/v1/staff/players/${encodeURIComponent(id)}`, undefined, { signal }), [id, tick])
  const [exported, setExported] = useState<{ tone: Tone; text: string } | null>(null)
  const [exporting, setExporting] = useState(false)
  const download = async () => {
    setExporting(true)
    setExported(null)
    const name = `player-${id}.csv`
    try {
      const { blob, sha256 } = await api.download(`/v1/staff/players/${encodeURIComponent(id)}/export`)
      saveBlob(blob, name)
      setExported({ tone: 'success', text: `Saved ${name}${sha256 ? ` · SHA-256 ${sha256}` : ''}. The export is in the activity log.` })
    } catch (e) {
      setExported({ tone: 'error', text: e instanceof ProblemError ? `${e.title}${e.detail ? `: ${e.detail}` : ''}` : e instanceof Error ? e.message : 'The export failed' })
    } finally {
      setExporting(false)
    }
  }
  const t = load.data
  const p = t?.player
  return (
    <div className="grid gap-5 p-6">
      <SheetHeader className="p-0">
        <SheetTitle className="font-display text-2xl">{p ? p.full_name || 'No name yet' : 'Player'}</SheetTitle>
        {t && p ? (
          <SheetDescription asChild>
            <div className="grid gap-2 text-sm">
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <span className="tabular-nums text-foreground">{msisdn(p.msisdn)}</span>
                {p.email ? <span>{p.email}</span> : null}
                {p.national_id ? <span>ID <Mono>{p.national_id}</Mono></span> : null}
                <Mono>{p.user_id}</Mono>
              </div>
              <StatusPills p={p} />
              <div className="flex flex-wrap gap-x-6 gap-y-1">
                <span>Balance <strong className="tabular-nums text-foreground">{kes(t.wallet.balance_kes)}</strong></span>
                <span>Owes <strong className={`tabular-nums ${t.wallet.debt_kes > 0 ? 'text-destructive' : 'text-foreground'}`}>{kes(t.wallet.debt_kes)}</strong></span>
                <span>{t.plays.count.toLocaleString('en-KE')} plays</span>
                <span>Joined {eat(p.created_at)}</span>
                {p.last_sign_in_at ? <span>Last sign-in {eat(p.last_sign_in_at)}</span> : <span>Never signed in</span>}
              </div>
            </div>
          </SheetDescription>
        ) : <SheetDescription>The player’s timeline: wallet, top-ups, plays, payouts, identity, limits and audit records.</SheetDescription>}
      </SheetHeader>
      <Loaded load={load} what="the player" skeleton={<TableSkeleton />}>
        {(t) => {
          const p = t.player
          return (
          <>
            <div className="flex flex-wrap gap-2">
              {playerActions(p, post, ask, busy)}
              {admin ? refundAction(p, t.wallet.balance_kes, post, ask, busy) : null}
              <Button type="button" size="sm" variant="outline" disabled={exporting} onClick={() => void download()}>
                <Download aria-hidden="true" />{exporting ? 'Preparing…' : 'Download player pack (CSV)'}
              </Button>
            </div>
            {exported ? <p role="status" className={`text-sm ${exported.tone === 'error' ? 'text-destructive' : 'text-muted-foreground'}`}>{exported.text}</p> : null}
            <Tabs defaultValue="wallet">
              <TabsList aria-label="Sections" className="h-auto flex-wrap">
                <TabsTrigger value="wallet">Wallet</TabsTrigger>
                <TabsTrigger value="topups">Top-ups</TabsTrigger>
                <TabsTrigger value="plays">Plays</TabsTrigger>
                <TabsTrigger value="payouts">Payouts</TabsTrigger>
                <TabsTrigger value="identity">Identity &amp; sessions</TabsTrigger>
                <TabsTrigger value="limits">Limits</TabsTrigger>
                <TabsTrigger value="activity">Activity</TabsTrigger>
              </TabsList>
              <TabsContent value="wallet" className="mt-3 grid gap-2">
                <p className="text-sm text-muted-foreground">The last {t.wallet.transactions.length} movements; credits positive, debits negative. The full list is in the player pack.</p>
                {t.wallet.transactions.length === 0 ? <Nothing>No money has moved yet.</Nothing> : (
                  <Rows caption="Wallet movements">
                    {t.wallet.transactions.map((x) => (
                      <tr key={x.id} className="align-top">
                        <td className="px-3 py-2"><When at={x.created_at} /></td>
                        <td className="px-3 py-2"><Stack primary={txKind[x.kind] ?? x.kind} secondary={x.reference ? <Mono>{x.reference}</Mono> : undefined} /></td>
                        <td className={`px-3 py-2 text-right tabular-nums ${x.amount_kes < 0 ? '' : 'text-success'}`}>{kes(x.amount_kes)}</td>
                      </tr>
                    ))}
                  </Rows>
                )}
              </TabsContent>
              <TabsContent value="topups" className="mt-3 grid gap-2">
                {t.top_ups.length === 0 ? <Nothing>No top-ups yet.</Nothing> : (
                  <Rows caption="Top-ups">
                    {t.top_ups.map((x) => (
                      <tr key={x.id} className="align-top">
                        <td className="px-3 py-2"><When at={x.created_at} /></td>
                        <td className="px-3 py-2">
                          <Stack primary={<Pill tone={topUpTone(x.status)}>{x.status.replaceAll('_', ' ')}</Pill>} secondary={x.failure_reason ? x.failure_reason.replaceAll('_', ' ') : x.channel === 'stk' ? 'M-Pesa prompt' : 'Paybill'}>
                            {x.receipt ? <Mono>{x.receipt}</Mono> : null}
                          </Stack>
                        </td>
                        <td className="px-3 py-2 text-right tabular-nums">{kes(x.amount_confirmed_kes ?? x.amount_requested_kes)}</td>
                        <td className="px-3 py-2 text-right"><Button asChild size="sm" variant="ghost"><Link to={`/topups?topup=${encodeURIComponent(x.id)}`}>Open top-up</Link></Button></td>
                      </tr>
                    ))}
                  </Rows>
                )}
              </TabsContent>
              <TabsContent value="plays" className="mt-3 grid gap-2">
                <p className="text-sm text-muted-foreground">{t.plays.count.toLocaleString('en-KE')} paid plays in all; the last {t.plays.last.length} here.</p>
                {t.plays.last.length === 0 ? <Nothing>No plays yet.</Nothing> : (
                  <Rows caption="Plays">
                    {t.plays.last.map((x) => (
                      <tr key={x.ticket_id} className="align-top">
                        <td className="px-3 py-2"><When at={x.created_at} /></td>
                        <td className="px-3 py-2"><Stack primary={`Play ${x.seq} of ${x.play_date}`} secondary={x.slot === null ? 'no spin recorded' : `Slot ${x.slot}${x.lucky ? ' · Lucky' : ''}`}><Mono className="text-muted-foreground">{x.ticket_id}</Mono></Stack></td>
                        <td className="px-3 py-2"><Pill tone={x.status === 'paid' ? 'success' : 'warning'}>{x.status}</Pill></td>
                        <td className="px-3 py-2 text-right tabular-nums">{kes(x.price_kes)}</td>
                      </tr>
                    ))}
                  </Rows>
                )}
              </TabsContent>
              <TabsContent value="payouts" className="mt-3 grid gap-2">
                {t.payouts.length === 0 ? <Nothing>No payouts yet.</Nothing> : (
                  <Rows caption="Payouts">
                    {t.payouts.map((x) => (
                      <tr key={x.id} className="align-top">
                        <td className="px-3 py-2"><When at={x.created_at} /></td>
                        <td className="px-3 py-2"><Stack primary={<span className="capitalize">{x.kind}</span>} secondary={x.receipt ? <Mono>{x.receipt}</Mono> : <Mono className="text-muted-foreground">{x.id}</Mono>} /></td>
                        <td className="px-3 py-2"><Pill tone={payoutTone(x.state)}>{x.state.replaceAll('_', ' ')}</Pill></td>
                        <td className="px-3 py-2 text-right tabular-nums"><Stack primary={kes(x.net_kes)} secondary={x.tax_kes > 0 ? `${kes(x.gross_kes)} less ${kes(x.tax_kes)} tax` : undefined} /></td>
                      </tr>
                    ))}
                  </Rows>
                )}
                <Button asChild size="sm" variant="ghost" className="justify-self-start"><Link to="/payouts">Open payouts</Link></Button>
              </TabsContent>
              <TabsContent value="identity" className="mt-3 grid gap-4">
                <section className="grid gap-2">
                  <h3 className="text-sm font-semibold">Identity submissions</h3>
                  {t.identity.kyc.length === 0 ? <Nothing>No submission yet.</Nothing> : (
                    <Rows caption="Identity submissions">
                      {t.identity.kyc.map((x) => (
                        <tr key={x.id} className="align-top">
                          <td className="px-3 py-2"><When at={x.created_at} /></td>
                          <td className="px-3 py-2"><Stack primary={<Pill tone={x.status === 'verified' ? 'success' : x.status === 'rejected' || x.status === 'mismatch' ? 'error' : 'pending'}>{x.status.replaceAll('_', ' ')}</Pill>} secondary={x.reviewed_by ? <>by <Mono>{x.reviewed_by}</Mono>{x.reason ? ` · ${x.reason}` : ''}</> : x.provider} /></td>
                        </tr>
                      ))}
                    </Rows>
                  )}
                </section>
                <section className="grid gap-2">
                  <h3 className="text-sm font-semibold">Sign-ins</h3>
                  {t.identity.sessions.length === 0 ? <Nothing>Never signed in.</Nothing> : (
                    <Rows caption="Sign-ins">
                      {t.identity.sessions.map((x) => (
                        <tr key={x.id} className="align-top">
                          <td className="px-3 py-2"><When at={x.created_at} /></td>
                          <td className="px-3 py-2"><Stack primary={<span className="capitalize">{x.platform || 'unknown platform'}</span>} secondary={x.revoked_at ? `ended ${eat(x.revoked_at)} (${(x.revoked_reason ?? '').replaceAll('_', ' ')})` : 'still open'} /></td>
                          <td className="px-3 py-2">{x.device_verdict === 'none' ? null : <Pill tone={x.device_verdict === 'passed' ? 'success' : 'error'}>device {x.device_verdict}</Pill>}</td>
                        </tr>
                      ))}
                    </Rows>
                  )}
                </section>
                <section className="grid gap-2">
                  <h3 className="text-sm font-semibold">Account status</h3>
                  {t.identity.status_history.length === 0 ? <Nothing>No status change recorded.</Nothing> : (
                    <Rows caption="Account status">
                      {t.identity.status_history.map((x) => (
                        <tr key={x.id} className="align-top">
                          <td className="px-3 py-2"><When at={x.at} /></td>
                          <td className="px-3 py-2"><Stack primary={<Pill tone={x.to === 'active' ? 'success' : 'error'}>{x.from ? `${x.from} → ${x.to}` : x.to}</Pill>} secondary={<>{x.actor_type === 'staff' && x.actor ? <>by <Mono>{x.actor}</Mono></> : x.actor_type ?? 'not named'}{x.reason ? ` · ${x.reason}` : ''}</>} /></td>
                        </tr>
                      ))}
                    </Rows>
                  )}
                </section>
                <section className="grid gap-2">
                  <h3 className="text-sm font-semibold">Where from</h3>
                  {t.identity.locations.length === 0 ? <Nothing>No location recorded.</Nothing> : (
                    <Rows caption="Where from">
                      {t.identity.locations.map((x) => (
                        <tr key={x.id} className="align-top">
                          <td className="px-3 py-2"><When at={x.at} /></td>
                          <td className="px-3 py-2"><Stack primary={<Mono>{x.country ?? 'unknown'}</Mono>} secondary={x.action === 'topup' ? 'top-up' : x.action} /></td>
                          <td className="px-3 py-2">{x.refused ? <Pill tone="error">refused</Pill> : null}</td>
                        </tr>
                      ))}
                    </Rows>
                  )}
                  <p className="text-xs text-muted-foreground">The country the network edge reports for the address. A signal, not proof: a VPN changes it.</p>
                </section>
                {t.identity.closure_requested_at ? <p className="text-sm">Asked to close the account on {eat(t.identity.closure_requested_at)}. <Link to="/closures">Open closures</Link></p> : null}
              </TabsContent>
              <TabsContent value="limits" className="mt-3 grid gap-3">
                {t.limits.restriction ? (
                  <p className="rounded-xl border border-warning/40 bg-warning/10 px-3 py-2 text-sm">
                    {t.limits.restriction.kind === 'timeout' ? 'In a time-out' : 'Self-excluded'}{t.limits.restriction.until ? ` until ${eat(t.limits.restriction.until)}` : ' permanently'}. Top-ups and plays are refused.
                  </p>
                ) : <p className="text-sm text-muted-foreground">No time-out or self-exclusion in force.</p>}
                {t.limits.deposit.length === 0 ? <p className="text-sm text-muted-foreground">No deposit limits set.</p> : (
                  <Rows caption="Deposit limits">
                    {t.limits.deposit.map((x) => (
                      <tr key={`${x.period}-${x.pending ? 'pending' : 'current'}`}>
                        <td className="px-3 py-2 capitalize">{x.period}</td>
                        <td className="px-3 py-2 tabular-nums">{kes(x.amount_kes)}</td>
                        <td className="px-3 py-2 text-muted-foreground">{x.pending ? `a raise, from ${eat(x.effective_from)}` : `since ${eat(x.effective_from)}`}</td>
                      </tr>
                    ))}
                  </Rows>
                )}
              </TabsContent>
              <TabsContent value="activity" className="mt-3 grid gap-2">
                <p className="text-sm text-muted-foreground">The last {t.events.length} audit records about this player. <Link to={`/activity?entity_id=${encodeURIComponent(p.user_id)}`}>Everything, in the activity log</Link>.</p>
                {t.events.length === 0 ? <Nothing>Nothing recorded yet.</Nothing> : (
                  <Rows caption="Audit records about the player">
                    {t.events.map((e: AuditEvent) => (
                      <tr key={e.id} className="align-top">
                        <td className="px-3 py-2"><When at={e.at} /></td>
                        <td className="px-3 py-2">
                          <Stack primary={<Mono>{e.action}</Mono>} secondary={<span><span className="capitalize">{e.actor_type}</span>{e.actor_id && e.actor_id !== p.user_id ? <> <Mono>{e.actor_id}</Mono></> : null}</span>}>
                            {Object.keys(e.details).length > 0 ? <span className="text-xs text-muted-foreground [overflow-wrap:anywhere]">{Object.entries(e.details).map(([k, v]) => `${k.replaceAll('_', ' ')}: ${typeof v === 'string' ? v : JSON.stringify(v)}`).join(' · ')}</span> : null}
                          </Stack>
                        </td>
                      </tr>
                    ))}
                  </Rows>
                )}
              </TabsContent>
            </Tabs>
          </>
          )
        }}
      </Loaded>
    </div>
  )
}
