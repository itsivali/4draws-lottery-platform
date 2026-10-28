import type { Role } from './types'

export interface Section { path: string; label: string; roles: Role[] }

/**
 * The back office's sections and the role each needs (ADR 0019). `staff` opens every operational queue and the
 * finance dashboard; `admin` opens everything, and only admins see the Staff and App metrics pages. The API checks
 * again on every call.
 */
export const sections: Section[] = [
  { path: '/finance', label: 'Finance', roles: ['staff'] },
  { path: '/metrics', label: 'App metrics', roles: ['admin'] },
  { path: '/players', label: 'Players', roles: ['staff'] },
  { path: '/kyc', label: 'Identity review', roles: ['staff'] },
  { path: '/draws', label: 'Draws', roles: ['staff'] },
  { path: '/payouts', label: 'Payouts', roles: ['staff'] },
  { path: '/payments', label: 'Payments', roles: ['staff'] },
  { path: '/tax', label: 'KRA tax', roles: ['staff'] },
  { path: '/alerts', label: 'Alerts', roles: ['staff'] },
  { path: '/closures', label: 'Account closures', roles: ['staff'] },
  { path: '/staff', label: 'Staff', roles: ['admin'] },
]

export const roleLabels: Record<Role, string> = {
  admin: 'Administrator',
  staff: 'Staff',
}

export const roleDescriptions: Record<Role, string> = {
  admin: 'Everything staff can do, plus the Staff page (members, roles, passwords) and App metrics.',
  staff: 'Identity review, payments, draws, payouts, KRA tax, alerts, account closures and the Finance dashboard.',
}

/** Whether roles allow something needing one of `needed`. An administrator may do everything staff may. */
export function can(roles: Role[], ...needed: Role[]): boolean {
  return roles.includes('admin') || needed.some((r) => roles.includes(r))
}

export function visibleSections(roles: Role[]): Section[] {
  return sections.filter((s) => can(roles, ...s.roles))
}
