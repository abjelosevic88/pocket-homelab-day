import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { fmtDate } from '../lib/format'
import { MOOD_ICON, type Entry, type Moment } from '../lib/types'
import { useDebounce } from './ui'

export function Search({ onClose }: { onClose: () => void }) {
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 200)
  const [res, setRes] = useState<{ entries: Entry[]; moments: Moment[] }>({ entries: [], moments: [] })
  const [hl, setHl] = useState(0)
  const input = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  useEffect(() => { input.current?.focus() }, [])
  useEffect(() => {
    if (dq.trim().length < 2) { setRes({ entries: [], moments: [] }); return }
    api.get<{ entries: Entry[]; moments: Moment[] }>(`${V1}/search?q=${encodeURIComponent(dq)}`).then(r => { setRes(r); setHl(0) }).catch(() => {})
  }, [dq])
  const rows = [...res.entries.map(e => ({ date: e.date, title: e.highlight || e.text.split('\n')[0] || '(no words)', sub: e.tags, mood: e.mood })), ...res.moments.map(m => ({ date: m.date, title: m.text, sub: `${m.time} · moment`, mood: 0 }))]
  const go = (d: string) => { navigate(`/day/${d}`); onClose() }
  const key = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') onClose()
    if (e.key === 'ArrowDown') { e.preventDefault(); setHl(h => Math.min(rows.length - 1, h + 1)) }
    if (e.key === 'ArrowUp') { e.preventDefault(); setHl(h => Math.max(0, h - 1)) }
    if (e.key === 'Enter') {
      if (/^\d{4}-\d{2}-\d{2}$/.test(q.trim())) { go(q.trim()); return }
      if (rows[hl]) go(rows[hl].date)
    }
  }
  return (
    <div className="modal-backdrop" onMouseDown={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className="search-box" role="dialog" aria-modal="true">
        <input ref={input} value={q} onChange={e => setQ(e.target.value)} onKeyDown={key} placeholder="Search your words… or type a date (2026-05-14)" />
        <div className="search-results">
          {rows.length === 0 && dq.trim().length >= 2 && <div className="muted pad">Nothing found.</div>}
          {rows.map((r, i) => <button key={r.date + r.title + i} className={`search-row ${i === hl ? 'active' : ''}`} onMouseEnter={() => setHl(i)} onClick={() => go(r.date)}>
            <span className="muted mono small" style={{ minWidth: 92 }}>{fmtDate(r.date)}</span>
            <span className="grow" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', textAlign: 'left' }}>{r.title}</span>
            {r.mood > 0 && <span>{MOOD_ICON[r.mood]}</span>}
            {r.sub && <span className="muted small">{r.sub}</span>}
          </button>)}
        </div>
        <div className="muted small" style={{ padding: '6px 12px' }}>↑↓ move · Enter open · Esc close</div>
      </div>
    </div>
  )
}
