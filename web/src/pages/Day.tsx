import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { addDays, fmtDate, todayISO, weekday } from '../lib/format'
import { KIND_ICON, MOODS, MOOD_ICON, SOURCE_ICON, tagList, type DayPage, type Entry, type Moment, type Pin, type SourceName } from '../lib/types'
import { Card, Confirm, Loading, Segmented, useAsync, useToast } from '../components/ui'
import { FactCard, WeatherChip, type PinFn } from '../components/facts'

const ORDER: SourceName[] = ['travel', 'dawarich', 'immich', 'maloja', 'jellystat', 'habits', 'bookshelf', 'kitchen', 'firefly', 'garage']

export default function Day() {
  const params = useParams()
  const navigate = useNavigate()
  const { settings, user } = useApp()
  const { toast } = useToast()
  const today = todayISO()
  let date = params.date || today
  if (date === 'today') date = today
  if (date === 'yesterday') date = addDays(today, -1)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) date = today
  const { data, setData, error, loading, reload } = useAsync(() => api.get<DayPage>(`${V1}/days/${date}`), [date])
  const [refreshing, setRefreshing] = useState(false)
  const [confirmDel, setConfirmDel] = useState(false)
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return
      if (e.key === 'ArrowLeft') navigate(`/day/${addDays(date, -1)}`)
      if (e.key === 'ArrowRight' && date < today) navigate(`/day/${addDays(date, 1)}`)
      if (e.key === 't') navigate('/')
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [date, navigate, today])

  const refresh = async () => {
    setRefreshing(true)
    try {
      const r = await api.post<{ ok: boolean; errors: Record<string, string> }>(`${V1}/days/${date}/refresh`)
      const bad = Object.entries(r.errors || {})
      toast(bad.length ? `Refreshed; ${bad.map(([k, v]) => `${k}: ${v}`).join(', ')}` : 'Refreshed', bad.length ? 'err' : 'ok')
      await reload()
    } catch (e) { toast((e as Error).message, 'err') } finally { setRefreshing(false) }
  }
  const onPin: PinFn = async p => {
    try { const r = await api.post<{ pins: Pin[] }>(`${V1}/days/${date}/pins`, p); setData(d => d ? { ...d, pins: r.pins } : d) } catch (e) { toast((e as Error).message, 'err') }
  }
  const onUnpin = async (id: number) => {
    try { await api.del(`${V1}/pins/${id}`); setData(d => d ? { ...d, pins: d.pins.filter(p => p.id !== id) } : d) } catch (e) { toast((e as Error).message, 'err') }
  }
  const deleteEntry = async () => {
    setConfirmDel(false)
    try { await api.del(`${V1}/days/${date}`); toast('Entry removed'); reload() } catch (e) { toast((e as Error).message, 'err') }
  }

  if (loading && !data) return <Loading />
  if (error) return <div className="callout danger">{error}</div>
  if (!data) return null
  const isToday = data.today
  const first = (user?.name || '').split(' ')[0]
  const hour = new Date().getHours()
  const greet = hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening'
  const unit = settings?.distance_unit || 'km'
  const have = new Set([...data.sources, ...Object.keys(data.facts)])
  const cards = ORDER.filter(s => have.has(s) && s !== 'weather')
  const trip = data.facts.travel?.payload as { trips?: { name: string; day_no: number; days: number }[] } | undefined
  return (
    <>
      <div className="day-head">
        <div className="day-nav">
          <Link className="btn ghost icon" to={`/day/${addDays(date, -1)}`} title="Previous day (←)">‹</Link>
          <div className="day-title">
            <h1>{isToday ? `${greet}${first ? ', ' + first : ''}` : weekday(date, true)}</h1>
            <div className="sub row wrap" style={{ gap: 8 }}>
              <span>{isToday ? `${weekday(date, true)}, ${fmtDate(date)}` : fmtDate(date)}</span>
              {!isToday && <Link className="status-pill" to="/">Today</Link>}
              <WeatherChip fact={data.facts.weather} />
              {trip?.trips?.[0] && <span className="status-pill">✈️ {trip.trips[0].name} · day {trip.trips[0].day_no}/{trip.trips[0].days}</span>}
              {data.streak > 0 && isToday && <span className="status-pill" title="Days in a row with an entry">🔥 {data.streak} day streak</span>}
            </div>
          </div>
          {date < today ? <Link className="btn ghost icon" to={`/day/${addDays(date, 1)}`} title="Next day (→)">›</Link> : <span className="btn ghost icon" style={{ visibility: 'hidden' }}>›</span>}
        </div>
        <div className="actions row">
          <input type="date" className="date-jump" value={date} max={today} onChange={e => e.target.value && navigate(`/day/${e.target.value}`)} aria-label="Jump to a date" />
          {data.sources.length > 0 && <button className="btn sm" onClick={refresh} disabled={refreshing || data.future} title="Pull the day again from every connected app">{refreshing ? 'Pulling…' : '↻ Pull again'}</button>}
        </div>
      </div>

      {data.future ? <Card><p className="muted">That day has not happened yet.</p></Card> : (
        <div className="day-grid">
          <div className="day-main">
            <EntryEditor date={date} entry={data.entry} prompt={isToday ? data.prompt : 'What happened on this day?'} onSaved={e => setData(d => d ? { ...d, entry: e } : d)} onDelete={() => setConfirmDel(true)} />
            <Moments date={date} moments={data.moments} isToday={isToday} onChange={ms => setData(d => d ? { ...d, moments: ms } : d)} />
            {data.pins.length > 0 && <Card title="Kept from this day" className="mb">
              <div className="pins">{data.pins.map(p => <div key={p.id} className="pin">{p.image ? <img src={p.image} alt="" /> : <span className="pin-ico">{SOURCE_ICON[p.source] || '★'}</span>}<div className="grow"><b>{p.title}</b>{p.subtitle && <div className="muted small">{p.subtitle}</div>}</div><button className="btn ghost sm" onClick={() => onUnpin(p.id)} title="Unpin">✕</button></div>)}</div>
            </Card>}
            {data.memories.length > 0 && <Card title="On this day" className="mb">
              <div className="inline-list">{data.memories.map(m => <Link key={m.id} className="li" to={`/day/${m.date}`} style={{ color: 'inherit' }}><span className="badge">{m.date.slice(0, 4)}</span><span className="grow"><b>{m.highlight || firstLine(m.text)}</b>{m.highlight && m.text && <div className="muted small clamp2">{m.text}</div>}</span>{m.mood > 0 && <span title={MOODS[m.mood]}>{MOOD_ICON[m.mood]}</span>}</Link>)}</div>
            </Card>}
            <div className="row muted small" style={{ justifyContent: 'space-between' }}>
              <span>{data.prev_entry ? <Link to={`/day/${data.prev_entry}`}>‹ {fmtDate(data.prev_entry)}</Link> : ''}</span>
              <span>{data.next_entry ? <Link to={`/day/${data.next_entry}`}>{fmtDate(data.next_entry)} ›</Link> : ''}</span>
            </div>
          </div>
          <div className="day-side">
            {cards.length === 0 && <Card title="Nothing connected yet"><p className="muted">Connect Dawarich, Immich, Maloja, Jellystat, Firefly III and the other Pocket apps under <Link to="/settings?tab=sources">Settings → Sources</Link>, and every day fills itself in.</p></Card>}
            {cards.map(s => <FactCard key={s} source={s} fact={data.facts[s]} link={data.links[s]} unit={unit} pins={data.pins} onPin={onPin} onUnpin={onUnpin} date={date} />)}
          </div>
        </div>
      )}
      {confirmDel && <Confirm title="Remove this day's entry?" message="The words, mood and tags go; moments and pins stay." onCancel={() => setConfirmDel(false)} onConfirm={deleteEntry} />}
    </>
  )
}

function firstLine(t: string) { return (t || '').split('\n')[0] || '(no words)' }

/** The user's words: highlight, text, mood, tags. Autosaves a second after typing stops. */
function EntryEditor({ date, entry, prompt, onSaved, onDelete }: { date: string; entry: Entry | null; prompt: string; onSaved: (e: Entry | null) => void; onDelete: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ highlight: entry?.highlight || '', text: entry?.text || '', mood: entry?.mood || 0, tags: entry?.tags || '', pinned: entry?.pinned || false })
  const [state, setState] = useState<'idle' | 'dirty' | 'saving' | 'saved'>('idle')
  const timer = useRef<number | null>(null)
  const latest = useRef(f)
  latest.current = f
  useEffect(() => { setF({ highlight: entry?.highlight || '', text: entry?.text || '', mood: entry?.mood || 0, tags: entry?.tags || '', pinned: entry?.pinned || false }); setState('idle') }, [date]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = useCallback(async () => {
    setState('saving')
    try {
      const r = await api.put<{ entry: Entry | null }>(`${V1}/days/${date}`, latest.current)
      onSaved(r.entry)
      setState('saved')
    } catch (e) { toast((e as Error).message, 'err'); setState('dirty') }
  }, [date, onSaved, toast])
  const change = (patch: Partial<typeof f>) => {
    setF(x => ({ ...x, ...patch })); setState('dirty')
    if (timer.current) window.clearTimeout(timer.current)
    timer.current = window.setTimeout(save, 900)
  }
  useEffect(() => () => { if (timer.current) window.clearTimeout(timer.current) }, [])
  const words = f.text.trim() ? f.text.trim().split(/\s+/).length : 0
  return (
    <Card className="mb entry-card" title={<span className="muted" style={{ fontWeight: 500 }}>{prompt}</span>} actions={<span className={`save-state ${state}`}>{state === 'saving' ? 'Saving…' : state === 'saved' ? 'Saved' : state === 'dirty' ? 'Unsaved' : ''}</span>}>
      <input className="highlight-input" value={f.highlight} onChange={e => change({ highlight: e.target.value })} placeholder="The one line for this day" maxLength={200} />
      <textarea className="entry-text" value={f.text} onChange={e => change({ text: e.target.value })} placeholder="And the rest, if you like…" rows={Math.max(4, Math.min(24, f.text.split('\n').length + 1))} />
      <div className="entry-foot">
        <Segmented size="sm" value={f.mood} onChange={v => change({ mood: v === f.mood ? 0 : v })} options={[1, 2, 3, 4, 5].map(m => ({ value: m, label: MOOD_ICON[m], title: MOODS[m] }))} />
        <input className="tags-input" value={f.tags} onChange={e => change({ tags: e.target.value })} placeholder="tags, comma, separated" />
        <button className={`btn ghost sm ${f.pinned ? 'on' : ''}`} onClick={() => change({ pinned: !f.pinned })} title="Mark as a day worth coming back to">{f.pinned ? '★ Kept' : '☆ Keep'}</button>
        <span className="spacer" />
        {words > 0 && <span className="muted small">{words} words</span>}
        {entry && <button className="btn ghost sm danger-text" onClick={onDelete}>Remove</button>}
      </div>
      {f.tags && <div className="chips mt-sm">{tagList(f.tags).map(t => <Link key={t} className="chip" to={`/journal?tag=${encodeURIComponent(t)}`}>#{t}</Link>)}</div>}
    </Card>
  )
}

/** Timestamped lines through the day. */
function Moments({ date, moments, isToday, onChange }: { date: string; moments: Moment[]; isToday: boolean; onChange: (m: Moment[]) => void }) {
  const { toast } = useToast()
  const [text, setText] = useState('')
  const [kind, setKind] = useState('note')
  const [time, setTime] = useState('')
  const add = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!text.trim()) return
    try {
      const body: Partial<Moment> = { date, text, kind }
      if (time) body.time = time
      else if (!isToday) body.time = ''
      const m = await api.post<Moment>(`${V1}/moments`, body)
      onChange([...moments, m].sort((a, b) => (a.time || '99').localeCompare(b.time || '99') || a.id - b.id))
      setText(''); setTime('')
    } catch (e) { toast((e as Error).message, 'err') }
  }
  const del = async (id: number) => { try { await api.del(`${V1}/moments/${id}`); onChange(moments.filter(m => m.id !== id)) } catch (e) { toast((e as Error).message, 'err') } }
  const edit = async (m: Moment, text: string) => {
    if (!text.trim() || text === m.text) return
    try { const r = await api.put<Moment>(`${V1}/moments/${m.id}`, { text }); onChange(moments.map(x => x.id === m.id ? r : x)) } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <Card title="Moments" className="mb" actions={<span className="muted small">{moments.length ? `${moments.length}` : ''}</span>}>
      {moments.length > 0 && <div className="moments">{moments.map(m => <div key={m.id} className="moment">
        <span className="muted mono small">{m.time || '—'}</span>
        <span className="kind" title={m.kind}>{KIND_ICON[m.kind] || '•'}</span>
        <span className="grow" contentEditable suppressContentEditableWarning onBlur={e => edit(m, e.currentTarget.textContent || '')} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); (e.target as HTMLElement).blur() } }}>{m.text}</span>
        <button className="btn ghost sm" onClick={() => del(m.id)} title="Delete">✕</button>
      </div>)}</div>}
      <form onSubmit={add} className="moment-add">
        <Segmented size="sm" value={kind} onChange={setKind} options={Object.entries(KIND_ICON).map(([k, ico]) => ({ value: k, label: ico, title: k }))} />
        <input value={text} onChange={e => setText(e.target.value)} placeholder={isToday ? 'What just happened?' : 'Something from that day…'} />
        <input type="time" value={time} onChange={e => setTime(e.target.value)} title="Time (optional)" className="time-input" />
        <button className="btn primary sm" disabled={!text.trim()}>Add</button>
      </form>
    </Card>
  )
}
