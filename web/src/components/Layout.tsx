import { useEffect, useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { Avatar } from './people'
import { Search } from './Search'

const nav = [
  { to: '/', label: 'Today', ico: '☀', end: true },
  { to: '/calendar', label: 'Calendar', ico: '▦' },
  { to: '/journal', label: 'Journal', ico: '✎' },
  { to: '/year', label: 'Year', ico: '◔' },
  { to: '/settings', label: 'Settings', ico: '⚙' },
]

export default function Layout() {
  const { user, settings, status, theme, toggleTheme, setUser } = useApp()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState(false)
  const navigate = useNavigate()
  const logout = async () => { await api.post(`${V1}/auth/logout`); setUser(null); navigate('/login') }
  const title = settings?.title || 'Pocket Day'
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setSearch(s => !s) } }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [])
  return (
    <div className="app">
      <aside className={`sidebar ${open ? 'open' : ''}`} onClick={() => setOpen(false)}>
        <div className="brand"><span className="logo"><Logo /></span><span className="brand-name">{title}</span></div>
        <nav>
          {nav.map(n => <NavLink key={n.to} to={n.to} end={n.end} className={({ isActive }) => isActive ? 'active' : ''}><span className="ico">{n.ico}</span>{n.label}</NavLink>)}
          <a onClick={e => { e.preventDefault(); setSearch(true) }} href="#search"><span className="ico">⌕</span>Search <kbd className="kbd">⌘K</kbd></a>
        </nav>
        <div className="foot">
          <span className="row" title={user?.email}>{user && <Avatar user={user} />}<span className="foot-id"><span>{user?.name || user?.email}</span>{status?.version && <a className="ver" href="https://github.com/abjelosevic88/pocket-homelab-day/releases" target="_blank" rel="noreferrer" title="Version — click for release notes">v{status.version.replace(/^v/, '')}</a>}</span></span>
          <span className="row">
            <button className="btn ghost sm" onClick={toggleTheme} title="Toggle theme">{theme === 'dark' ? '☀' : '☾'}</button>
            {status?.auth_mode === 'local' && <button className="btn ghost sm" onClick={logout} title="Sign out">⎋</button>}
          </span>
        </div>
      </aside>
      <div className="main">
        <header className="topbar">
          <button className="btn ghost icon menu-btn" onClick={() => setOpen(o => !o)} aria-label="Menu">☰</button>
          <span className="topbar-title">{title}</span>
          <span className="spacer" />
          <button className="btn ghost sm" onClick={() => setSearch(true)} title="Search">⌕</button>
          <button className="btn ghost sm" onClick={toggleTheme} title="Toggle theme">{theme === 'dark' ? '☀' : '☾'}</button>
        </header>
        <main className="content"><Outlet /></main>
        <nav className="tabbar">
          {nav.filter(n => n.to !== '/settings').map(n => <NavLink key={n.to} to={n.to} end={n.end} className={({ isActive }) => isActive ? 'active' : ''}><span className="ico">{n.ico}</span><span>{n.label}</span></NavLink>)}
          <NavLink to="/settings" className={({ isActive }) => isActive ? 'active' : ''}><span className="ico">⚙</span><span>More</span></NavLink>
        </nav>
      </div>
      {search && <Search onClose={() => setSearch(false)} />}
    </div>
  )
}

/** A calendar leaf with a sun. */
export function Logo() {
  return (
    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <rect x="4" y="5" width="16" height="15" rx="2.5" /><path d="M4 10h16" /><path d="M9 3v4M15 3v4" /><circle cx="12" cy="15.5" r="2" fill="currentColor" stroke="none" />
    </svg>
  )
}
