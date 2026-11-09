// Shapes of the /v1/staff answers (api/openapi.yaml, back-office tag).

/** The two back-office roles (ADR 0019): an administrator may do everything staff may. */
export type Role = 'admin' | 'staff'

export interface Me {
  id: string
  name: string
  email: string
  roles: Role[]
  /** signed in with a temporary password: only changing it is allowed until done */
  must_change_password: boolean
}

export interface KycSubmission {
  user_id: string
  msisdn: string
  national_id: string
  full_name: string
  date_of_birth: string
  submitted_at: string
}

export interface Draw {
  id: string
  draw_type: 'daily' | 'weekly' | 'monthly' | 'quarterly'
  period_start: string
  period_end: string
  /** personal: the players whose own period (counted from their first play) ends on period_end */
  basis: 'calendar' | 'personal'
  status: 'open' | 'frozen' | 'signed' | 'conducted' | 'approved' | 'paid' | 'voided'
  freeze_at: string
  entries: number
  drand_round_at: string | null
  signoffs: { step: 'sign' | 'approve' | 'void'; attestation: string; signed_at: string }[]
  results: { rank: number; amount_kes: number; entry_no: number }[]
  voided_reason: string | null
}

export interface Late { draw_id: string; draw_type: string; period_start: string; status: string; due: string; problem: string }

export interface Payout {
  id: string
  user_id: string
  kind: 'withdrawal' | 'prize'
  status: string
  state: string
  gross_kes: number
  tax_kes: number
  net_kes: number
  debt_repaid_kes: number
  receipt?: string
  reason?: string
  /** a refund to source (ADR 0031): why, which admin asked, and the number that paid in */
  refund?: { reason: string; requested_by: string; msisdn: string }
  /** a held payout: who approved it so far; two different people release it (ADR 0032) */
  approvals?: string[]
  /** a manual payout: the "not paid" settlement one person proposed, waiting for a second to confirm it (ADR 0032) */
  proposal?: { paid: boolean; receipt?: string; reason?: string; by: string; at: string }
  /** a manual payout: the receipt staff claimed from the statement and M-Pesa's answer; '' while it is asked (ADR 0034) */
  check?: { receipt: string; by: string; at: string; outcome: '' | 'confirmed' | 'refuted' | 'no_answer'; detail?: string }
  created_at: string
}

export interface Batch {
  batch_no: number
  events: number
  gross_kes: number
  tax_kes: number
  file_sha256: string
  prepared_by: string
  prepared_at: string
  prn?: string
  remitted_by?: string
  remitted_at?: string
}

export interface Alert {
  id: string
  kind: string
  subject: string
  severity: string
  freezes_payouts: boolean
  summary: string
  /** what the check found: ids, amounts, ages (and escalated_at once a warning became a page) */
  details?: Record<string, unknown>
  first_seen_at: string
  last_seen_at: string
  occurrences: number
  notifications?: number
  acknowledged_by?: string
  acknowledged_at?: string
  ack_note?: string
  resolved_by?: string
  resolved_at?: string
}

export interface Closure { user_id: string; msisdn: string; status: string; requested_at: string }

/** A registered player as the back office lists them (GET /v1/staff/players). */
export interface Player {
  user_id: string
  msisdn: string
  /** where a forgotten password's reset link goes (ADR 0021); null for accounts from before addresses were asked for */
  email: string | null
  kyc_status: 'none' | 'pending' | 'verified' | 'manual_review' | 'rejected'
  status: 'active' | 'suspended' | 'closed'
  phone_verified: boolean
  balance_kes: number
  plays: number
  created_at: string
  last_sign_in_at: string | null
  closure_requested_at: string | null
  /** from the latest identity submission; empty when the player never submitted one */
  full_name: string
  national_id: string
}

export interface Reconciliation {
  ran_at: string
  ok: boolean
  mismatches: Record<string, string[] | undefined> | null
  attention?: Record<string, string[] | undefined> | null
  totals: Record<string, number> | null
}

/** A back-office member as admins see them (GET /v1/staff/members). */
export interface StaffMember {
  id: string
  email: string
  name: string
  roles: Role[]
  active: boolean
  invited_by: string
  providers: string[]
  last_sign_in: string | null
  created_at: string
  /** can sign in with a password; must_change_password: it is a temporary one an admin set */
  has_password: boolean
  must_change_password: boolean
}

/** One platform's counts on one day, or over the range (GET /v1/staff/metrics). Money in whole KES. */
export interface MetricFigures {
  signups: number
  sessions: number
  active_players: number
  plays: number
  stakes_kes: number
  top_ups: number
  top_ups_kes: number
  withdrawals: number
  withdrawals_kes: number
}

export interface MetricsSummary {
  from: string
  to: string
  platforms: string[]
  days: (MetricFigures & { day: string; platform: string })[]
  totals: (MetricFigures & { platform: string })[]
  all: MetricFigures
}

/** One period's, or a range's, money movements in whole KES (GET /v1/staff/finance). */
export interface Figures {
  deposits_kes: number
  stakes_kes: number
  prizes_awarded_kes: number
  prizes_paid_kes: number
  withdrawals_kes: number
  reversals_kes: number
  excise_kes: number
  tax_withheld_kes: number
  tax_remitted_kes: number
  ggr_kes: number
}

/** How a range is cut: Nairobi days, Monday-start weeks or calendar months, the first and last clipped to the range. */
export type FinanceGroup = 'day' | 'week' | 'month'

/** One day, week or month of the range (start and end inclusive) with its figures and activity. */
export interface FinancePeriod extends Figures {
  start: string
  end: string
  plays: number
  players: number
  signups: number
}

/** How many of something, and how much money they carry. */
export interface Counted { count: number; kes: number }

/** The top-ups started in one period, by how they ended; reversed ones count among the credited ones too. */
export interface TopUpFunnelPeriod {
  start: string
  end: string
  started: Counted
  credited: Counted
  failed: Counted
  cancelled: Counted
  expired: Counted
  unconfirmed: Counted
  reversed: Counted
  pending_now: Counted
  median_seconds_to_credit: number | null
}

/** The payouts requested in one period, by where they stand now. */
export interface PayoutFunnelPeriod {
  start: string
  end: string
  requested: Counted
  paid: Counted
  failed: Counted
  held: Counted
  manual: Counted
  tax_withheld_kes: number
}

/** One ledger account's line of the trial balance; the balance carries the account's natural sign. */
export interface TrialBalanceAccount {
  code: string
  name: string
  type: 'asset' | 'liability' | 'revenue' | 'expense'
  debits_kes: number
  credits_kes: number
  balance_kes: number
}

export interface FinanceSummary {
  from: string
  to: string
  group: FinanceGroup
  totals: Figures
  periods: FinancePeriod[]
  balances: {
    owed_to_players_kes: number
    player_debt_kes: number
    excise_owed_kes: number
    tax_owed_kes: number
    prize_pools_kes: number
    suspense_kes: number
    mpesa_clearing_kes: number
    b2c_float_kes: number
  }
  activity: { signups: number; plays: number; players: number }
  /** every journal kind posted in the range */
  by_kind: { kind: string; journals: number; debit_kes: number; credit_kes: number }[]
  /** every account as at the end of the range; balanced is false when a posting is broken */
  trial_balance: { as_at: string; accounts: TrialBalanceAccount[]; debits_kes: number; credits_kes: number; balanced: boolean }
  top_ups: TopUpFunnelPeriod[]
  payouts: PayoutFunnelPeriod[]
  prize_pools: { type: string; balance_kes: number; funded_kes: number; paid_kes: number }[]
  tax: {
    withheld_kes: number
    remitted_kes: number
    awaiting_remittance: { batches: number; kes: number }
    unbatched: { events: number; kes: number }
    oldest_unremitted_age_seconds: number | null
  }
}

/** One stored reconciliation run (GET /v1/staff/reconciliations): findings are counted per kind. */
export interface ReconciliationRun {
  id: string
  area: 'payments' | 'game' | 'draws' | 'payouts'
  ran_at: string
  ok: boolean
  mismatches: Record<string, number>
  attention: Record<string, number>
  totals: Record<string, number>
}

/** One paid play (GET /v1/staff/plays). */
export interface PlayRow {
  ticket_id: string
  user_id: string
  msisdn: string
  full_name: string | null
  play_date: string
  seq: number
  price_kes: number
  status: 'paid' | 'refunded'
  slot: number | null
  lucky: boolean | null
  created_at: string
}

/** One top-up (GET /v1/staff/topups). */
export interface TopUpRow {
  id: string
  user_id: string
  msisdn: string
  full_name: string | null
  channel: 'stk' | 'c2b'
  amount_requested_kes: number
  amount_confirmed_kes: number | null
  status: string
  failure_reason: string | null
  result_code: number | null
  receipt: string | null
  sandbox: boolean
  /** the receipt the player gave for an uncredited top-up, and what came of it */
  claimed_receipt: string | null
  claim_status: 'claimed' | 'checking' | 'confirmed' | 'unverified' | 'rejected' | null
  created_at: string
  updated_at: string
}

/** Safaricom's STK result, parsed (GET /v1/staff/topups/{id}). */
export interface CallbackView {
  received_at?: string
  result_code: number
  result_desc: string
  amount_kes?: number
  receipt?: string
  payer?: string
  unreadable?: boolean
}

/** Everything about one top-up (GET /v1/staff/topups/{id}). */
export interface TopUpDetail extends TopUpRow {
  merchant_request_id: string | null
  checkout_request_id: string | null
  reversed_at: string | null
  reversal_reason: string | null
  callback: CallbackView | null
  raw_callback?: unknown
  claim: { receipt: string; status: NonNullable<TopUpRow['claim_status']>; claimed_at: string; note: string | null; originator: string | null; result?: unknown } | null
  orphan_callbacks: CallbackView[]
  jobs: { kind: string; state: string; attempt: number; scheduled_at: string; finalized_at: string | null; last_error?: string }[]
  events: AuditEvent[]
  player: { status: string; kyc_status: string; phone_verified: boolean; email: string | null; balance_kes: number; top_ups: number; credited_kes: number; unconfirmed_count: number }
}

/** What M-Pesa said when an operator asked again (POST /v1/staff/topups/{id}/recheck). */
export interface Recheck {
  outcome: 'credited' | 'paid_without_receipt' | 'failed' | 'cancelled' | 'pending' | 'no_prompt' | 'asking' | 'verifying'
  status: string
  result_code: number | null
  result_desc?: string
}

/** One statement row and what the platform found for it (POST /v1/staff/payments/statement/match). */
export interface StatementMatch {
  row: { line: number; receipt: string; completed_at: string; paid_in_kes: number; withdrawn_kes?: number; party: string; party_number: string; account: string; details: string; status: string }
  kind: 'credited' | 'claimed' | 'matched' | 'ambiguous' | 'unmatched' | 'skipped'
  payment_id?: string
  user_id?: string
  msisdn?: string
  status?: string
  reason?: string
  candidates?: { payment_id: string; user_id: string; msisdn: string; amount_kes: number; status: string; created_at: string; why: string }[]
}

export interface StatementCredit { payment_id: string; receipt: string; amount_kes: number }

export interface StatementReport {
  sha256: string
  rows: number
  skipped?: string[]
  from: string
  to: string
  counts: Record<string, number>
  matches: StatementMatch[]
  missing_from_statement?: { payment_id: string; user_id: string; receipt: string; amount_kes: number; created_at: string }[]
  to_credit: StatementCredit[]
}

export interface StatementApplied {
  import_id: string
  credited: number
  credited_kes: number
  results: { payment_id: string; receipt: string; amount_kes: number; credited: boolean; error?: string }[]
}

export interface StatementImport {
  id: string
  imported_by: string
  imported_at: string
  filename: string
  sha256: string
  rows: number
  matched: number
  credited: number
  credited_kes: number
}

/** One audit record (GET /v1/staff/activity). */
export interface AuditEvent {
  id: string
  at: string
  actor_type: 'system' | 'player' | 'staff'
  actor_id: string | null
  action: string
  entity_type: string
  entity_id: string | null
  details: Record<string, unknown>
}

/** The system's state (GET /v1/staff/system). */
export interface SystemState {
  now: string
  settings: {
    daraja: string
    sandbox_settlement: boolean
    top_up_min_kes: number
    top_up_cooldown_s: number
    buy_goods: boolean
    paybill: boolean
    payouts: boolean
    email: boolean
    google_sign_in: boolean
    apple_sign_in: boolean
    attestation: string
    kyc_registry: boolean
    alerts_delivery: string
    heartbeat: boolean
    workers: string
    started_at: string
    version?: string
    play_price_kes: number | null
    wheel_slots: number | null
    wheel_lucky_slots: number | null
  }
  jobs: { kind: string; state: string; count: number }[]
  failed_jobs: { id: number; kind: string; state: string; attempt: number; last_error: string; at: string }[]
  reconciliations: { area: string; ran_at: string | null; ok: boolean | null }[]
  open_alerts: number
  players: number
  plays_today: number
  top_ups_pending: number
  top_ups_unconfirmed: number
  receipt_claims: number
}

/** One player's timeline (GET /v1/staff/players/{id}): what happened to them and their money (owner, 2026-10-31). */
export interface PlayerTimeline {
  player: Player
  wallet: {
    balance_kes: number
    /** what the player owes after reversed top-ups */
    debt_kes: number
    transactions: { id: string; kind: string; amount_kes: number; journal_id: string; reference: string | null; created_at: string }[]
  }
  top_ups: {
    id: string
    channel: 'stk' | 'c2b'
    status: string
    amount_requested_kes: number
    amount_confirmed_kes: number | null
    receipt: string | null
    failure_reason: string | null
    result_code: number | null
    created_at: string
  }[]
  plays: {
    count: number
    last: { ticket_id: string; play_date: string; seq: number; price_kes: number; status: 'paid' | 'refunded'; slot: number | null; lucky: boolean | null; created_at: string }[]
  }
  payouts: { id: string; kind: 'prize' | 'withdrawal'; state: string; gross_kes: number; tax_kes: number; net_kes: number; receipt: string | null; created_at: string }[]
  identity: {
    /** submissions' outcomes, newest first, without the personal data they carry */
    kyc: { id: string; status: string; provider: string; reviewed_by: string | null; reason: string | null; created_at: string }[]
    sessions: { id: string; platform: string; created_at: string; revoked_at: string | null; revoked_reason: string | null; device_verdict: string }[]
    /** every change of the account's status, newest first (ADR 0030); from is null for the first */
    status_history: { id: number; from: string | null; to: 'active' | 'suspended' | 'closed'; actor_type: 'player' | 'staff' | 'system' | null; actor: string | null; reason: string | null; at: string }[]
    /** where sign-ups and money moves came from, newest first (ADR 0031); country null when the edge said nothing */
    locations: { id: number; action: 'signup' | 'topup' | 'play' | 'withdrawal'; country: string | null; refused: boolean; at: string }[]
    closure_requested_at: string | null
  }
  limits: {
    deposit: { period: 'daily' | 'weekly' | 'monthly'; amount_kes: number; effective_from: string; pending: boolean }[]
    restriction: { kind: 'timeout' | 'self_exclusion'; until: string | null } | null
  }
  /** audit records about the player, newest first */
  events: AuditEvent[]
}

/** GET /v1/staff/alerts: a page of alerts, newest first, and how many of each kind are open (ADR 0025). */
export interface AlertKindCount { kind: string; open: number }
export interface AlertsPage { alerts: Alert[]; next_before: string | null; kinds: AlertKindCount[] }
export type AlertState = 'open' | 'acknowledged' | 'resolved' | 'all'

/** One thing that happened to an alert: raised, escalated, alert.acknowledged, resolved. */
export interface AlertEvent { at: string; event: string; actor: string; note?: string }

/** GET /v1/staff/alerts/{id}: the alert, the check that owns it and its history. */
export interface AlertDetail extends Alert { check: string; history: AlertEvent[] }

/** One recorded change of the operating mode (ADR 0027). */
export interface OperatingChange {
  id: string
  mode: 'disabled' | 'sandbox' | 'live'
  reason: string
  changed_by: string
  at: string
  licence_reference?: string
}

/** A request to operate with real money, waiting for a second admin. */
export interface LiveRequest {
  id: string
  licence_reference: string
  approval_reference: string
  evidence_sha256: string
  evidence_location: string
  reason: string
  requested_by: string
  at: string
}

/** The operating mode (GET /v1/staff/operating). */
export interface OperatingState {
  status: { mode: 'disabled' | 'sandbox' | 'live'; configured: 'disabled' | 'sandbox' | 'live'; real_money: boolean; last_change: OperatingChange | null }
  live_requests: LiveRequest[]
  history: OperatingChange[]
}
