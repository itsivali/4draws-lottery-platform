import { Globe, Smartphone, TabletSmartphone, type LucideIcon } from 'lucide-react'
import { useState } from 'react'
import { LineChart } from '../components/LineChart'
import { Loaded, PageHeader, SectionTitle } from '../components/ui'
import { useSession } from '../context'
import { kes } from '../format'
import { useLoad } from '../hooks'
import type { MetricFigures, MetricsSummary } from '../types'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCaption, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

const ranges = [7, 30, 90] as const

const shortDay = (d: string) => new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short', timeZone: 'UTC' }).format(new Date(`${d}T00:00:00Z`))

function nairobiToday(): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Africa/Nairobi' }).format(new Date())
}

function minusDays(d: string, n: number): string {
  const t = new Date(`${d}T00:00:00Z`)
  t.setUTCDate(t.getUTCDate() - n)
  return t.toISOString().slice(0, 10)
}

const count = (n: number) => n.toLocaleString('en-KE')

/** The measures the chart can show, each with how to read a cell and how to print a value. */
const measures = [
  { key: 'active_players', label: 'Players seen', money: false },
  { key: 'signups', label: 'Sign-ups', money: false },
  { key: 'sessions', label: 'Sign-ins', money: false },
  { key: 'plays', label: 'Plays', money: false },
  { key: 'stakes_kes', label: 'Stakes (KES)', money: true },
  { key: 'top_ups_kes', label: 'Top-ups (KES)', money: true },
  { key: 'withdrawals_kes', label: 'Withdrawals (KES)', money: true },
] as const satisfies readonly { key: keyof MetricFigures; label: string; money: boolean }[]

type MeasureKey = (typeof measures)[number]['key']

/** Each platform's name, icon and chart colour: the same everywhere on the page, and never the only key. */
const platforms: Record<string, { label: string; icon: LucideIcon; color: string }> = {
  web: { label: 'Web', icon: Globe, color: 'var(--chart-1)' },
  android: { label: 'Android', icon: Smartphone, color: 'var(--chart-2)' },
  ios: { label: 'iOS', icon: TabletSmartphone, color: 'var(--chart-3)' },
  unknown: { label: 'Unknown', icon: Globe, color: 'var(--chart-5)' },
}

const platformOf = (p: string) => platforms[p] ?? { label: p, icon: Globe, color: 'var(--chart-5)' }

function PlatformCard({ platform, figures, share }: { platform: string; figures: MetricFigures; share: number }) {
  const { label, icon: Icon, color } = platformOf(platform)
  return (
    <div role="group" aria-label={label} className="grid gap-3 rounded-xl border bg-card p-4 shadow-card">
      <div className="flex items-center gap-2">
        <span aria-hidden="true" className="flex size-8 items-center justify-center rounded-lg text-white" style={{ background: color }}><Icon className="size-4" /></span>
        <span className="font-semibold">{label}</span>
        <span className="ml-auto text-sm text-muted-foreground">{Math.round(share * 100)}% of players seen</span>
      </div>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        <div><dt className="text-xs uppercase tracking-wide text-muted-foreground">Players seen</dt><dd className="font-display text-xl font-bold tracking-tight">{count(figures.active_players)}</dd></div>
        <div><dt className="text-xs uppercase tracking-wide text-muted-foreground">Sign-ups</dt><dd className="font-display text-xl font-bold tracking-tight">{count(figures.signups)}</dd></div>
        <div><dt className="text-xs uppercase tracking-wide text-muted-foreground">Plays</dt><dd className="font-medium tabular-nums">{count(figures.plays)} <span className="font-normal text-muted-foreground">· {kes(figures.stakes_kes)}</span></dd></div>
        <div><dt className="text-xs uppercase tracking-wide text-muted-foreground">Sign-ins</dt><dd className="font-medium tabular-nums">{count(figures.sessions)}</dd></div>
        <div><dt className="text-xs uppercase tracking-wide text-muted-foreground">Top-ups</dt><dd className="font-medium tabular-nums">{count(figures.top_ups)} <span className="font-normal text-muted-foreground">· {kes(figures.top_ups_kes)}</span></dd></div>
        <div><dt className="text-xs uppercase tracking-wide text-muted-foreground">Withdrawals</dt><dd className="font-medium tabular-nums">{count(figures.withdrawals)} <span className="font-normal text-muted-foreground">· {kes(figures.withdrawals_kes)}</span></dd></div>
      </dl>
    </div>
  )
}

const num = 'text-right tabular-nums'

/** How the web and mobile apps are used, per platform and Nairobi day (GET /v1/staff/metrics; administrators). */
export function MetricsPage() {
  const { api } = useSession()
  const [days, setDays] = useState<number>(30)
  const [measure, setMeasure] = useState<MeasureKey>('active_players')
  const load = useLoad((signal) => {
    const to = nairobiToday()
    return api.get<MetricsSummary>('/v1/staff/metrics', { from: minusDays(to, days - 1), to }, { signal })
  }, [days])
  const current = measures.find((m) => m.key === measure) ?? measures[0]
  return (
    <section>
      <PageHeader
        title="App metrics"
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
        How players use the web app and the Android and iOS apps: sign-ups, sign-ins, players seen, plays and money,
        by the platform of the device they used. Read from the platform’s own records; nothing is tracked elsewhere.
      </PageHeader>
      <Loaded load={load} what="the metrics" skeleton={<div className="grid gap-3 lg:grid-cols-3">{Array.from({ length: 3 }, (_, i) => <Skeleton key={i} className="h-44 rounded-xl" />)}</div>}>
        {(s) => {
          const uniqueDays = [...new Set(s.days.map((d) => d.day))]
          const series = s.platforms.map((p) => ({
            key: p, label: platformOf(p).label, color: platformOf(p).color,
            values: uniqueDays.map((day) => s.days.find((c) => c.day === day && c.platform === p)?.[measure] ?? 0),
          }))
          const seen = Math.max(1, s.all.active_players)
          return (
            <>
              <SectionTitle className="mt-0">{shortDay(s.from)} – {shortDay(s.to)} · {count(s.all.active_players)} players seen, {count(s.all.signups)} new, {count(s.all.plays)} plays</SectionTitle>
              <div className="grid gap-3 lg:grid-cols-3">
                {s.totals.map((t) => <PlatformCard key={t.platform} platform={t.platform} figures={t} share={t.active_players / seen} />)}
              </div>

              <SectionTitle>Per day</SectionTitle>
              <Tabs value={measure} onValueChange={(v) => { setMeasure(v as MeasureKey) }}>
                <TabsList aria-label="Measure" className="h-auto flex-wrap">
                  {measures.map((m) => <TabsTrigger key={m.key} value={m.key} className="cursor-pointer">{m.label}</TabsTrigger>)}
                </TabsList>
              </Tabs>
              <LineChart
                title={`${current.label} per day, by platform`}
                days={uniqueDays}
                dayLabel={shortDay}
                format={current.money ? kes : count}
                series={series}
              />
              <p className="mt-2 text-xs text-muted-foreground">
                A player seen on two days counts on both; sign-ups follow the player’s first device; money follows the device the player used that day.
              </p>

              <div className="mt-8 overflow-hidden rounded-xl border bg-card shadow-card">
                <Table>
                  <TableCaption className="mt-0 border-b px-4 py-3 text-left text-base font-semibold text-foreground caption-top">{current.label} per day</TableCaption>
                  <TableHeader className="bg-muted/50">
                    <TableRow className="hover:bg-transparent">
                      <TableHead scope="col" className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Day</TableHead>
                      {s.platforms.map((p) => <TableHead key={p} scope="col" className={cn(num, 'text-xs font-medium uppercase tracking-wide text-muted-foreground')}>{platformOf(p).label}</TableHead>)}
                      <TableHead scope="col" className={cn(num, 'text-xs font-medium uppercase tracking-wide text-muted-foreground')}>All</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {[...uniqueDays].reverse().map((day) => {
                      const row = s.platforms.map((p) => s.days.find((c) => c.day === day && c.platform === p)?.[measure] ?? 0)
                      const fmt = (n: number) => (current.money ? n.toLocaleString('en-KE') : count(n))
                      return (
                        <TableRow key={day}>
                          <TableHead scope="row" className="font-medium text-foreground">{shortDay(day)}</TableHead>
                          {row.map((v, i) => <TableCell key={s.platforms[i]} className={num}>{fmt(v)}</TableCell>)}
                          <TableCell className={cn(num, 'font-medium')}>{fmt(row.reduce((a, b) => a + b, 0))}</TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </div>
            </>
          )
        }}
      </Loaded>
    </section>
  )
}
