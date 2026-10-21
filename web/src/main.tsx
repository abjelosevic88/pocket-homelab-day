import React, { useEffect, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import './styles/base.css'
import './styles/day.css'
import './styles/app.css'
import './styles/settings.css'
import { AppProvider, useApp } from './lib/app-context'
import { Loading, ToastProvider } from './components/ui'
import Layout from './components/Layout'
import { ErrorBoundary } from './components/ErrorBoundary'
import { ShortcutsSheet } from './components/ShortcutsSheet'
import Login from './pages/Login'
import Day from './pages/Day'
import Calendar from './pages/Calendar'
import Journal from './pages/Journal'
import Year from './pages/Year'
import Settings from './pages/Settings'
import Ask from './pages/Ask'
import Stories from './pages/Stories'

function Gate({ children }: { children: React.ReactNode }) {
  const { status, user } = useApp()
  const loc = useLocation()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace state={{ from: loc.pathname + loc.search }} />
  return <>{children}</>
}

/** The keyboard shortcuts sheet, from anywhere (the shell, Settings, the palette). */
function Shortcuts() {
  const [open, setOpen] = useState(false)
  useEffect(() => {
    const h = () => setOpen(true)
    window.addEventListener('pd:shortcuts', h)
    return () => window.removeEventListener('pd:shortcuts', h)
  }, [])
  return open ? <ShortcutsSheet onClose={() => setOpen(false)} /> : null
}

function App() {
  return (
    <>
      <Routes>
        <Route path="/login" element={<Login mode="login" />} />
        <Route path="/setup" element={<Login mode="setup" />} />
        <Route path="/settings/:tab?" element={<Gate><ErrorBoundary where="settings"><Settings /></ErrorBoundary></Gate>} />
        <Route element={<Gate><ErrorBoundary where="day"><Layout /></ErrorBoundary></Gate>}>
          <Route path="/" element={<Day />} />
          <Route path="/day/:date" element={<Day />} />
          <Route path="/calendar" element={<Calendar />} />
          <Route path="/calendar/:month" element={<Calendar />} />
          <Route path="/journal" element={<Journal />} />
          <Route path="/year" element={<Year />} />
          <Route path="/year/:year" element={<Year />} />
          <Route path="/ask" element={<Ask />} />
          <Route path="/stories" element={<Stories />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
      <Shortcuts />
    </>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary where="app"><ToastProvider><AppProvider><App /></AppProvider></ToastProvider></ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>,
)
