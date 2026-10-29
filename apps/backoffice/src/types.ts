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
  first_seen_at: string
  last_seen_at: string
  occurrences: number
  acknowledged_by?: string
  acknowledged_at?: string
  ack_note?: string
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

/** One day's, or a range's, money movements in whole KES (GET /v1/staff/finance). */
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

export interface FinanceSummary {
  from: string
  to: string
  totals: Figures
  days: (Figures & { day: string })[]
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
}
