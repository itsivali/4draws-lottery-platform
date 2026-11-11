import { Dices, Smartphone, Wallet, Banknote, type LucideIcon } from 'lucide-react'
import { useId } from 'react'
import { useSession } from '../context'
import { eat } from '../format'
import { useLoad } from '../hooks'
import { can } from '../roles'
import type { AreaSwitch } from '../types'
import { useConfirmedAction } from './useConfirmedAction'
import { Loaded, Mono, Pill } from './ui'
import { Switch } from '@/components/ui/switch'

const areaWords: Record<AreaSwitch['area'], { name: string; stops: string; icon: LucideIcon }> = {
  'top-ups': { name: 'Top-ups', icon: Smartphone, stops: 'New M-Pesa prompts and paybill payments are refused; prompts already sent still settle.' },
  plays: { name: 'Plays', icon: Dices, stops: 'Nobody can spin. Wallets, draws and withdrawals carry on.' },
  withdrawals: { name: 'Withdrawals', icon: Wallet, stops: 'Players cannot ask for their money out. Payouts already made still go.' },
  payouts: { name: 'Payouts', icon: Banknote, stops: 'Nothing is sent to M-Pesa: withdrawals and prizes wait, pending, until resumed.' },
}

/**
 * The money switches (ADR 0036): pause one area without stopping the rest. Anyone on staff can pause; an admin
 * resumes. Every change needs a reason and is audited; a paused area raises an alert until it is resumed.
 */
export function Switches() {
  const { api, me } = useSession()
  const load = useLoad((signal) => api.get<{ switches: AreaSwitch[] }>('/v1/staff/operating/switches', {}, { signal }), [])
  const { ask, busy, view } = useConfirmedAction(load.reload)
  const admin = can(me.roles, 'admin')
  const titleId = useId()
  return (
    <section aria-labelledby={titleId} className="overflow-hidden rounded-3xl border bg-card shadow-card animate-rise-in">
      <div className="px-5 pb-4 pt-5">
        <h2 id={titleId} className="m-0 font-display text-lg font-bold tracking-tight">Money switches</h2>
        <p className="m-0 mt-1 max-w-[75ch] text-[14px] leading-relaxed text-muted-foreground">
          Pause one area while the rest keep running: for an M-Pesa outage, a fault, or while something is checked. Anyone on
          staff can pause; only an admin can resume. Each change needs a reason and is recorded in the activity log.
        </p>
        {view}
      </div>
      <Loaded load={load} what="the money switches">
        {({ switches }) => (
          <ul className="m-0 grid list-none p-0">
            {switches.map((sw) => {
              const w = areaWords[sw.area]
              const Icon = w.icon
              const locked = sw.paused && !admin
              const labelId = `switch-${sw.area}`
              return (
                <li key={sw.area} className="flex items-start gap-4 border-t px-5 py-4">
                  <span aria-hidden="true" className={`mt-0.5 flex size-11 shrink-0 items-center justify-center rounded-2xl ${sw.paused ? 'bg-destructive/10 text-destructive' : 'bg-primary-container text-primary-container-foreground'}`}><Icon className="size-5" /></span>
                  <div className="min-w-0 flex-1">
                    <p className="m-0 flex flex-wrap items-center gap-2">
                      <span id={labelId} className="text-[15px] font-semibold">{w.name}</span>
                      {sw.paused ? <Pill tone="error">Paused</Pill> : <Pill tone="success">Running</Pill>}
                    </p>
                    <p className="m-0 mt-0.5 text-[14px] leading-relaxed text-muted-foreground">{sw.paused ? 'Paused: ' : 'Pausing: '}{w.stops}</p>
                    {sw.at ? (
                      <p className="m-0 mt-1 text-[13px] text-muted-foreground">
                        {sw.paused ? 'Paused' : 'Resumed'} by <Mono>{sw.changed_by}</Mono>, {eat(sw.at)} — “{sw.reason}”
                      </p>
                    ) : null}
                    {locked ? <p className="m-0 mt-1 text-[13px] font-medium text-foreground">An admin resumes it.</p> : null}
                  </div>
                  <Switch
                    className="mt-2.5"
                    checked={!sw.paused}
                    disabled={busy || locked}
                    aria-labelledby={labelId}
                    onCheckedChange={(on) => {
                      ask(on ? {
                        title: `Resume ${w.name.toLowerCase()}?`, confirmLabel: 'Resume', success: `${w.name} resumed.`,
                        body: <p>{w.name} start again at once, as far as the operating mode allows.</p>,
                        fields: [{ name: 'reason', label: 'Why', required: true, help: 'What was checked or fixed.' }],
                        act: (v) => api.post(`/v1/staff/operating/switches/${sw.area}/resume`, { reason: v.reason }),
                      } : {
                        title: `Pause ${w.name.toLowerCase()}?`, confirmLabel: 'Pause', success: `${w.name} paused.`, danger: true,
                        body: <p>{w.stops} Players are told it is paused and their balance is safe. An alert stays open until an admin resumes it.</p>,
                        fields: [{ name: 'reason', label: 'Why', required: true, help: 'Players never see this; staff do.' }],
                        act: (v) => api.post(`/v1/staff/operating/switches/${sw.area}/pause`, { reason: v.reason }),
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
