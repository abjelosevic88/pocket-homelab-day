// ⌘K / "/" — search your words and moments, jump to a date, or run a command.
import React, { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { addDays, fmtDate, todayISO, weekday } from '../lib/format'
import { MOOD_ICON, type Entry, type Moment } from '../lib/types'
import { Icon } from './icons'
import { useDebounce } from './ui'

export interface PaletteCommand { id: string; label: string; icon: string; hint?: string; keywords?: string; run: () => void }

type Row = { key: string; group: string; icon: React.ReactNode; label: React.ReactNode; sub?: React.ReactNode; hint?: React.ReactNode; run: () => void }

/** "2026-05-14", "today", "yesterday" → an ISO date. */
function asDate(q: string): string {
  const t = q.trim().toLowerCase()
  const today = todayISO()
  if (t === 'today') return today
  if (t === 'yesterday') return addDays(today, -1)
  if (/^\d{4}-\d{2}-\d{2}$/.test(t) && !Number.isNaN(Date.parse(t))) return t
  return ''
}

export function CommandPalette({ onClose, commands }: { onClose: () => void; commands: PaletteCommand[] }) {
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 200)
  const [hi, setHi] = useState(0)
  const [found, setFound] = useState<{ entries: Entry[]; moments: Moment[] }>({ entries: [], moments: [] })
  const list = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (dq.trim().length < 2 || asDate(dq)) { setFound({ entries: [], moments: [] }); return }
    let live = true
    api.get<{ entries: Entry[]; moments: Moment[] }>(`${V1}/search?q=${encodeURIComponent(dq)}`).then(r => { if (live) setFound({ entries: r.entries || [], moments: r.moments || [] }) }).catch(() => {})
    return () => { live = false }
  }, [dq])

  const rows = useMemo<Row[]>(() => {
    const out: Row[] = []
    const n = q.trim().toLowerCase()
    const words = n.split(/\s+/).filter(Boolean)
    const hit = (text: string) => words.every(w => text.toLowerCase().includes(w))
    const date = asDate(q)
    if (date) out.push({ key: 'date', group: 'Go to', icon: <Icon name="calendar" size={16} />, label: `${weekday(date, true)}, ${fmtDate(date)}`, sub: date > todayISO() ? 'not yet' : undefined, run: () => navigate(`/day/${date}`) })
    for (const e of found.entries) out.push({
      key: `e${e.id}`, group: 'Days', icon: e.mood > 0 ? <span className="pal-mood">{MOOD_ICON[e.mood]}</span> : <Icon name="note" size={16} />,
      label: e.highlight || (e.text || '').split('\n')[0] || '(no words)', sub: [e.tags && e.tags.split(',').filter(Boolean).map(t => '#' + t).join(' ')].filter(Boolean).join(' · ') || undefined,
      hint: <span className="pal-date">{fmtDate(e.date)}</span>, run: () => navigate(`/day/${e.date}`),
    })
    for (const m of found.moments) out.push({
      key: `m${m.id}`, group: 'Moments', icon: <Icon name="clock" size={16} />, label: m.text, sub: m.time ? `${m.time} · moment` : 'moment',
      hint: <span className="pal-date">{fmtDate(m.date)}</span>, run: () => navigate(`/day/${m.date}`),
    })
    for (const c of commands) {
      if (n && !hit(`${c.label} ${c.keywords || ''}`)) continue
      out.push({ key: c.id, group: 'Commands', icon: <Icon name={c.icon} size={16} />, label: c.label, hint: c.hint ? <kbd>{c.hint}</kbd> : undefined, run: c.run })
    }
    return out
  }, [q, found, commands, navigate])
  useEffect(() => setHi(0), [q, found])
  useEffect(() => { list.current?.querySelector(`[data-i="${hi}"]`)?.scrollIntoView({ block: 'nearest' }) }, [hi])

  const run = (r: Row | undefined) => { if (!r) return; onClose(); r.run() }
  let last = ''
  return createPortal(
    <div className="palette-backdrop" onMouseDown={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className="palette-box" role="dialog" aria-label="Search and commands">
        <div className="pal-input">
          <Icon name="search" size={18} />
          <input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder="Search your words… or type a date (2026-05-14)" aria-label="Search"
            onKeyDown={e => {
              if (e.key === 'ArrowDown') { e.preventDefault(); setHi(h => Math.min(rows.length - 1, h + 1)) }
              else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(h => Math.max(0, h - 1)) }
              else if (e.key === 'Enter') { e.preventDefault(); run(rows[hi]) }
              else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); onClose() }
            }} />
          <kbd>esc</kbd>
        </div>
        <div className="pal-list" ref={list} role="listbox">
          {rows.length === 0 && <div className="pal-empty">{dq.trim().length >= 2 ? 'Nothing found.' : 'Nothing matches.'}</div>}
          {rows.map((r, i) => {
            const head = r.group !== last ? <div className="pal-group" key={'g' + r.group}>{r.group}</div> : null
            last = r.group
            return (
              <React.Fragment key={r.key}>
                {head}
                <div data-i={i} role="option" aria-selected={i === hi} className={`pal-row ${i === hi ? 'hi' : ''}`} onMouseEnter={() => setHi(i)} onClick={() => run(r)}>
                  <span className="pal-ico">{r.icon}</span>
                  <span className="grow"><span className="pal-label ellipsis">{r.label}</span>{r.sub && <span className="pal-sub ellipsis">{r.sub}</span>}</span>
                  {r.hint}
                  {i === hi && <Icon name="chevron-right" size={14} className="muted" />}
                </div>
              </React.Fragment>
            )
          })}
        </div>
      </div>
    </div>, document.body)
}
