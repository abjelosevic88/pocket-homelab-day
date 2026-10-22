// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { type ReactNode, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { addDays, fmtDate, todayISO, weekday } from '../lib/format'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { Avatar, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

const SIDEBAR_KEY = 'pd-sidebar'
/** Where Settings goes back to. */
export const LAST_PAGE_KEY = 'pd-last-page'

/** An entry or a moment was saved: the sidebar (and anything listening) reloads. */
export const announceChange = () => window.dispatchEvent(new CustomEvent('pd:changed'))
export function useOnChange(cb: () => void) {
  const ref = useRef(cb)
  ref.current = cb
  useEffect(() => {
    const h = () => ref.current()
    window.addEventListener('pd:changed', h)
    return () => window.removeEventListener('pd:changed', h)
  }, [])
}

export type WriteKind = 'entry' | 'moment'

export default function Layout() {
  const { settings, setThemeMode, theme } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])
  useEffect(() => { sessionStorage.setItem(LAST_PAGE_KEY, loc.pathname + loc.search) }, [loc.pathname, loc.search])

  const [palette, setPalette] = useState(false)
  const ai = !!settings?.ai_configured

  /** Today's page with the entry (or the moment field) focused. */
  const write = (kind: WriteKind) => { closeDrawer(); navigate('/', { state: { focus: kind } }) }
  const writeRef = useRef(write)
  writeRef.current = write

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t?.closest?.('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); window.dispatchEvent(new CustomEvent('pd:shortcuts')); break
        case 'n': e.preventDefault(); writeRef.current('entry'); break
        case 'm': e.preventDefault(); writeRef.current('moment'); break
        case 't': go('/'); break
        case 'c': go('/calendar'); break
        case 'j': go('/journal'); break
        case 'y': go('/year'); break
        case 'a': if (ai) go('/ask'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const today = todayISO()
  const commands: PaletteCommand[] = [
    { id: 'write', label: "Write today's entry", icon: 'edit', hint: 'N', keywords: 'new entry journal', run: () => write('entry') },
    { id: 'moment', label: 'Add a moment', icon: 'clock', hint: 'M', keywords: 'note now timeline', run: () => write('moment') },
    { id: 'today', label: 'Today', icon: 'today', hint: 'T', run: () => navigate('/') },
    { id: 'yesterday', label: 'Yesterday', icon: 'calendar', keywords: 'day before', run: () => navigate(`/day/${addDays(today, -1)}`) },
    { id: 'calendar', label: 'Calendar', icon: 'calendar', hint: 'C', keywords: 'month', run: () => navigate('/calendar') },
    { id: 'journal', label: 'Journal', icon: 'book-open', hint: 'J', keywords: 'entries list search', run: () => navigate('/journal') },
    { id: 'kept', label: 'Kept days', icon: 'star', keywords: 'starred pinned', run: () => navigate('/journal?pinned=1') },
    { id: 'year', label: 'Year in review', icon: 'chart', hint: 'Y', run: () => navigate('/year') },
    ...(ai ? [
      { id: 'ask', label: 'Ask your journal', icon: 'sparkles', hint: 'A', keywords: 'ai question', run: () => navigate('/ask') },
      { id: 'stories', label: 'Stories', icon: 'book', keywords: 'ai week month year story', run: () => navigate('/stories') },
    ] : []),
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'sources', label: 'Settings → Sources', icon: 'layers', keywords: 'connect apps immich dawarich backfill', run: () => navigate('/settings/sources') },
    { id: 'ai', label: 'Settings → AI', icon: 'sparkles', keywords: 'openai key model', run: () => navigate('/settings/ai') },
    { id: 'notify', label: 'Settings → Notifications', icon: 'bell', keywords: 'push evening morning', run: () => navigate('/settings/notifications') },
    { id: 'theme', label: theme === 'dark' ? 'Switch to light' : 'Switch to dark', icon: theme === 'dark' ? 'sun' : 'moon', keywords: 'theme appearance', run: () => setThemeMode(theme === 'dark' ? 'light' : 'dark') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => window.dispatchEvent(new CustomEvent('pd:shortcuts')) },
  ]

  return (
    <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
      {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
      <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
        <Sidebar onWrite={write} onNavigate={closeDrawer} onPalette={() => setPalette(true)} />
      </div>
      <div className="main">
        <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} />
        <main className="content"><div className="content-inner"><Outlet /></div></main>
      </div>
      {mobile && <button type="button" className="fab" aria-label="Write today" title="Write today" onClick={() => write('entry')}><Icon name="pen" size={22} strokeWidth={2} /></button>}
      {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
    </div>
  )
}

const SECTION: Record<string, string> = { '': 'Today', calendar: 'Calendar', journal: 'Journal', year: 'Year', ask: 'Ask', stories: 'Stories' }
const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const parts = loc.pathname.split('/')
  const today = todayISO()
  let title: ReactNode = SECTION[parts[1] || ''] || ''
  if (parts[1] === 'day') {
    const raw = parts[2] || ''
    const date = raw === 'yesterday' ? addDays(today, -1) : raw === 'today' ? today : raw
    if (date === today) title = 'Today'
    else if (/^\d{4}-\d{2}-\d{2}$/.test(date)) {
      const [y, m] = date.split('-').map(Number)
      // phones: the month only (the page says which day), still a way back to the calendar
      title = mobile
        ? <button type="button" className="crumb" style={{ color: 'inherit' }} onClick={() => navigate(`/calendar/${date.slice(0, 7)}`)}>{MONTHS[m - 1]} {y}</button>
        : <><button type="button" className="crumb" onClick={() => navigate(`/calendar/${date.slice(0, 7)}`)}>{MONTHS[m - 1]} {y}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{weekday(date, true)}, {fmtDate(date)}</span></>
    }
  } else if (parts[1] === 'calendar' && /^\d{4}-\d{2}$/.test(parts[2] || '')) {
    title = <><button type="button" className="crumb" onClick={() => navigate('/calendar')}>Calendar</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{MONTHS[Number(parts[2].slice(5)) - 1]} {parts[2].slice(0, 4)}</span></>
  } else if (parts[1] === 'year' && /^\d{4}$/.test(parts[2] || '')) {
    title = <><button type="button" className="crumb" onClick={() => navigate('/year')}>Year</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span>{parts[2]}</span></>
  }
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('pd:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">{title}</h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      {mobile && <AskButton />}
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
