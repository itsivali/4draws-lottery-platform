import { Download } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { ProblemError } from '../api'
import { BarChart } from '../components/BarChart'
import { LineChart } from '../components/LineChart'
import { ReconcileCard } from '../components/Reconcile'
import { Empty, Loaded, PageHeader, Pill, SectionTitle } from '../components/ui'
import { useSession } from '../context'
import { eat, kes } from '../format'
import { useLoad, usePages } from '../hooks'
import type { FinanceGroup, FinancePeriod, FinanceSummary, ReconciliationRun } from '../types'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCaption, TableCell, TableFooter, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ExportReceipt, type SavedFile } from '../components/polish'

// Dates are Nairobi dates as YYYY-MM-DD strings; the arithmetic below runs on them in UTC so no browser time zone
// leaks in.

function nairobiToday(): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Africa/Nairobi' }).format(new Date())
}

const at = (d: string) => new Date(`${d}T00:00:00Z`)
const iso = (t: Date) => t.toISOString().slice(0, 10)

function minusDays(d: string, n: number): string {
  const t = at(d)
  t.setUTCDate(t.getUTCDate() - n)
  return iso(t)
}

/** Whole days from a to b, inclusive (negative when b is before a). */
const daysBetween = (a: string, b: string) => Math.round((at(b).getTime() - at(a).getTime()) / 86_400_000) + 1

/** "2026-10-26" → "26 Oct". */
const shortDay = (d: string) => new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short', timeZone: 'UTC' }).format(at(d))
const monthName = (d: string) => new Intl.DateTimeFormat('en-GB', { month: 'short', year: 'numeric', timeZone: 'UTC' }).format(at(d))

/** How a period is named: a day by its date, a week by its two ends, a whole month by its name. */
function periodLabel(p: { start: string; end: string }, group: FinanceGroup): string {
  if (p.start === p.end) return shortDay(p.start)
  if (group === 'month' && p.start.endsWith('-01') && at(p.end).getUTCMonth() !== at(minusDays(p.end, -1)).getUTCMonth()) return monthName(p.start)
  return `${shortDay(p.start)} – ${shortDay(p.end)}`
}

const presets = [
  { id: '7', label: 'Last 7 days' },
  { id: '30', label: 'Last 30 days' },
  { id: '90', label: 'Last 90 days' },
  { id: 'this-month', label: 'This month' },
  { id: 'last-month', label: 'Last month' },
  { id: 'this-quarter', label: 'This quarter' },
  { id: 'ytd', label: 'Year to date' },
  { id: 'custom', label: 'Custom' },
] as const

type PresetId = (typeof presets)[number]['id']

export const MAX_RANGE_DAYS = 366
export const RANGE_PROBLEM = 'The range must run forward and cover at most 366 days.'

/** The dates a preset means today; null when a custom range is not a valid one yet. */
export function rangeOf(preset: PresetId, today: string, custom: { from: string; to: string }): { from: string; to: string } | null {
  const t = at(today)
  const y = t.getUTCFullYear()
  const m = t.getUTCMonth()
  switch (preset) {
    case '7': case '30': case '90':
      return { from: minusDays(today, Number(preset) - 1), to: today }
    case 'this-month':
      return { from: iso(new Date(Date.UTC(y, m, 1))), to: today }
    case 'last-month':
      return { from: iso(new Date(Date.UTC(y, m - 1, 1))), to: iso(new Date(Date.UTC(y, m, 0))) }
    case 'this-quarter':
      return { from: iso(new Date(Date.UTC(y, m - (m % 3), 1))), to: today }
    case 'ytd':
      return { from: iso(new Date(Date.UTC(y, 0, 1))), to: today }
    case 'custom': {
      const ok = /^\d{4}-\d{2}-\d{2}$/.test(custom.from) && /^\d{4}-\d{2}-\d{2}$/.test(custom.to) && !Number.isNaN(at(custom.from).getTime()) && !Number.isNaN(at(custom.to).getTime())
      if (!ok) return null
      const n = daysBetween(custom.from, custom.to)
      return n >= 1 && n <= MAX_RANGE_DAYS ? custom : null
    }
  }
}

const groups: { id: FinanceGroup; label: string }[] = [{ id: 'day', label: 'Day' }, { id: 'week', label: 'Week' }, { id: 'month', label: 'Month' }]

const count = (n: number) => n.toLocaleString('en-KE')

/** Seconds as people say them. */
export function duration(s: number): string {
  if (s < 90) return `${count(Math.round(s))} s`
  if (s < 5400) return `${count(Math.round(s / 60))} min`
  if (s < 172_800) return `${(s / 3600).toFixed(1)} h`
  return `${count(Math.floor(s / 86_400))} days`
}

function Stat({ label, value, note, money = true, emphasis = false, tone }: { label: string; value: number | string; note?: string | undefined; money?: boolean; emphasis?: boolean; tone?: 'warning' | undefined }) {
  const text = typeof value === 'string' ? value : money ? kes(value) : count(value)
  return (
    <div role="group" aria-label={label} className={cn('grid gap-1 rounded-xl border bg-card p-4 shadow-card', emphasis && 'border-primary/30 bg-primary-container/40', tone === 'warning' && 'border-warning/50 bg-warning/5')}>
      <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</span>
      <span className="font-display text-2xl font-bold tracking-tight md:text-[28px]">{text}</span>
      {note ? <span className="text-sm text-muted-foreground">{note}</span> : null}
    </div>
  )
}

const num = 'text-right tabular-nums'
const th = 'text-xs font-medium uppercase tracking-wide text-muted-foreground'

/** A finance table on its card, with its title and what it shows (every table has a description, 2026-11-11). */
function Frame({ caption, description, children }: { caption: string; description: string; children: ReactNode }) {
  return (
    <div className="mt-6 overflow-x-auto rounded-3xl border bg-card shadow-card animate-rise-in">
      <div className="px-5 pb-4 pt-5">
        <h3 className="m-0 font-display text-lg font-bold tracking-tight">{caption}</h3>
        <p className="m-0 mt-1 max-w-[70ch] text-[14px] leading-relaxed text-muted-foreground">{description}</p>
      </div>
      <Table>
        <TableCaption className="sr-only">{caption}</TableCaption>
        {children}
      </Table>
    </div>
  )
}

function Head({ first, columns }: { first: string; columns: string[] }) {
  return (
    <TableHeader className="bg-muted/50">
      <TableRow className="hover:bg-transparent">
        <TableHead scope="col" className={th}>{first}</TableHead>
        {columns.map((h) => <TableHead key={h} scope="col" className={cn(num, th)}>{h}</TableHead>)}
      </TableRow>
    </TableHeader>
  )
}

const counted = (c: { count: number; kes: number }) => (c.count === 0 ? '–' : `${count(c.count)} · ${count(c.kes)}`)

/** The financial dashboard: every money figure is read from the ledger (GET /v1/staff/finance). */
export function FinancePage() {
  const { api } = useSession()
  const [preset, setPreset] = useState<PresetId>('30')
  const [custom, setCustom] = useState({ from: '', to: '' })
  const [group, setGroup] = useState<FinanceGroup>('day')
  const [exporting, setExporting] = useState<'finance' | 'ledger' | null>(null)
  const [exported, setExported] = useState<{ tone: 'error'; text: string } | { tone: 'success'; file: SavedFile } | null>(null)
  const today = nairobiToday()
  const range = rangeOf(preset, today, custom)
  const customProblem = preset === 'custom' && (custom.from !== '' || custom.to !== '') && range === null

  const download = async (kind: 'finance' | 'ledger') => {
    if (!range) return
    setExporting(kind)
    setExported(null)
    const q = new URLSearchParams({ from: range.from, to: range.to, ...(kind === 'finance' ? { group } : {}) })
    const path = kind === 'finance' ? `/v1/staff/finance/export?${q.toString()}` : `/v1/staff/ledger/export?${q.toString()}`
    const name = `${kind}-${range.from}-${range.to}.csv`
    try {
      const { blob, sha256, rows } = await api.download(path)
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = name
      a.hidden = true
      document.body.appendChild(a)
      a.click()
      a.remove()
      setTimeout(() => { URL.revokeObjectURL(a.href) }, 1000)
      setExported({ tone: 'success', file: { name, sha256, rows, at: new Date(), note: 'The export is in the activity log.' } })
    } catch (e) {
      const text = e instanceof ProblemError ? `${e.title}${e.detail ? `: ${e.detail}` : ''}` : e instanceof Error ? e.message : 'The export failed'
      setExported({ tone: 'error', text })
    } finally {
      setExporting(null)
    }
  }

  return (
    <section>
      <PageHeader
        title="Finance"
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button type="button" size="sm" variant="outline" disabled={!range || exporting !== null} onClick={() => void download('finance')}>
              <Download aria-hidden="true" />{exporting === 'finance' ? 'Exporting…' : 'Export figures (CSV)'}
            </Button>
            <Button type="button" size="sm" variant="outline" disabled={!range || exporting !== null} onClick={() => void download('ledger')}>
              <Download aria-hidden="true" />{exporting === 'ledger' ? 'Exporting…' : 'Export ledger (CSV)'}
            </Button>
          </div>
        }
      >
        Money in and out, what the platform earned and what it owes, from the ledger; the funnels, the trial balance
        and the reconciliations beside it. Dates are Nairobi dates. Exports are recorded in the activity log.
      </PageHeader>

      <div className="flex flex-wrap items-end gap-x-6 gap-y-3">
        <div role="group" aria-label="Range" className="inline-flex flex-wrap rounded-lg border bg-card p-0.5 shadow-card">
          {presets.map((p) => (
            <Button key={p.id} type="button" size="sm" variant={preset === p.id ? 'default' : 'ghost'} aria-pressed={preset === p.id} className="h-8 rounded-md" onClick={() => { setPreset(p.id) }}>
              {p.label}
            </Button>
          ))}
        </div>
        <div role="group" aria-label="Group by" className="inline-flex rounded-lg border bg-card p-0.5 shadow-card">
          {groups.map((g) => (
            <Button key={g.id} type="button" size="sm" variant={group === g.id ? 'default' : 'ghost'} aria-pressed={group === g.id} className="h-8 rounded-md" onClick={() => { setGroup(g.id) }}>
              {g.label}
            </Button>
          ))}
        </div>
        {preset === 'custom' ? (
          <div className="flex flex-wrap items-end gap-3">
            <div className="grid gap-1">
              <Label htmlFor="finance-from">From</Label>
              <Input id="finance-from" type="date" value={custom.from} max={today} onChange={(e) => { setCustom((c) => ({ ...c, from: e.target.value })) }} className="h-9 w-44" aria-describedby={customProblem ? 'finance-range-problem' : undefined} />
            </div>
            <div className="grid gap-1">
              <Label htmlFor="finance-to">To</Label>
              <Input id="finance-to" type="date" value={custom.to} max={today} onChange={(e) => { setCustom((c) => ({ ...c, to: e.target.value })) }} className="h-9 w-44" aria-describedby={customProblem ? 'finance-range-problem' : undefined} />
            </div>
          </div>
        ) : null}
      </div>
      {customProblem ? <p id="finance-range-problem" role="alert" className="mt-2 text-sm text-destructive">{RANGE_PROBLEM}</p> : null}
      {preset === 'custom' && !range && !customProblem ? <p className="mt-2 text-sm text-muted-foreground">Pick the first and last day.</p> : null}
      {exported?.tone === 'success' ? <ExportReceipt file={exported.file} className="max-w-xl" /> : null}
      {exported?.tone === 'error' ? <p role="status" className="mt-3 rounded-2xl border border-destructive/40 bg-destructive/10 px-4 py-3 text-[14.5px] font-medium text-destructive [overflow-wrap:anywhere]">{exported.text}</p> : null}

      {range ? <FinanceData key={`${range.from}:${range.to}:${group}`} from={range.from} to={range.to} group={group} /> : null}
    </section>
  )
}

function FinanceData({ from, to, group }: { from: string; to: string; group: FinanceGroup }) {
  const { api } = useSession()
  const load = useLoad((signal) => api.get<FinanceSummary>('/v1/staff/finance', { from, to, group }, { signal }), [from, to, group])
  const unit = group === 'day' ? 'day' : group === 'week' ? 'week' : 'month'
  return (
    <Loaded load={load} what="the figures" skeleton={<div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">{Array.from({ length: 8 }, (_, i) => <Skeleton key={i} className="h-24 rounded-xl" />)}</div>}>
      {(s) => {
        const labelOf = new Map(s.periods.map((p) => [p.start, periodLabel(p, s.group)]))
        const label = (start: string) => labelOf.get(start) ?? start
        const starts = s.periods.map((p) => p.start)
        return (
          <Tabs defaultValue="overview" className="mt-6">
            <TabsList aria-label="Finance sections" className="h-auto flex-wrap">
              <TabsTrigger value="overview" className="cursor-pointer">Overview</TabsTrigger>
              <TabsTrigger value="topups" className="cursor-pointer">Top-ups</TabsTrigger>
              <TabsTrigger value="payouts" className="cursor-pointer">Payouts &amp; tax</TabsTrigger>
              <TabsTrigger value="trial" className="cursor-pointer">Trial balance{s.trial_balance.balanced ? '' : ' ⚠'}</TabsTrigger>
              <TabsTrigger value="recon" className="cursor-pointer">Reconciliations</TabsTrigger>
            </TabsList>

            <TabsContent value="overview">
              <SectionTitle className="mt-6">{shortDay(s.from)} – {shortDay(s.to)}</SectionTitle>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                <Stat label="Deposits" value={s.totals.deposits_kes} note={s.totals.reversals_kes ? `${kes(s.totals.reversals_kes)} reversed` : undefined} />
                <Stat label="Stakes" value={s.totals.stakes_kes} note={`${count(s.activity.plays)} plays by ${count(s.activity.players)} players`} />
                <Stat label="Prizes awarded" value={s.totals.prizes_awarded_kes} note={`${kes(s.totals.prizes_paid_kes)} paid out`} />
                <Stat label="Gross gaming revenue" value={s.totals.ggr_kes} note="stakes less prizes awarded" emphasis />
                <Stat label="Withdrawals" value={s.totals.withdrawals_kes} />
                <Stat label="Excise (operator pays)" value={s.totals.excise_kes} />
                <Stat label="Tax withheld from prizes" value={s.totals.tax_withheld_kes} note={`${kes(s.totals.tax_remitted_kes)} remitted`} />
                <Stat label="New players" value={s.activity.signups} money={false} />
              </div>

              <LineChart
                title={`Deposits, stakes, prizes paid and withdrawals per ${unit} (KES)`}
                days={starts}
                dayLabel={label}
                format={kes}
                series={[
                  { key: 'deposits', label: 'Deposits', color: 'var(--chart-1)', values: s.periods.map((d) => d.deposits_kes) },
                  { key: 'stakes', label: 'Stakes', color: 'var(--chart-2)', values: s.periods.map((d) => d.stakes_kes) },
                  { key: 'prizes', label: 'Prizes paid', color: 'var(--chart-3)', values: s.periods.map((d) => d.prizes_paid_kes) },
                  { key: 'withdrawals', label: 'Withdrawals', color: 'var(--chart-4)', values: s.periods.map((d) => d.withdrawals_kes) },
                ]}
              />
              <BarChart
                title={`Gross gaming revenue per ${unit} (KES)`}
                categories={starts}
                categoryLabel={label}
                format={kes}
                series={[{ key: 'ggr', label: 'GGR', color: 'var(--chart-2)', values: s.periods.map((d) => d.ggr_kes) }]}
                note="Stakes less prizes awarded; a period in which more was awarded than staked goes below the line."
              />

              <SectionTitle>Now</SectionTitle>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                <Stat label="Owed to players" value={s.balances.owed_to_players_kes} note="wallet balances" />
                <Stat label="Excise owed to KRA" value={s.balances.excise_owed_kes} />
                <Stat label="Withheld tax owed to KRA" value={s.balances.tax_owed_kes} />
                <Stat label="Prize pools" value={s.balances.prize_pools_kes} note="set aside, not yet paid" />
                <Stat label="Player debt" value={s.balances.player_debt_kes} note="from reversed top-ups" />
                <Stat label="Unmatched paybill" value={s.balances.suspense_kes} note="in suspense" />
                <Stat label="M-Pesa clearing" value={s.balances.mpesa_clearing_kes} note="confirmed by M-Pesa, not yet settled" />
                <Stat label="B2C float" value={s.balances.b2c_float_kes} note="negative: paid out beyond what was funded" />
              </div>

              <Frame caption={`Per ${unit}`} description="Money in and out for each period of the range: deposits, stakes, prizes, withdrawals, reversals, tax and what the platform kept.">
                <Head first="Period" columns={['Deposits', 'Stakes', 'Prizes awarded', 'Prizes paid', 'Withdrawals', 'GGR', 'Plays', 'Players', 'New players']} />
                <TableBody>
                  {[...s.periods].reverse().map((d: FinancePeriod) => (
                    <TableRow key={d.start}>
                      <TableHead scope="row" className="font-medium text-foreground">{label(d.start)}</TableHead>
                      <TableCell className={num}>{count(d.deposits_kes)}</TableCell>
                      <TableCell className={num}>{count(d.stakes_kes)}</TableCell>
                      <TableCell className={num}>{count(d.prizes_awarded_kes)}</TableCell>
                      <TableCell className={num}>{count(d.prizes_paid_kes)}</TableCell>
                      <TableCell className={num}>{count(d.withdrawals_kes)}</TableCell>
                      <TableCell className={cn(num, 'font-medium')}>{count(d.ggr_kes)}</TableCell>
                      <TableCell className={num}>{count(d.plays)}</TableCell>
                      <TableCell className={num}>{count(d.players)}</TableCell>
                      <TableCell className={num}>{count(d.signups)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
                <TableFooter>
                  <TableRow>
                    <TableHead scope="row" className="font-semibold text-foreground">Total</TableHead>
                    <TableCell className={num}>{count(s.totals.deposits_kes)}</TableCell>
                    <TableCell className={num}>{count(s.totals.stakes_kes)}</TableCell>
                    <TableCell className={num}>{count(s.totals.prizes_awarded_kes)}</TableCell>
                    <TableCell className={num}>{count(s.totals.prizes_paid_kes)}</TableCell>
                    <TableCell className={num}>{count(s.totals.withdrawals_kes)}</TableCell>
                    <TableCell className={cn(num, 'font-semibold')}>{count(s.totals.ggr_kes)}</TableCell>
                    <TableCell className={num}>{count(s.activity.plays)}</TableCell>
                    <TableCell className={num}>{count(s.activity.players)}</TableCell>
                    <TableCell className={num}>{count(s.activity.signups)}</TableCell>
                  </TableRow>
                </TableFooter>
              </Frame>
            </TabsContent>

            <TabsContent value="topups">
              {(() => {
                const sum = (f: (t: FinanceSummary['top_ups'][number]) => number) => s.top_ups.reduce((a, t) => a + f(t), 0)
                const started = sum((t) => t.started.count)
                const credited = sum((t) => t.credited.count)
                return (
                  <>
                    <div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                      <Stat label="Started" value={started} money={false} note={`${kes(sum((t) => t.started.kes))} asked for`} />
                      <Stat label="Credited" value={credited} money={false} note={`${kes(sum((t) => t.credited.kes))} · ${started ? Math.round((credited / started) * 100) : 0}% of started`} emphasis />
                      <Stat label="Did not complete" value={sum((t) => t.failed.count + t.cancelled.count + t.expired.count)} money={false} note="failed, cancelled by the player, or expired" />
                      <Stat label="Unconfirmed" value={sum((t) => t.unconfirmed.count)} money={false} note="M-Pesa never answered; resolve on Top-ups" tone={sum((t) => t.unconfirmed.count) > 0 ? 'warning' : undefined} />
                    </div>
                    <BarChart
                      title={`Top-ups per ${unit}, by outcome`}
                      categories={starts}
                      categoryLabel={label}
                      format={count}
                      series={[
                        { key: 'credited', label: 'Credited', color: 'var(--chart-2)', values: s.top_ups.map((t) => t.credited.count) },
                        { key: 'failed', label: 'Failed', color: 'var(--chart-4)', values: s.top_ups.map((t) => t.failed.count) },
                        { key: 'cancelled', label: 'Cancelled', color: 'var(--chart-3)', values: s.top_ups.map((t) => t.cancelled.count) },
                        { key: 'expired', label: 'Expired', color: 'var(--chart-5)', values: s.top_ups.map((t) => t.expired.count) },
                        { key: 'unconfirmed', label: 'Unconfirmed', color: 'var(--chart-1)', values: s.top_ups.map((t) => t.unconfirmed.count) },
                        { key: 'pending', label: 'Still pending', color: 'var(--muted-foreground)', values: s.top_ups.map((t) => t.pending_now.count) },
                      ]}
                      note="Counts of top-ups by the period they were started in. Reversed top-ups are among the credited ones; the table lists them."
                    />
                    <Frame caption={`Top-ups per ${unit}: count · KES`} description="How many top-ups ended in each state in each period, and their value in shillings.">
                      <Head first="Period" columns={['Started', 'Credited', 'Failed', 'Cancelled', 'Expired', 'Unconfirmed', 'Reversed', 'Still pending', 'Median time to credit']} />
                      <TableBody>
                        {[...s.top_ups].reverse().map((t) => (
                          <TableRow key={t.start}>
                            <TableHead scope="row" className="font-medium text-foreground">{label(t.start)}</TableHead>
                            <TableCell className={num}>{counted(t.started)}</TableCell>
                            <TableCell className={num}>{counted(t.credited)}</TableCell>
                            <TableCell className={num}>{counted(t.failed)}</TableCell>
                            <TableCell className={num}>{counted(t.cancelled)}</TableCell>
                            <TableCell className={num}>{counted(t.expired)}</TableCell>
                            <TableCell className={num}>{counted(t.unconfirmed)}</TableCell>
                            <TableCell className={num}>{counted(t.reversed)}</TableCell>
                            <TableCell className={num}>{counted(t.pending_now)}</TableCell>
                            <TableCell className={num}>{t.median_seconds_to_credit === null ? '–' : duration(t.median_seconds_to_credit)}</TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Frame>
                  </>
                )
              })()}
            </TabsContent>

            <TabsContent value="payouts">
              <SectionTitle className="mt-6">Withholding tax</SectionTitle>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                <Stat label="Withheld in the range" value={s.tax.withheld_kes} />
                <Stat label="Remitted in the range" value={s.tax.remitted_kes} />
                <Stat label="Owed to KRA now" value={s.balances.tax_owed_kes} note="withheld and not yet remitted" emphasis />
                <Stat label="Batches awaiting remittance" value={s.tax.awaiting_remittance.batches} money={false} note={kes(s.tax.awaiting_remittance.kes)} tone={s.tax.awaiting_remittance.batches > 0 ? 'warning' : undefined} />
                <Stat label="Withholding not yet batched" value={s.tax.unbatched.events} money={false} note={kes(s.tax.unbatched.kes)} />
                <Stat label="Oldest unremitted" value={s.tax.oldest_unremitted_age_seconds === null ? 'Nothing owed' : duration(s.tax.oldest_unremitted_age_seconds)} note={s.tax.oldest_unremitted_age_seconds === null ? undefined : 'since the prize was paid'} tone={(s.tax.oldest_unremitted_age_seconds ?? 0) > 20 * 86_400 ? 'warning' : undefined} />
              </div>

              <Frame caption="Prize pools" description="What each prize pool holds now: the share of stakes set aside for prizes, less what has been awarded.">
                <Head first="Pool" columns={['Balance now', 'Funded in the range', 'Paid in the range']} />
                <TableBody>
                  {s.prize_pools.length === 0 ? (
                    <TableRow><TableCell colSpan={4} className="py-8 text-center text-muted-foreground">No prize pool has been funded yet.</TableCell></TableRow>
                  ) : s.prize_pools.map((p) => (
                    <TableRow key={p.type}>
                      <TableHead scope="row" className="font-medium capitalize text-foreground">{p.type}</TableHead>
                      <TableCell className={num}>{kes(p.balance_kes)}</TableCell>
                      <TableCell className={num}>{kes(p.funded_kes)}</TableCell>
                      <TableCell className={num}>{kes(p.paid_kes)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Frame>

              <Frame caption={`Payouts per ${unit}: count · KES`} description="How many payouts ended in each state in each period, and their value: requested and failed before tax, paid, held and manual after it.">
                <Head first="Period" columns={['Requested', 'Paid', 'Failed', 'Held for approval', 'Manual settlement', 'Tax withheld']} />
                <TableBody>
                  {[...s.payouts].reverse().map((p) => (
                    <TableRow key={p.start}>
                      <TableHead scope="row" className="font-medium text-foreground">{label(p.start)}</TableHead>
                      <TableCell className={num}>{counted(p.requested)}</TableCell>
                      <TableCell className={num}>{counted(p.paid)}</TableCell>
                      <TableCell className={num}>{counted(p.failed)}</TableCell>
                      <TableCell className={num}>{counted(p.held)}</TableCell>
                      <TableCell className={num}>{counted(p.manual)}</TableCell>
                      <TableCell className={num}>{count(p.tax_withheld_kes)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Frame>
            </TabsContent>

            <TabsContent value="trial">
              <div className="mt-6 flex flex-wrap items-center gap-3">
                <Pill tone={s.trial_balance.balanced ? 'success' : 'error'}>{s.trial_balance.balanced ? 'Balanced' : 'Debits and credits differ'}</Pill>
                <span className="text-sm text-muted-foreground">
                  As at the end of {shortDay(s.trial_balance.as_at)}: debits {kes(s.trial_balance.debits_kes)}, credits {kes(s.trial_balance.credits_kes)}.
                </span>
              </div>
              {s.trial_balance.balanced ? null : (
                <p role="alert" className="mt-3 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                  The ledger does not balance. A posting is broken; the figures on this page cannot be trusted until it is found. Raise it now.
                </p>
              )}
              <Frame caption="Trial balance: every ledger account, with its natural sign" description="Every account in the double-entry ledger with its balance for the range. Debits equal credits when the books are right.">
                <TableHeader className="bg-muted/50">
                  <TableRow className="hover:bg-transparent">
                    <TableHead scope="col" className={th}>Account</TableHead>
                    <TableHead scope="col" className={th}>Type</TableHead>
                    {['Debits', 'Credits', 'Balance'].map((h) => <TableHead key={h} scope="col" className={cn(num, th)}>{h}</TableHead>)}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {s.trial_balance.accounts.map((a) => (
                    <TableRow key={a.code}>
                      <TableHead scope="row" className="font-medium text-foreground"><span className="font-mono text-[13px]">{a.code}</span><span className="block text-xs font-normal text-muted-foreground">{a.name}</span></TableHead>
                      <TableCell className="capitalize">{a.type}</TableCell>
                      <TableCell className={num}>{count(a.debits_kes)}</TableCell>
                      <TableCell className={num}>{count(a.credits_kes)}</TableCell>
                      <TableCell className={cn(num, 'font-medium')}>{count(a.balance_kes)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
                <TableFooter>
                  <TableRow>
                    <TableHead scope="row" colSpan={2} className="font-semibold text-foreground">Total</TableHead>
                    <TableCell className={cn(num, 'font-semibold')}>{count(s.trial_balance.debits_kes)}</TableCell>
                    <TableCell className={cn(num, 'font-semibold')}>{count(s.trial_balance.credits_kes)}</TableCell>
                    <TableCell className={num}>{s.trial_balance.balanced ? 'balanced' : `off by ${count(Math.abs(s.trial_balance.debits_kes - s.trial_balance.credits_kes))}`}</TableCell>
                  </TableRow>
                </TableFooter>
              </Frame>

              <Frame caption="Journals posted in the range, by kind" description="How many ledger journals of each kind were posted in the range, and the money they moved.">
                <Head first="Kind" columns={['Journals', 'Debited', 'Credited']} />
                <TableBody>
                  {s.by_kind.length === 0 ? (
                    <TableRow><TableCell colSpan={4} className="py-8 text-center text-muted-foreground">Nothing was posted in the range.</TableCell></TableRow>
                  ) : s.by_kind.map((k) => (
                    <TableRow key={k.kind}>
                      <TableHead scope="row" className="font-mono text-[13px] font-medium text-foreground">{k.kind}</TableHead>
                      <TableCell className={num}>{count(k.journals)}</TableCell>
                      <TableCell className={num}>{count(k.debit_kes)}</TableCell>
                      <TableCell className={num}>{count(k.credit_kes)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Frame>
            </TabsContent>

            <TabsContent value="recon">
              <Reconciliations />
            </TabsContent>
          </Tabs>
        )
      }}
    </Loaded>
  )
}

const areas = [
  { id: 'payments', label: 'Payments', path: '/v1/staff/payments/reconcile', description: 'Top-ups and paybill payments against M-Pesa and the ledger.' },
  { id: 'game', label: 'Game', path: '/v1/staff/game/reconcile', description: 'Plays, stakes and chances against the ledger.' },
  { id: 'draws', label: 'Draws', path: '/v1/staff/draws/reconcile', description: 'Entries, seeds and prize pools against the ledger.' },
  { id: 'payouts', label: 'Payouts', path: '/v1/staff/payouts/reconcile', description: 'Payouts against M-Pesa and the ledger.' },
] as const

type AreaId = (typeof areas)[number]['id']

const findings = (m: Record<string, number>) => Object.entries(m).filter(([, n]) => n > 0).map(([k, n]) => `${k} × ${count(n)}`)

/** Every stored run of one area's reconciliation, newest first, and a way to run it now. */
function Reconciliations() {
  const { api } = useSession()
  const [area, setArea] = useState<AreaId>('payments')
  const current = areas.find((a) => a.id === area) ?? areas[0]
  const pages = usePages<ReconciliationRun>(async (cursor, signal) => {
    const r = await api.get<{ runs: ReconciliationRun[]; next_before: string | null }>('/v1/staff/reconciliations', { area, limit: '20', ...(cursor ? { before: cursor } : {}) }, { signal })
    return { rows: r.runs, next: r.next_before }
  }, [area])
  return (
    <div className="mt-6 grid gap-6">
      <Tabs value={area} onValueChange={(v) => { setArea(v as AreaId) }}>
        <TabsList aria-label="Area" className="h-auto flex-wrap">
          {areas.map((a) => <TabsTrigger key={a.id} value={a.id} className="cursor-pointer">{a.label}</TabsTrigger>)}
        </TabsList>
      </Tabs>
      <ReconcileCard path={current.path} title={`${current.label} reconciliation`} description={current.description} onDone={pages.reload} />
      <Loaded load={pages} what="the reconciliation runs">
        {(runs) => runs.length === 0 ? (
          <Empty>No {current.label.toLowerCase()} reconciliation has run yet.</Empty>
        ) : (
          <>
            <Frame caption={`${current.label} reconciliation runs, newest first`} description="Each time this reconciliation ran: whether the books agreed, and what differed when they did not.">
              <TableHeader className="bg-muted/50">
                <TableRow className="hover:bg-transparent">
                  <TableHead scope="col" className={th}>Ran at</TableHead>
                  <TableHead scope="col" className={th}>Outcome</TableHead>
                  <TableHead scope="col" className={th}>Mismatches</TableHead>
                  <TableHead scope="col" className={th}>Needs attention</TableHead>
                  <TableHead scope="col" className={th}>Totals</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs.map((r) => (
                  <TableRow key={r.id}>
                    <TableHead scope="row" className="whitespace-nowrap font-medium text-foreground">{eat(r.ran_at)}</TableHead>
                    <TableCell><Pill tone={r.ok ? 'success' : 'error'}>{r.ok ? 'Everything matched' : 'Mismatches'}</Pill></TableCell>
                    <TableCell className="text-sm">{findings(r.mismatches).join(', ') || '–'}</TableCell>
                    <TableCell className="text-sm">{findings(r.attention).join(', ') || '–'}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">{Object.entries(r.totals).map(([k, v]) => `${k} ${count(v)}`).join(' · ') || '–'}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Frame>
            {pages.more ? (
              <div><Button type="button" variant="outline" disabled={pages.loadingMore} onClick={pages.loadMore}>{pages.loadingMore ? 'Loading…' : 'Older runs'}</Button></div>
            ) : null}
          </>
        )}
      </Loaded>
    </div>
  )
}
