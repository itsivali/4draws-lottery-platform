/**
 * The platform's background jobs in words: what each kind is called, what it does, and how often it runs. A kind not
 * listed here (a new one) is shown by its code name.
 */
export interface JobInfo {
  name: string
  does: string
  area: JobArea
  /** how often it is scheduled; absent: it runs when something asks for it */
  every?: string
}

export type JobArea = 'Top-ups' | 'Payouts and prizes' | 'Draws and game' | 'Identity' | 'Monitoring'

export const jobAreas: JobArea[] = ['Top-ups', 'Payouts and prizes', 'Draws and game', 'Identity', 'Monitoring']

export const jobInfo: Record<string, JobInfo> = {
  'payments.stk_push': { area: 'Top-ups', name: 'Send M-Pesa prompts', does: 'Asks M-Pesa to show the player the PIN prompt for a top-up they started.' },
  'payments.stk_verify': { area: 'Top-ups', name: 'Confirm prompt payments', does: 'Asks M-Pesa how a prompt ended when its callback is late or unclear.' },
  'payments.c2b_verify': { area: 'Top-ups', name: 'Confirm paybill payments', does: 'Checks a paybill payment with M-Pesa before the player is credited.' },
  'payments.claim_check': { area: 'Top-ups', name: 'Check player receipts', does: 'Asks M-Pesa about a receipt a player gave for a top-up that was not credited.' },
  'payments.sweep': { area: 'Top-ups', name: 'Find stuck top-ups', does: 'Picks up top-ups left waiting too long and checks them again.', every: 'Every minute' },
  'payments.reconcile': { area: 'Top-ups', name: 'Reconcile top-ups', does: 'Checks every payment against the ledger and records any mismatch.', every: 'Daily' },
  'payouts.send': { area: 'Payouts and prizes', name: 'Send payouts', does: 'Sends a withdrawal or prize to the player’s M-Pesa (B2C).' },
  'payouts.verify': { area: 'Payouts and prizes', name: 'Confirm payouts', does: 'Follows a sent payout until M-Pesa says it was paid or not.' },
  'payouts.sweep': { area: 'Payouts and prizes', name: 'Find stuck payouts', does: 'Picks up payouts left without an answer and asks again.', every: 'Every 5 minutes' },
  'payouts.prizes': { area: 'Payouts and prizes', name: 'Pay draw prizes', does: 'Creates the payouts for the winners of approved draws.', every: 'Every 5 minutes' },
  'payouts.reconcile': { area: 'Payouts and prizes', name: 'Reconcile payouts', does: 'Checks payouts against M-Pesa and the ledger.', every: 'Daily' },
  'draws.schedule': { area: 'Draws and game', name: 'Schedule draws', does: 'Opens, freezes and runs draws when their times come.', every: 'Every 5 minutes' },
  'draws.reconcile': { area: 'Draws and game', name: 'Reconcile draws', does: 'Checks entries, seeds and prize pools against the ledger.', every: 'Daily' },
  'game.reconcile': { area: 'Draws and game', name: 'Reconcile plays', does: 'Checks every spin’s stake and prize against the ledger.', every: 'Daily' },
  'kyc.verify': { area: 'Identity', name: 'Check identities', does: 'Checks a player’s ID details with the registry, when one is set up.' },
  'kyc.review_notice': { area: 'Identity', name: 'Tell reviewers', does: 'Emails reviewers that an identity submission is waiting.' },
  'aml.scan': { area: 'Monitoring', name: 'Run monitoring rules', does: 'Runs the compliance rules and opens a case for each player they find.', every: 'Hourly' },
  'alerts.watch': { area: 'Monitoring', name: 'Watch for problems', does: 'Runs the health checks and sends the alerts that are due.', every: 'Every 5 minutes' },
}

/** A job kind's info, or its code name when it is not in the catalog. */
export function infoOf(kind: string): JobInfo {
  return jobInfo[kind] ?? { area: 'Monitoring', name: kind, does: 'A background job.' }
}

/** River's job states in words, and how each reads. */
export const stateWords: Record<string, { label: string; tone: 'success' | 'pending' | 'warning' | 'error' | 'neutral' }> = {
  completed: { label: 'done in 24 h', tone: 'success' },
  running: { label: 'running', tone: 'pending' },
  available: { label: 'ready to run', tone: 'pending' },
  scheduled: { label: 'scheduled', tone: 'neutral' },
  retryable: { label: 'retrying', tone: 'warning' },
  discarded: { label: 'gave up', tone: 'error' },
  cancelled: { label: 'cancelled', tone: 'neutral' },
  pending: { label: 'pending', tone: 'neutral' },
}

/** "3 min ago" or "in 2 h", against now. */
export function relative(iso: string, now: Date): string {
  const s = Math.round((new Date(iso).getTime() - now.getTime()) / 1000)
  const a = Math.abs(s)
  const v = a < 60 ? `${String(a)} s` : a < 3600 ? `${String(Math.round(a / 60))} min` : a < 86400 ? `${String(Math.round(a / 3600))} h` : `${String(Math.round(a / 86400))} d`
  if (a < 5) return 'now'
  return s < 0 ? `${v} ago` : `in ${v}`
}
