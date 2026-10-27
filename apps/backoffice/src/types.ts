// Shapes of the /v1/staff answers (api/openapi.yaml, back-office tag).

export type Role =
  | 'kyc_reviewer'
  | 'payments_operator'
  | 'draw_official'
  | 'payout_operator'
  | 'tax_officer'
  | 'alerts_operator'
  | 'support'
  | 'auditor'

export interface Me { id: string; name: string; email: string; roles: Role[] }

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

export interface Reconciliation {
  ran_at: string
  ok: boolean
  mismatches: Record<string, string[] | undefined> | null
  attention?: Record<string, string[] | undefined> | null
  totals: Record<string, number> | null
}
