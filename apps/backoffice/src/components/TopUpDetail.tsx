import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useSession } from '../context'
import { eat, kes, msisdn } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { CallbackView, Recheck, TopUpDetail } from '../types'
import { Loaded, Mono, Pill, type Tone } from './ui'
import { useConfirmedAction } from './useConfirmedAction'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'

export const statusLabel: Record<string, [Tone, string]> = {
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
export const resultCodes: Record<number, string> = {
  0: 'paid', 1: 'insufficient balance', 1032: 'cancelled by the customer', 1037: 'phone did not answer (timeout)', 2001: 'wrong PIN', 4999: 'still processing',
}

export const claimLabel: Record<string, [Tone, string]> = {
  claimed: ['pending', 'Receipt from player: not asked of M-Pesa yet'],
  checking: ['pending', 'Receipt from player: M-Pesa asked'],
  confirmed: ['success', 'Receipt from player: confirmed by M-Pesa'],
  unverified: ['warning', 'Receipt from player: not confirmed'],
  rejected: ['neutral', 'Receipt from player: rejected'],
}

const outcomeText: Record<Recheck['outcome'], string> = {
  credited: 'A paid callback had arrived: the top-up is now credited.',
  paid_without_receipt: 'M-Pesa says the prompt was paid, but gave no receipt. Credit it from the statement or the player’s receipt.',
  failed: 'M-Pesa says it failed; recorded.',
  cancelled: 'M-Pesa says the customer cancelled it; recorded.',
  pending: 'M-Pesa has no final answer yet.',
  no_prompt: 'The prompt was never accepted by M-Pesa.',
  asking: 'M-Pesa is being asked about the player’s receipt; look again in a minute.',
  verifying: 'A paid callback had arrived. Its receipt is checked with M-Pesa before anything is credited; look again in a minute.',
}

/** Everything the platform knows about one top-up, and what an operator can do about it. */
export function TopUpDetailSheet({ id, onClose, onChanged }: { id: string | null; onClose: () => void; onChanged: () => void }) {
  const { api, me } = useSession()
  const load = useLoad((signal) => (id ? api.get<TopUpDetail>(`/v1/staff/topups/${encodeURIComponent(id)}`, undefined, { signal }) : Promise.resolve(null)), [id])
  const [recheck, setRecheck] = useState<Recheck | null>(null)
  const { ask, busy, view } = useConfirmedAction(() => { load.reload(); onChanged() })
  const operator = can(me.roles, 'staff')
  return (
    <Sheet open={id !== null} onOpenChange={(open) => { if (!open) onClose() }}>
      <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>Top-up</SheetTitle>
          <SheetDescription>{id ? <Mono>{id}</Mono> : null}</SheetDescription>
        </SheetHeader>
        <div className="grid gap-5 px-4 pb-8">
          {view}
          <Loaded load={load} what="the top-up">
            {(d) => d === null ? null : (
              <>
                <Summary d={d} />
                {recheck ? (
                  <p role="status" className="rounded-lg border bg-muted/30 px-3 py-2 text-sm">
                    <strong>M-Pesa says:</strong> {outcomeText[recheck.outcome]}{recheck.result_desc ? <> <span className="text-muted-foreground">({recheck.result_desc})</span></> : null}
                  </p>
                ) : null}
                {operator ? (
                  <div className="flex flex-wrap gap-2">
                    {d.status !== 'credited' && d.status !== 'reversed' ? (
                      <Button type="button" variant="outline" disabled={busy} onClick={() => {
                        ask({
                          title: 'Ask M-Pesa again', confirmLabel: 'Ask M-Pesa', success: 'Asked.',
                          body: <p>A paid callback that arrived late is credited; otherwise M-Pesa’s status query is asked and its answer recorded.</p>,
                          act: async () => { setRecheck(await api.post<Recheck>(`/v1/staff/topups/${encodeURIComponent(d.id)}/recheck`)) },
                        })
                      }}>Ask M-Pesa again</Button>
                    ) : null}
                    {['unconfirmed', 'failed', 'cancelled', 'expired'].includes(d.status) ? (
                      <Button type="button" disabled={busy} onClick={() => {
                        ask({
                          title: 'Credit from a receipt', confirmLabel: 'Credit', success: 'Credited.',
                          body: <p>Credits the player once, exactly like a confirmed callback, and proves their number. Use the receipt and amount on the M-Pesa statement or in the player’s message.</p>,
                          fields: [
                            { name: 'receipt', label: 'M-Pesa receipt', required: true, help: '10 letters and digits' },
                            { name: 'amount', label: 'Amount paid (KES)', type: 'number', required: true },
                            { name: 'note', label: 'Note', help: 'Where you found it (statement date, the player’s message).' },
                          ],
                          initial: { receipt: d.claim?.receipt ?? '', amount: String(d.amount_requested_kes), note: '' },
                          act: (v) => api.post(`/v1/staff/topups/${encodeURIComponent(d.id)}/resolve`, { receipt: v.receipt, amount_kes: Number(v.amount), note: v.note }),
                        })
                      }}>Credit from receipt</Button>
                    ) : null}
                    {d.status === 'unconfirmed' || (d.claim && (d.claim.status === 'claimed' || d.claim.status === 'checking' || d.claim.status === 'unverified')) ? (
                      <Button type="button" variant="outline" disabled={busy} onClick={() => {
                        ask({
                          title: 'Not on the statement', confirmLabel: 'Close it', success: 'Closed.', danger: true,
                          body: <p>The top-up is closed as not paid; a receipt the player gave is rejected with your note. It can still be credited later if it turns up.</p>,
                          fields: [{ name: 'note', label: 'Note', required: true, help: 'Which statement you checked.' }],
                          act: (v) => api.post(`/v1/staff/topups/${encodeURIComponent(d.id)}/dismiss`, { note: v.note }),
                        })
                      }}>Not on the statement</Button>
                    ) : null}
                    {d.status === 'credited' && d.receipt && !d.sandbox ? (
                      <Button type="button" variant="outline" disabled={busy} onClick={() => {
                        ask({
                          title: 'Record a reversal', confirmLabel: 'Record reversal', success: 'Reversal recorded.', danger: true,
                          body: <p>M-Pesa took the payment back. The player gives back what they received: from the wallet as far as it goes, the rest as debt.</p>,
                          fields: [{ name: 'reason', label: 'Reason', required: true }],
                          act: (v) => api.post('/v1/staff/payments/reversals', { receipt: d.receipt, reason: v.reason }),
                        })
                      }}>Record a reversal</Button>
                    ) : null}
                  </div>
                ) : null}
                <Section title="Receipt from the player">
                  {d.claim ? (
                    <dl className="grid gap-1 text-sm">
                      <Row k="Receipt"><Mono>{d.claim.receipt}</Mono></Row>
                      <Row k="Status"><Pill tone={claimLabel[d.claim.status]?.[0] ?? 'neutral'}>{claimLabel[d.claim.status]?.[1] ?? d.claim.status}</Pill></Row>
                      <Row k="Given">{eat(d.claim.claimed_at)}</Row>
                      {d.claim.note ? <Row k="Note">{d.claim.note}</Row> : null}
                      {d.claim.originator ? <Row k="Transaction Status request"><Mono>{d.claim.originator}</Mono></Row> : null}
                      {d.claim.result ? <Raw label="M-Pesa’s answer" value={d.claim.result} /> : null}
                    </dl>
                  ) : <p className="text-sm text-muted-foreground">The player has not given a receipt.</p>}
                </Section>
                <Section title="M-Pesa">
                  <dl className="grid gap-1 text-sm">
                    <Row k="Checkout id">{d.checkout_request_id ? <Mono>{d.checkout_request_id}</Mono> : <span className="text-muted-foreground">none: the prompt was never accepted</span>}</Row>
                    {d.merchant_request_id ? <Row k="Merchant request"><Mono>{d.merchant_request_id}</Mono></Row> : null}
                    <Row k="Callback">{d.callback ? <Callback c={d.callback} /> : <span className="text-muted-foreground">No callback from Safaricom.</span>}</Row>
                    {d.orphan_callbacks.length > 0 ? (
                      <Row k="Orphan callbacks">
                        <ul className="grid gap-1">{d.orphan_callbacks.map((c, i) => <li key={i}><Callback c={c} /></li>)}</ul>
                      </Row>
                    ) : null}
                    {d.raw_callback ? <Raw label="Raw callback" value={d.raw_callback} /> : null}
                  </dl>
                </Section>
                <Section title="What happened">
                  {d.events.length === 0 ? <p className="text-sm text-muted-foreground">No audit records.</p> : (
                    <ol className="grid gap-2 text-sm">
                      {[...d.events].reverse().map((e) => (
                        <li key={e.id} className="grid grid-cols-[9rem_1fr] gap-x-3">
                          <span className="tabular-nums text-muted-foreground">{eat(e.at)}</span>
                          <span>
                            <span className="font-medium">{e.action.replace(/^payment\.topup\./, '').replaceAll('_', ' ').replaceAll('.', ' · ')}</span>
                            {e.actor_type === 'staff' ? <span className="text-muted-foreground"> by {e.actor_id}</span> : e.actor_type === 'player' ? <span className="text-muted-foreground"> by the player</span> : null}
                            {detailsLine(e.details) ? <span className="block text-muted-foreground">{detailsLine(e.details)}</span> : null}
                          </span>
                        </li>
                      ))}
                    </ol>
                  )}
                </Section>
                <Section title="Background jobs">
                  {d.jobs.length === 0 ? <p className="text-sm text-muted-foreground">None recorded.</p> : (
                    <ul className="grid gap-1 text-sm">
                      {d.jobs.map((j, i) => (
                        <li key={i} className="flex flex-wrap items-center gap-x-2">
                          <Mono>{j.kind.replace('payments.', '')}</Mono>
                          <Pill tone={j.state === 'completed' ? 'success' : j.state === 'discarded' ? 'error' : 'pending'}>{j.state}</Pill>
                          <span className="text-muted-foreground">attempt {j.attempt} · {eat(j.finalized_at ?? j.scheduled_at)}</span>
                          {j.last_error ? <span className="basis-full text-muted-foreground">{j.last_error}</span> : null}
                        </li>
                      ))}
                    </ul>
                  )}
                </Section>
                <Section title="Player">
                  <dl className="grid gap-1 text-sm">
                    <Row k="Account"><Link to={`/players?q=${encodeURIComponent(d.user_id)}`} className="underline-offset-2 hover:underline">{d.full_name ?? 'No name yet'}</Link> · <span className="tabular-nums">{msisdn(d.msisdn)}</span>{d.player.email ? <> · {d.player.email}</> : null}</Row>
                    <Row k="Standing">
                      <span className="flex flex-wrap gap-1">
                        <Pill tone={d.player.status === 'active' ? 'success' : 'error'}>{d.player.status}</Pill>
                        <Pill tone={d.player.kyc_status === 'verified' ? 'success' : 'neutral'}>ID {d.player.kyc_status}</Pill>
                        <Pill tone={d.player.phone_verified ? 'success' : 'warning'}>{d.player.phone_verified ? 'Phone proved' : 'Phone not proved'}</Pill>
                      </span>
                    </Row>
                    <Row k="Wallet">{kes(d.player.balance_kes)}</Row>
                    <Row k="Top-ups">{d.player.top_ups} in all · {kes(d.player.credited_kes)} credited{d.player.unconfirmed_count > 0 ? ` · ${d.player.unconfirmed_count} unconfirmed` : ''}</Row>
                  </dl>
                </Section>
              </>
            )}
          </Loaded>
        </div>
      </SheetContent>
    </Sheet>
  )
}

function Summary({ d }: { d: TopUpDetail }) {
  const [tone, text] = statusLabel[d.status] ?? ['neutral', d.status]
  const code = d.sandbox ? 'credited without M-Pesa’s confirmation (sandbox)' : d.result_code !== null ? (resultCodes[d.result_code] ?? `M-Pesa code ${d.result_code}`) : null
  return (
    <div className="grid gap-2 rounded-xl border bg-card p-4 shadow-card">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <span className="font-display text-2xl font-semibold tabular-nums">{kes(d.amount_confirmed_kes ?? d.amount_requested_kes)}</span>
        <span className="flex flex-wrap gap-1">
          <Pill tone={tone}>{text}</Pill>
          {d.sandbox ? <Pill tone="pending">Sandbox</Pill> : null}
          {d.claim_status ? <Pill tone={claimLabel[d.claim_status]?.[0] ?? 'neutral'}>Receipt from player</Pill> : null}
        </span>
      </div>
      <dl className="grid gap-1 text-sm">
        <Row k="Started">{eat(d.created_at)} · {d.channel === 'stk' ? 'M-Pesa prompt' : 'Paybill'}</Row>
        <Row k="Last change">{eat(d.updated_at)}</Row>
        {d.amount_confirmed_kes !== null && d.amount_confirmed_kes !== d.amount_requested_kes ? <Row k="Asked">{kes(d.amount_requested_kes)}</Row> : null}
        <Row k="Why">{[code, d.failure_reason?.replaceAll('_', ' ')].filter(Boolean).join(' · ') || <span className="text-muted-foreground">—</span>}</Row>
        <Row k="Receipt">{d.receipt ? <Mono>{d.receipt}</Mono> : <span className="text-muted-foreground">none</span>}</Row>
        {d.reversed_at ? <Row k="Reversed">{eat(d.reversed_at)}{d.reversal_reason ? ` · ${d.reversal_reason}` : ''}</Row> : null}
      </dl>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="grid gap-2">
      <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{title}</h3>
      {children}
    </section>
  )
}

function Row({ k, children }: { k: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[9rem_1fr] gap-x-3">
      <dt className="text-muted-foreground">{k}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </div>
  )
}

function Callback({ c }: { c: CallbackView }) {
  if (c.unreadable) return <span>unreadable{c.received_at ? ` · ${eat(c.received_at)}` : ''}</span>
  return (
    <span>
      {c.result_code === 0 ? 'paid' : (resultCodes[c.result_code] ?? `code ${c.result_code}`)}
      {c.result_desc ? <span className="text-muted-foreground"> · {c.result_desc}</span> : null}
      {c.amount_kes ? <> · {kes(c.amount_kes)}</> : null}
      {c.receipt ? <> · <Mono>{c.receipt}</Mono></> : null}
      {c.payer ? <> · from {msisdn(c.payer)}</> : null}
      {c.received_at ? <span className="text-muted-foreground"> · {eat(c.received_at)}</span> : null}
    </span>
  )
}

function Raw({ label, value }: { label: string; value: unknown }) {
  return (
    <details className="col-span-2 rounded-lg border bg-muted/30 px-3 py-2">
      <summary className="cursor-pointer text-sm font-medium">{label}</summary>
      <pre className="mt-2 overflow-x-auto whitespace-pre-wrap break-all font-mono text-xs">{JSON.stringify(value, null, 2)}</pre>
    </details>
  )
}

/** The parts of an audit record's details an operator reads at a glance. */
function detailsLine(details: unknown): string {
  if (typeof details !== 'object' || details === null) return ''
  const d = details as Record<string, unknown>
  const parts: string[] = []
  for (const k of ['source', 'receipt', 'amount_kes', 'reason', 'note', 'outcome', 'result_code', 'was', 'phone_proof', 'debt_recovered_kes']) {
    const v = d[k]
    if (v === undefined || v === null || v === '' || v === 0) continue
    const text = typeof v === 'string' ? v : typeof v === 'number' || typeof v === 'boolean' ? String(v) : JSON.stringify(v)
    parts.push(`${k.replaceAll('_', ' ')}: ${text}`)
  }
  return parts.join(' · ')
}
