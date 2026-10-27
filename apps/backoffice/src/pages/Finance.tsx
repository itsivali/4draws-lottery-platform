import { useState } from 'react'
import { LineChart } from '../components/LineChart'
import { Loaded, PageHeader, SectionTitle } from '../components/ui'
import { useSession } from '../context'
import { kes } from '../format'
import { useLoad } from '../hooks'
import type { FinanceSummary } from '../types'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCaption, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const ranges = [7, 30, 90] as const

/** "2026-10-26" → "26 Oct". */
const shortDay = (d: string) => new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short', timeZone: 'UTC' }).format(new Date(`${d}T00:00:00Z`))

function nairobiToday(): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Africa/Nairobi' }).format(new Date())
}

function minusDays(d: string, n: number): string {
  const t = new Date(`${d}T00:00:00Z`)
  t.setUTCDate(t.getUTCDate() - n)
  return t.toISOString().slice(0, 10)
}

function Stat({ label, value, note, money = true, emphasis = false }: { label: string; value: number; note?: string | undefined; money?: boolean; emphasis?: boolean }) {
  return (
    <div role="group" aria-label={label} className={cn('grid gap-1 rounded-xl border bg-card p-4 shadow-card', emphasis && 'border-primary/30 bg-primary-container/40')}>
      <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</span>
      <span className="font-display text-2xl font-bold tracking-tight md:text-[28px]">{money ? kes(value) : value.toLocaleString('en-KE')}</span>
      {note ? <span className="text-sm text-muted-foreground">{note}</span> : null}
    </div>
  )
}

const num = 'text-right tabular-nums'

/** The financial dashboard: every figure is read from the ledger (GET /v1/staff/finance). */
export function FinancePage() {
  const { api } = useSession()
  const [days, setDays] = useState<number>(30)
  const load = useLoad(() => {
    const to = nairobiToday()
    return api.get<FinanceSummary>('/v1/staff/finance', { from: minusDays(to, days - 1), to })
  }, [days])
  return (
    <section>
      <PageHeader
        title="Finance"
        actions={
          <div role="group" aria-label="Range" className="inline-flex rounded-lg border bg-card p-0.5 shadow-card">
            {ranges.map((n) => (
              <Button key={n} type="button" size="sm" variant={days === n ? 'default' : 'ghost'} aria-pressed={days === n} className="h-8 rounded-md" onClick={() => { setDays(n) }}>
                Last {n} days
              </Button>
            ))}
          </div>
        }
      >
        Money in and out, what the platform earned and what it owes, from the ledger. Days are Nairobi days.
      </PageHeader>
      <Loaded load={load} what="the figures" skeleton={<div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">{Array.from({ length: 8 }, (_, i) => <Skeleton key={i} className="h-24 rounded-xl" />)}</div>}>
        {(s) => (
          <>
            <SectionTitle className="mt-0">{shortDay(s.from)} – {shortDay(s.to)}</SectionTitle>
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <Stat label="Deposits" value={s.totals.deposits_kes} note={s.totals.reversals_kes ? `${kes(s.totals.reversals_kes)} reversed` : undefined} />
              <Stat label="Stakes" value={s.totals.stakes_kes} note={`${s.activity.plays.toLocaleString('en-KE')} plays by ${s.activity.players.toLocaleString('en-KE')} players`} />
              <Stat label="Prizes awarded" value={s.totals.prizes_awarded_kes} note={`${kes(s.totals.prizes_paid_kes)} paid out`} />
              <Stat label="Gross gaming revenue" value={s.totals.ggr_kes} note="stakes less prizes awarded" emphasis />
              <Stat label="Withdrawals" value={s.totals.withdrawals_kes} />
              <Stat label="Excise (operator pays)" value={s.totals.excise_kes} />
              <Stat label="Tax withheld from prizes" value={s.totals.tax_withheld_kes} note={`${kes(s.totals.tax_remitted_kes)} remitted`} />
              <Stat label="New players" value={s.activity.signups} money={false} />
            </div>

            <LineChart
              title="Deposits, stakes, prizes paid and withdrawals per day (KES)"
              days={s.days.map((d) => d.day)}
              dayLabel={shortDay}
              format={kes}
              series={[
                { key: 'deposits', label: 'Deposits', color: 'var(--chart-1)', values: s.days.map((d) => d.deposits_kes) },
                { key: 'stakes', label: 'Stakes', color: 'var(--chart-2)', values: s.days.map((d) => d.stakes_kes) },
                { key: 'prizes', label: 'Prizes paid', color: 'var(--chart-3)', values: s.days.map((d) => d.prizes_paid_kes) },
                { key: 'withdrawals', label: 'Withdrawals', color: 'var(--chart-4)', values: s.days.map((d) => d.withdrawals_kes) },
              ]}
            />

            <SectionTitle>Now</SectionTitle>
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              <Stat label="Owed to players" value={s.balances.owed_to_players_kes} note="wallet balances" />
              <Stat label="Excise owed to KRA" value={s.balances.excise_owed_kes} />
              <Stat label="Withheld tax owed to KRA" value={s.balances.tax_owed_kes} />
              <Stat label="Prize pools" value={s.balances.prize_pools_kes} note="set aside, not yet paid" />
              <Stat label="Player debt" value={s.balances.player_debt_kes} note="from reversed top-ups" />
              <Stat label="Unmatched paybill" value={s.balances.suspense_kes} note="in suspense" />
            </div>

            <div className="mt-8 overflow-hidden rounded-xl border bg-card shadow-card">
              <Table>
                <TableCaption className="mt-0 border-b px-4 py-3 text-left text-base font-semibold text-foreground caption-top">Per day</TableCaption>
                <TableHeader className="bg-muted/50">
                  <TableRow className="hover:bg-transparent">
                    <TableHead scope="col" className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Day</TableHead>
                    {['Deposits', 'Stakes', 'Prizes paid', 'Withdrawals', 'GGR'].map((h) => (
                      <TableHead key={h} scope="col" className={cn(num, 'text-xs font-medium uppercase tracking-wide text-muted-foreground')}>{h}</TableHead>
                    ))}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {[...s.days].reverse().map((d) => (
                    <TableRow key={d.day}>
                      <TableHead scope="row" className="font-medium text-foreground">{shortDay(d.day)}</TableHead>
                      <TableCell className={num}>{d.deposits_kes.toLocaleString('en-KE')}</TableCell>
                      <TableCell className={num}>{d.stakes_kes.toLocaleString('en-KE')}</TableCell>
                      <TableCell className={num}>{d.prizes_paid_kes.toLocaleString('en-KE')}</TableCell>
                      <TableCell className={num}>{d.withdrawals_kes.toLocaleString('en-KE')}</TableCell>
                      <TableCell className={cn(num, 'font-medium')}>{d.ggr_kes.toLocaleString('en-KE')}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </>
        )}
      </Loaded>
    </section>
  )
}
