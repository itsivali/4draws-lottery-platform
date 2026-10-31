import { useId, useState } from 'react'
import { Table, TableBody, TableCaption, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

export interface BarSeries { key: string; label: string; color: string; values: number[] }

const W = 720
const H = 260
const PAD = { top: 16, right: 24, bottom: 28, left: 64 }

/** A round step for about four gridlines. */
function niceStep(span: number): number {
  if (span <= 0) return 1
  const raw = span / 4
  const p = 10 ** Math.floor(Math.log10(raw))
  const m = raw / p
  return (m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10) * p
}

const compact = (n: number) => new Intl.NumberFormat('en-KE', { notation: 'compact', maximumFractionDigits: 1 }).format(n)

/**
 * One bar per category, stacked when there are several series (positive values stack up from zero, negative ones
 * down). A legend and a table view carry the same numbers, so the colour is never the only key; hovering a category
 * shows every value in it.
 */
export function BarChart({ title, categories, categoryLabel, series, format, note }: {
  title: string
  categories: string[]
  categoryLabel: (c: string) => string
  series: BarSeries[]
  format: (n: number) => string
  note?: string | undefined
}) {
  const id = useId()
  const [hover, setHover] = useState<number | null>(null)
  const sums = categories.map((_, i) => {
    let pos = 0
    let neg = 0
    for (const s of series) {
      const v = s.values[i] ?? 0
      if (v >= 0) pos += v
      else neg += v
    }
    return { pos, neg }
  })
  const max = Math.max(0, ...sums.map((s) => s.pos))
  const min = Math.min(0, ...sums.map((s) => s.neg))
  const step = niceStep(max - min)
  const top = max > 0 ? Math.ceil(max / step) * step : min < 0 ? 0 : step
  const bottom = min < 0 ? Math.floor(min / step) * step : 0
  const iw = W - PAD.left - PAD.right
  const ih = H - PAD.top - PAD.bottom
  const y = (v: number) => PAD.top + ih - ((v - bottom) / (top - bottom)) * ih
  const slot = categories.length > 0 ? iw / categories.length : iw
  const bar = Math.max(2, Math.min(slot * 0.7, 48))
  const x = (i: number) => PAD.left + slot * i + (slot - bar) / 2
  const ticks: number[] = []
  for (let t = bottom; t <= top + step / 1000; t += step) ticks.push(Math.round(t * 1000) / 1000)

  const pick = (clientX: number, rect: DOMRect) => {
    const px = ((clientX - rect.left) / rect.width) * W
    const i = Math.floor((px - PAD.left) / slot)
    setHover(i >= 0 && i < categories.length ? i : null)
  }

  return (
    <figure className="m-0 mt-6 rounded-xl border bg-card p-4 shadow-card md:p-5" aria-labelledby={`${id}-t`}>
      <figcaption id={`${id}-t`} className="text-base font-semibold">{title}</figcaption>
      {series.length > 1 ? (
        <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 p-0 text-sm text-muted-foreground" aria-label="Legend">
          {series.map((s) => (
            <li key={s.key} className="flex items-center gap-1.5"><span className="inline-block size-2.5 rounded-sm" style={{ background: s.color }} aria-hidden="true" />{s.label}</li>
          ))}
        </ul>
      ) : null}
      <div className="relative mt-3">
        <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`${title}. Values are in the table view below.`} className="block h-auto w-full"
          onMouseMove={(e) => { pick(e.clientX, e.currentTarget.getBoundingClientRect()) }}
          onMouseLeave={() => { setHover(null) }}>
          {ticks.map((t) => (
            <g key={t}>
              <line className={t === 0 ? 'stroke-foreground/50' : 'stroke-border'} x1={PAD.left} x2={W - PAD.right} y1={y(t)} y2={y(t)} />
              <text className="fill-muted-foreground text-[11px]" x={PAD.left - 8} y={y(t)} dy="0.32em" textAnchor="end">{compact(t)}</text>
            </g>
          ))}
          {categories.length > 0 ? (
            [0, Math.floor((categories.length - 1) / 2), categories.length - 1].filter((v, i, a) => a.indexOf(v) === i).map((i) => (
              <text key={i} className="fill-muted-foreground text-[11px]" x={x(i) + bar / 2} y={H - 8} textAnchor="middle">{categoryLabel(categories[i] ?? '')}</text>
            ))
          ) : null}
          {categories.map((c, i) => {
            let up = 0
            let down = 0
            return (
              <g key={c} opacity={hover === null || hover === i ? 1 : 0.55}>
                {series.map((s) => {
                  const v = s.values[i] ?? 0
                  if (v === 0) return null
                  let y0: number
                  let y1: number
                  if (v > 0) {
                    y0 = y(up + v)
                    y1 = y(up)
                    up += v
                  } else {
                    y0 = y(down)
                    y1 = y(down + v)
                    down += v
                  }
                  return <rect key={s.key} x={x(i)} y={y0} width={bar} height={Math.max(0.5, y1 - y0)} fill={s.color} rx={1.5} />
                })}
              </g>
            )
          })}
        </svg>
        {hover !== null ? (
          <div aria-hidden="true" className="pointer-events-none absolute top-2 z-10 grid min-w-44 -translate-x-1/2 gap-0.5 rounded-lg border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-float" style={{ left: `${((x(hover) + bar / 2) / W) * 100}%` }}>
            <strong className="mb-0.5 text-[13px]">{categoryLabel(categories[hover] ?? '')}</strong>
            {series.map((s) => (
              <span key={s.key} className="flex items-center gap-1.5"><span className="inline-block size-2 rounded-sm" style={{ background: s.color }} />{s.label}: {format(s.values[hover] ?? 0)}</span>
            ))}
          </div>
        ) : null}
      </div>
      {note ? <p className="mt-2 text-xs text-muted-foreground">{note}</p> : null}
      <details className="mt-3 text-sm">
        <summary className="cursor-pointer text-muted-foreground">Table view</summary>
        <div className="mt-2 overflow-x-auto rounded-lg border">
          <Table>
            <TableCaption className="sr-only">{title}, as a table</TableCaption>
            <TableHeader className="bg-muted/50">
              <TableRow className="hover:bg-transparent">
                <TableHead scope="col">Period</TableHead>
                {series.map((s) => <TableHead key={s.key} scope="col" className="text-right">{s.label}</TableHead>)}
              </TableRow>
            </TableHeader>
            <TableBody>
              {categories.map((c, i) => (
                <TableRow key={c}>
                  <TableHead scope="row" className="font-medium text-foreground">{categoryLabel(c)}</TableHead>
                  {series.map((s) => <TableCell key={s.key} className="text-right tabular-nums">{format(s.values[i] ?? 0)}</TableCell>)}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </details>
    </figure>
  )
}
