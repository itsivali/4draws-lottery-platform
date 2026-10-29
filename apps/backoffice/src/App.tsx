import {
  Banknote, Bell, ChartSpline, ChevronsUpDown, IdCard, KeyRound, Landmark, LayoutDashboard, LogOut, Receipt, Search, Ticket, UserPen, UserRound, UserX, Users, Wallet,
} from 'lucide-react'
import { useEffect, useState, type ComponentType } from 'react'
import { BrandGlyph } from './components/BrandGlyph'
import { NameDialog } from './components/NameDialog'
import { PasswordDialog } from './components/PasswordDialog'
import { MetricsPage } from './pages/Metrics'
import { Link, NavLink, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { useSession } from './context'
import { AlertsPage } from './pages/Alerts'
import { ClosuresPage } from './pages/Closures'
import { DrawsPage } from './pages/Draws'
import { FinancePage } from './pages/Finance'
import { KycPage } from './pages/Kyc'
import { OverviewPage } from './pages/Overview'
import { PaymentsPage } from './pages/Payments'
import { PayoutsPage } from './pages/Payouts'
import { PlayersPage } from './pages/Players'
import { StaffPage } from './pages/Staff'
import { TaxPage } from './pages/Tax'
import { ReviewQueueProvider, useReviewQueue } from './reviewQueue'
import { roleLabels, visibleSections, type Section } from './roles'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Breadcrumb, BreadcrumbItem, BreadcrumbLink, BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator } from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator } from '@/components/ui/command'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Separator } from '@/components/ui/separator'
import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarHeader, SidebarInset, SidebarMenu, SidebarMenuBadge,
  SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarRail, SidebarTrigger, useSidebar,
} from '@/components/ui/sidebar'

const pages: Record<string, () => React.JSX.Element> = {
  '/finance': FinancePage,
  '/metrics': MetricsPage,
  '/staff': StaffPage,
  '/players': PlayersPage,
  '/kyc': KycPage,
  '/draws': DrawsPage,
  '/payouts': PayoutsPage,
  '/payments': PaymentsPage,
  '/tax': TaxPage,
  '/alerts': AlertsPage,
  '/closures': ClosuresPage,
}

const icons: Record<string, ComponentType<{ className?: string }>> = {
  '/finance': Landmark,
  '/metrics': ChartSpline,
  '/players': UserRound,
  '/kyc': IdCard,
  '/draws': Ticket,
  '/payouts': Banknote,
  '/payments': Wallet,
  '/tax': Receipt,
  '/alerts': Bell,
  '/closures': UserX,
  '/staff': Users,
}

/** The navigation's groups, in order; a section appears under the first group that lists it. */
const groups: { label: string; paths: string[] }[] = [
  { label: 'Money', paths: ['/finance', '/payments', '/payouts', '/tax'] },
  { label: 'Apps', paths: ['/metrics'] },
  { label: 'Players', paths: ['/players', '/kyc', '/closures'] },
  { label: 'Operations', paths: ['/draws', '/alerts'] },
  { label: 'Administration', paths: ['/staff'] },
]

/** The signed-in back office: only the sections the member's roles allow are linked or routed. */
export function App() {
  return (
    <ReviewQueueProvider>
      <SidebarProvider>
        <Layout />
      </SidebarProvider>
    </ReviewQueueProvider>
  )
}

function Layout() {
  const { me } = useSession()
  const { arrived, dismiss } = useReviewQueue()
  const sections = visibleSections(me.roles)
  const { pathname } = useLocation()
  const current = sections.find((s) => pathname === s.path || pathname.startsWith(`${s.path}/`))
  return (
    <>
      <a className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 focus:rounded-lg focus:bg-card focus:px-3 focus:py-2 focus:shadow-float" href="#main">Skip to content</a>
      <AppSidebar sections={sections} />
      <SidebarInset id="main" tabIndex={-1} className="min-w-0 outline-none">
        <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center gap-2 border-b bg-background/80 px-4 backdrop-blur supports-[backdrop-filter]:bg-background/60">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-1 data-[orientation=vertical]:h-4" />
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                {current ? <BreadcrumbLink asChild><Link to="/">Overview</Link></BreadcrumbLink> : <BreadcrumbPage>Overview</BreadcrumbPage>}
              </BreadcrumbItem>
              {current ? (
                <>
                  <BreadcrumbSeparator />
                  <BreadcrumbItem><BreadcrumbPage>{current.label}</BreadcrumbPage></BreadcrumbItem>
                </>
              ) : null}
            </BreadcrumbList>
          </Breadcrumb>
          <div className="ml-auto flex items-center gap-1">
            <CommandMenu sections={sections} />
          </div>
        </header>
        <div className="mx-auto w-full max-w-[1200px] px-4 py-6 md:px-8 md:py-8">
          {arrived > 0 ? (
            <Alert role="status" className="mb-6 border-pending/40 bg-pending/10 [&>svg]:text-pending">
              <IdCard />
              <AlertTitle>{arrived === 1 ? 'A new identity submission is waiting for review.' : `${arrived} new identity submissions are waiting for review.`}</AlertTitle>
              <AlertDescription className="flex flex-wrap items-center gap-2">
                <Button asChild size="sm"><Link to="/kyc" onClick={dismiss}>Review now</Link></Button>
                <Button type="button" size="sm" variant="ghost" onClick={dismiss}>Dismiss</Button>
              </AlertDescription>
            </Alert>
          ) : null}
          <Routes>
            <Route path="/" element={<OverviewPage />} />
            {sections.map((s) => {
              const Page = pages[s.path]
              return Page ? <Route key={s.path} path={s.path} element={<Page />} /> : null
            })}
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </div>
      </SidebarInset>
    </>
  )
}

function AppSidebar({ sections }: { sections: Section[] }) {
  const { me, signOut, updateMe } = useSession()
  const { queue } = useReviewQueue()
  const { pathname } = useLocation()
  const { isMobile, setOpenMobile } = useSidebar()
  const close = () => { if (isMobile) setOpenMobile(false) }
  const waiting = queue && queue.waiting > 0 ? queue.waiting : 0
  const initials = me.name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase() ?? '').join('')
  const roles = me.roles.map((r) => roleLabels[r]).join(', ') || 'No role'
  const [changing, setChanging] = useState(false)
  const [renaming, setRenaming] = useState(false)
  return (
    <Sidebar collapsible="icon" variant="inset">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild tooltip="Overview">
              <Link to="/" onClick={close}>
                <span aria-hidden="true" className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-card">
                  <BrandGlyph />
                </span>
                <span className="grid flex-1 text-left leading-tight">
                  <span className="truncate font-display text-[15px] font-bold tracking-tight">Tajirika</span>
                  <span className="truncate text-xs text-muted-foreground">Back office</span>
                </span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <nav aria-label="Back office" className="contents">
          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton asChild isActive={pathname === '/'} tooltip="Overview">
                    <NavLink to="/" end onClick={close}><LayoutDashboard /><span>Overview</span></NavLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
          {groups.map((g) => {
            const items = g.paths.map((p) => sections.find((s) => s.path === p)).filter((s): s is Section => s !== undefined)
            if (items.length === 0) return null
            return (
              <SidebarGroup key={g.label}>
                <SidebarGroupLabel>{g.label}</SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {items.map((s) => {
                      const Icon = icons[s.path] ?? LayoutDashboard
                      const active = pathname === s.path || pathname.startsWith(`${s.path}/`)
                      return (
                        <SidebarMenuItem key={s.path}>
                          <SidebarMenuButton asChild isActive={active} tooltip={s.label}>
                            <NavLink to={s.path} onClick={close}>
                              <Icon />
                              <span>{s.label}</span>
                              {s.path === '/kyc' && waiting > 0 ? <span className="sr-only">, {waiting} waiting</span> : null}
                            </NavLink>
                          </SidebarMenuButton>
                          {s.path === '/kyc' && waiting > 0 ? (
                            <SidebarMenuBadge aria-hidden="true" className="rounded-full bg-primary px-1.5 text-primary-foreground">{waiting}</SidebarMenuBadge>
                          ) : null}
                        </SidebarMenuItem>
                      )
                    })}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            )
          })}
        </nav>
      </SidebarContent>
      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <SidebarMenuButton size="lg" className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground">
                  <span aria-hidden="true" className="flex size-8 items-center justify-center rounded-lg bg-primary-container text-sm font-semibold text-primary-container-foreground">{initials}</span>
                  <span className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-medium">{me.name}</span>
                    <span className="truncate text-xs text-muted-foreground">{roles}</span>
                  </span>
                  <ChevronsUpDown aria-hidden="true" className="ml-auto size-4" />
                </SidebarMenuButton>
              </DropdownMenuTrigger>
              <DropdownMenuContent side={isMobile ? 'bottom' : 'right'} align="end" sideOffset={4} className="min-w-56 rounded-lg">
                <DropdownMenuLabel className="font-normal">
                  <span className="block truncate font-medium">{me.name}</span>
                  <span className="block truncate text-xs text-muted-foreground">{roles}</span>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem onSelect={() => { setRenaming(true) }}><UserPen /> Change name</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => { setChanging(true) }}><KeyRound /> Change password</DropdownMenuItem>
                <DropdownMenuItem onSelect={signOut}><LogOut /> Sign out</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <PasswordDialog open={changing} onClose={() => { setChanging(false) }} />
            <NameDialog open={renaming} onClose={() => { setRenaming(false) }} onSaved={updateMe} />
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}


/** ⌘K: jump to a section or sign out. */
function CommandMenu({ sections }: { sections: Section[] }) {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const { signOut } = useSession()
  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); setOpen((o) => !o) }
    }
    document.addEventListener('keydown', down)
    return () => { document.removeEventListener('keydown', down) }
  }, [])
  const go = (path: string) => { setOpen(false); void navigate(path) }
  return (
    <>
      <Button type="button" variant="outline" size="sm" className="hidden h-8 gap-2 text-muted-foreground md:inline-flex" onClick={() => { setOpen(true) }}>
        <Search aria-hidden="true" />
        <span>Jump to…</span>
        <kbd aria-hidden="true" className="pointer-events-none rounded border bg-muted px-1.5 font-mono text-[10px] font-medium">⌘K</kbd>
      </Button>
      <Button type="button" variant="ghost" size="icon-sm" className="md:hidden" aria-label="Jump to a section" onClick={() => { setOpen(true) }}><Search /></Button>
      <CommandDialog open={open} onOpenChange={setOpen} title="Jump to" description="Search the back office's sections">
        <CommandInput placeholder="Jump to a section…" />
        <CommandList>
          <CommandEmpty>No section matches.</CommandEmpty>
          <CommandGroup heading="Sections">
            <CommandItem value="Overview" onSelect={() => { go('/') }}><LayoutDashboard /> Overview</CommandItem>
            {sections.map((s) => {
              const Icon = icons[s.path] ?? LayoutDashboard
              return <CommandItem key={s.path} value={s.label} onSelect={() => { go(s.path) }}><Icon /> {s.label}</CommandItem>
            })}
          </CommandGroup>
          <CommandSeparator />
          <CommandGroup heading="Session">
            <CommandItem value="Sign out" onSelect={() => { setOpen(false); signOut() }}><LogOut /> Sign out</CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </>
  )
}
