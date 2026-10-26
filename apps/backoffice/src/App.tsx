import { NavLink, Navigate, Route, Routes } from 'react-router-dom'
import { useSession } from './context'
import { AlertsPage } from './pages/Alerts'
import { ClosuresPage } from './pages/Closures'
import { DrawsPage } from './pages/Draws'
import { KycPage } from './pages/Kyc'
import { OverviewPage } from './pages/Overview'
import { PaymentsPage } from './pages/Payments'
import { PayoutsPage } from './pages/Payouts'
import { TaxPage } from './pages/Tax'
import { visibleSections } from './roles'

const pages: Record<string, () => React.JSX.Element> = {
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
  const { me, signOut } = useSession()
  const sections = visibleSections(me.roles)
  return (
    <div className="layout">
      <a className="skip" href="#main">Skip to content</a>
      <nav aria-label="Back office">
        <p className="brand">Back office</p>
        <ul>
          <li><NavLink to="/" end>Overview</NavLink></li>
          {sections.map((s) => <li key={s.path}><NavLink to={s.path}>{s.label}</NavLink></li>)}
        </ul>
        <button type="button" className="secondary signout" onClick={signOut}>Sign out</button>
      </nav>
      <main id="main" tabIndex={-1}>
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
