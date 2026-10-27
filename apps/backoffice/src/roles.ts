import type { Role } from './types'

export interface Section { path: string; label: string; roles: Role[] }

/** The back office's sections, and the roles that may open each (the API checks again on every call). */
export const sections: Section[] = [
  { path: '/finance', label: 'Finance', roles: ['finance', 'auditor'] },
  { path: '/kyc', label: 'Identity review', roles: ['kyc_reviewer'] },
  { path: '/draws', label: 'Draws', roles: ['draw_official', 'auditor'] },
  { path: '/payouts', label: 'Payouts', roles: ['payout_operator', 'auditor'] },
  { path: '/payments', label: 'Payments', roles: ['payments_operator', 'auditor'] },
  { path: '/tax', label: 'KRA tax', roles: ['tax_officer', 'auditor'] },
  { path: '/alerts', label: 'Alerts', roles: ['alerts_operator', 'auditor'] },
  { path: '/closures', label: 'Account closures', roles: ['support', 'auditor'] },
  { path: '/staff', label: 'Staff', roles: ['admin'] },
]

export const roleLabels: Record<Role, string> = {
  admin: 'Admin',
  finance: 'Finance',
  kyc_reviewer: 'Identity reviewer',
  payments_operator: 'Payments operator',
  draw_official: 'Draw official',
  payout_operator: 'Payout operator',
  tax_officer: 'Tax officer',
  alerts_operator: 'Alerts operator',
  support: 'Support',
  auditor: 'Auditor',
}

export function can(roles: Role[], ...needed: Role[]): boolean {
  return needed.some((r) => roles.includes(r))
}

export function visibleSections(roles: Role[]): Section[] {
  return sections.filter((s) => can(roles, ...s.roles))
}
