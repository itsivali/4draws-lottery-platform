import { BadgeCheck, Pencil } from 'lucide-react'
import { useSession } from '../context'
import { useLoad } from '../hooks'
import { ruleLabels } from '../aml'
import { can } from '../roles'
import type { AmlRule } from '../types'
import { useConfirmedAction } from './useConfirmedAction'
import { Loaded, Pill, type ConfirmField } from './ui'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'

/** Each threshold in words: its label, what it counts (for the dialog's error), and what it means. */
const paramWords: Record<string, { label: string; unit: string; help: string }> = {
  window_hours: { label: 'Window (hours)', unit: 'hours', help: 'How far back the rule looks.' },
  max_kes: { label: 'Most money in (KES)', unit: 'shillings', help: 'More than this in the window opens a case.' },
  max_count: { label: 'Most top-ups', unit: 'top-ups', help: 'More top-ups than this in the window opens a case.' },
  min_withdrawal_kes: { label: 'Smallest withdrawal (KES)', unit: 'shillings', help: 'Withdrawals below this are ignored.' },
  max_play_percent: { label: 'Most play (%)', unit: 'percent', help: 'Playing less than this share of what came in opens a case (1–100).' },
  min_failed: { label: 'Failed top-ups', unit: 'failed top-ups', help: 'This many failures in the window opens a case.' },
  min_accounts: { label: 'Accounts', unit: 'accounts', help: 'This many accounts sharing it opens a case (2 or more).' },
  quiet_days: { label: 'Quiet period (days)', unit: 'days', help: 'After a case closes, the rule leaves the player alone this long.' },
}

/** A rule parameter's name, in words: "max_kes" → "Max KES". */
function paramLabel(k: string): string {
  if (paramWords[k]) return paramWords[k].label.replace(/ \(.*\)$/, '')
  const w = k.split('_').map((p) => (p === 'kes' ? 'KES' : p))
  const first = w[0] ?? ''
  return [first.charAt(0).toUpperCase() + first.slice(1), ...w.slice(1)].join(' ')
}

/** A threshold's value with its unit: "24 h", "KES 100,000", "20%", "30 days". */
function shown(k: string, v: number): string {
  const n = v.toLocaleString('en-KE')
  if (k.endsWith('_kes')) return `KES ${n}`
  if (k.endsWith('_hours')) return `${n} h`
  if (k.endsWith('_days')) return `${n} days`
  if (k.endsWith('_percent')) return `${n}%`
  return n
}

const note: ConfirmField = { name: 'note', label: 'Note', required: true, help: 'Why: the compliance owner’s decision, a memo, a finding. Kept in the audit log.' }

/**
 * The monitoring rules (ADR 0033, 0036): what the hourly scan looks for, its thresholds, and whether the compliance
 * owner approved them. An admin switches a rule on or off, edits its thresholds, or records the owner's approval;
 * each change needs a note and is audited. Everyone else sees them.
 */
export function Rules() {
  const { api, me } = useSession()
  const load = useLoad((signal) => api.get<{ rules: AmlRule[] }>('/v1/staff/aml/rules', undefined, { signal }))
  const { ask, busy, view } = useConfirmedAction(load.reload)
  const admin = can(me.roles, 'admin')
  const put = (r: AmlRule, enabled: boolean, params: Record<string, number>, approval: string, why: string) =>
    api.put(`/v1/staff/aml/rules/${r.rule}`, { enabled, params, approval_reference: approval, note: why })

  const edit = (r: AmlRule) => {
    const keys = Object.keys(r.params)
    ask({
      title: `Edit “${ruleLabels[r.rule]}”`, confirmLabel: 'Save the rule', success: 'Rule saved.',
      body: <p>The next hourly scan uses these. Give the compliance owner’s approval reference only if they approved these exact values; left empty, the rule is marked pending approval.</p>,
      fields: [
        { name: 'enabled', label: 'The rule runs', type: 'checkbox' },
        ...keys.map((k): ConfirmField => ({ name: k, label: paramWords[k]?.label ?? paramLabel(k), type: 'number', required: true, unit: paramWords[k]?.unit ?? 'units', help: paramWords[k]?.help ?? '' })),
        { name: 'approval', label: 'Compliance owner’s approval reference', help: 'For example the memo or minute number.' },
        note,
      ],
      initial: { enabled: r.enabled ? 'true' : '', approval: '', note: '', ...Object.fromEntries(keys.map((k) => [k, String(r.params[k] ?? '')])) },
      act: (v) => put(r, v.enabled === 'true', Object.fromEntries(keys.map((k) => [k, Number(v[k])])), v.approval ?? '', v.note ?? ''),
    })
  }

  return (
    <section aria-labelledby="rules-title" className="mt-8 overflow-hidden rounded-3xl border bg-card shadow-card animate-rise-in">
      <div className="px-5 pb-4 pt-5">
        <h3 id="rules-title" className="m-0 font-display text-lg font-bold tracking-tight">Rules</h3>
        <p className="m-0 mt-1 max-w-[75ch] text-[14px] leading-relaxed text-muted-foreground">
          What the hourly scan looks for, and the thresholds it uses. They are the compliance owner’s decision and stay marked
          pending until their approval is recorded. {admin ? 'Click a rule to change it; every change needs a note.' : 'Only an admin changes them.'}
        </p>
        {view}
      </div>
      <Loaded load={load} what="rules">
        {({ rules: list }) => (
          <ul className="m-0 grid list-none p-0">
            {list.map((r) => {
              const labelId = `rule-${r.rule}`
              return (
                <li key={r.rule} className="group flex items-start gap-4 border-t px-5 py-4 transition-colors hover:bg-muted/30">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      {admin ? (
                        <button type="button" disabled={busy} onClick={() => { edit(r) }}
                          className="inline-flex cursor-pointer items-center gap-1.5 rounded-md text-left text-[15px] font-semibold underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring">
                          <span id={labelId}>{ruleLabels[r.rule]}</span> <Pencil aria-hidden="true" className="size-3.5 text-muted-foreground opacity-60 group-hover:opacity-100" />
                          <span className="sr-only">: edit</span>
                        </button>
                      ) : <span id={labelId} className="text-[15px] font-semibold">{ruleLabels[r.rule]}</span>}
                      <Pill tone={r.enabled ? 'success' : 'neutral'}>{r.enabled ? 'on' : 'off'}</Pill>
                      {r.pending_approval ? <Pill tone="warning">pending compliance-owner approval</Pill> : <Pill tone="success">approved: {r.approval_reference}</Pill>}
                    </div>
                    <dl className="m-0 mt-2 flex flex-wrap gap-2">
                      {Object.entries(r.params).map(([k, v]) => (
                        <div key={k} className="inline-flex items-baseline gap-1.5 rounded-lg bg-muted/70 px-2.5 py-1 text-[13.5px]">
                          <dt className="text-muted-foreground">{paramLabel(k)}</dt>
                          <dd className="m-0 font-semibold tabular-nums">{shown(k, v)}</dd>
                        </div>
                      ))}
                    </dl>
                    {admin && r.pending_approval ? (
                      <Button type="button" variant="outline" size="sm" className="mt-3" disabled={busy} onClick={() => {
                        ask({
                          title: `Record the approval of “${ruleLabels[r.rule]}”`, confirmLabel: 'Record approval', success: 'Approval recorded.',
                          body: <p>Records that the compliance owner approved this rule as it stands now. To change its thresholds too, edit the rule instead.</p>,
                          fields: [{ name: 'approval', label: 'Approval reference', required: true, help: 'The memo or minute number.' }, note],
                          act: (v) => put(r, r.enabled, r.params, v.approval ?? '', v.note ?? ''),
                        })
                      }}><BadgeCheck aria-hidden="true" /> Record approval</Button>
                    ) : null}
                  </div>
                  <Switch
                    className="mt-0.5"
                    checked={r.enabled}
                    disabled={!admin || busy}
                    aria-labelledby={labelId}
                    title={admin ? undefined : 'Only an admin changes the rules'}
                    onCheckedChange={(on) => {
                      ask({
                        title: `${on ? 'Turn on' : 'Turn off'} “${ruleLabels[r.rule]}”?`, confirmLabel: on ? 'Turn on' : 'Turn off', success: on ? 'Rule on.' : 'Rule off.', danger: !on,
                        body: <p>{on ? 'The next hourly scan runs it.' : 'The scan stops running it. Cases it already opened stay open.'}</p>,
                        fields: [note],
                        act: (v) => put(r, on, r.params, r.approval_reference ?? '', v.note ?? ''),
                      })
                    }}
                  />
                </li>
              )
            })}
          </ul>
        )}
      </Loaded>
    </section>
  )
}
