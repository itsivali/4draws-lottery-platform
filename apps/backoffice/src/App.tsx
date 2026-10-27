import { Link, NavLink, Navigate, Route, Routes } from 'react-router-dom'
import { useSession } from './context'
import { AlertsPage } from './pages/Alerts'
import { ClosuresPage } from './pages/Closures'
import { DrawsPage } from './pages/Draws'
import { FinancePage } from './pages/Finance'
import { KycPage } from './pages/Kyc'
import { OverviewPage } from './pages/Overview'
import { PaymentsPage } from './pages/Payments'
import { PayoutsPage } from './pages/Payouts'
import { StaffPage } from './pages/Staff'
import { TaxPage } from './pages/Tax'
import { ReviewQueueProvider, useReviewQueue } from './reviewQueue'
import { visibleSections } from './roles'

const pages: Record<string, () => React.JSX.Element> = {
  '/finance': FinancePage,
  '/staff': StaffPage,
  '/kyc': KycPage,
  '/draws': DrawsPage,
  '/payouts': PayoutsPage,
  '/payments': PaymentsPage,
  '/tax': TaxPage,
  '/alerts': AlertsPage,
  '/closures': ClosuresPage,
}

/** The signed-in back office: only the sections the member's roles allow are linked or routed. */
export function App() {
  return (
    <ReviewQueueProvider>
      <Layout />
    </ReviewQueueProvider>
  )
}

function Layout() {
  const { me, signOut } = useSession()
  const { queue, arrived, dismiss } = useReviewQueue()
  const sections = visibleSections(me.roles)
  return (
    <div className="layout">
      <a className="skip" href="#main">Skip to content</a>
      <nav aria-label="Back office">
        <p className="brand">Back office</p>
        <ul>
          <li><NavLink to="/" end>Overview</NavLink></li>
          {sections.map((s) => (
            <li key={s.path}>
              <NavLink to={s.path}>
                {s.label}
                {s.path === '/kyc' && queue && queue.waiting > 0 ? (
                  <>
                    <span className="badge" aria-hidden="true">{queue.waiting}</span>
                    <span className="sr-only">, {queue.waiting} waiting</span>
                  </>
                ) : null}
              </NavLink>
            </li>
          ))}
        </ul>
        <button type="button" className="secondary signout" onClick={signOut}>Sign out</button>
      </nav>
      <main id="main" tabIndex={-1}>
        {arrived > 0 ? (
          <div className="notice notice-pending arrival" role="status">
            <p>{arrived === 1 ? 'A new identity submission is waiting for review.' : `${arrived} new identity submissions are waiting for review.`}</p>
            <Link to="/kyc" onClick={dismiss}>Review now</Link>
            <button type="button" className="secondary" onClick={dismiss}>Dismiss</button>
          </div>
        ) : null}
        <Routes>
          <Route path="/" element={<OverviewPage />} />
          {sections.map((s) => {
            const Page = pages[s.path]
            return Page ? <Route key={s.path} path={s.path} element={<Page />} /> : null
          })}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  )
}
