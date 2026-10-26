/** Whole shillings: "KES 1,250". */
export function kes(n: number): string {
  return `${n < 0 ? '−' : ''}KES ${Math.abs(n).toLocaleString('en-KE')}`
}

/** An instant in Nairobi time (EAT, UTC+3, no daylight saving). */
export function eat(iso: string): string {
  const d = new Date(iso)
  return (
    new Intl.DateTimeFormat('en-GB', {
      timeZone: 'Africa/Nairobi',
      day: 'numeric',
      month: 'short',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    }).format(d) + ' EAT'
  )
}

/** "254712345678" → "0712 345 678". */
export function msisdn(m: string): string {
  const x = /^254(\d{3})(\d{3})(\d{3})$/.exec(m)
  return x ? `0${x[1] ?? ''} ${x[2] ?? ''} ${x[3] ?? ''}` : m
}
