import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { fmtDate, todayISO, weekday } from '../lib/format'
import { ACTIVITY, MOODS, MOOD_COLOR, MOOD_ICON, WEATHER_ICON, fmtHeadlineShort, isActivity, fmtAmount, fmtKm, fmtMinutes, type CalendarDay, type DayPage, type Photos, type YearRecap } from '../lib/types'
import { Loading, useAsync } from '../components/ui'
import { Icon, SourceIcon } from '../components/icons'
import { StoryCard, useAI } from '../components/ai'
import HabitTrends from '../components/HabitTrends'

const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']
type Layer = 'photos' | 'mood' | 'habits' | 'dawarich' | 'maloja' | 'jellystat' | 'firefly' | 'podcasts' | 'pinepods' | 'audiobookshelf' | 'forgejo' | 'nextcloud' | 'tasks' | 'jellyseerr' | 'horoscope'
// icon: a stroke icon name; the source layers use their source's glyph
const LAYERS: { id: Layer; label: string; icon: string }[] = [
  { id: 'photos', label: 'Photos', icon: 'camera' }, { id: 'mood', label: 'Mood', icon: 'smile' }, { id: 'habits', label: 'Habits', icon: 'habit' }, { id: 'horoscope', label: 'Sky', icon: 'moon' }, { id: 'dawarich', label: 'Distance', icon: 'map' },
  { id: 'maloja', label: 'Music', icon: 'music' }, { id: 'podcasts', label: 'Podcasts', icon: 'mic' }, { id: 'pinepods', label: 'PinePods', icon: 'mic' }, { id: 'audiobookshelf', label: 'Audiobooks', icon: 'headphones' },
  { id: 'jellystat', label: 'Watched', icon: 'tv' }, { id: 'firefly', label: 'Spend', icon: 'wallet' }, { id: 'forgejo', label: 'Code', icon: 'code' }, { id: 'nextcloud', label: 'Nextcloud Tasks', icon: 'check-square' }, { id: 'tasks', label: 'Pocket Tasks', icon: 'check-square' }, { id: 'jellyseerr', label: 'Requests', icon: 'film' },
]
const MOON_EMOJI: Record<string, string> = { new: '🌑', waxing_crescent: '🌒', first_quarter: '🌓', waxing_gibbous: '🌔', full: '🌕', waning_gibbous: '🌖', last_quarter: '🌗', waning_crescent: '🌘' }
const MOON_NAME: Record<string, string> = { new: 'New Moon', waxing_crescent: 'Waxing crescent', first_quarter: 'First quarter', waxing_gibbous: 'Waxing gibbous', full: 'Full Moon', waning_gibbous: 'Waning gibbous', last_quarter: 'Last quarter', waning_crescent: 'Waning crescent' }
/** A horoscope score (0–100) as a colour: low warm, high cool-green. */
const skyColor = (v: number) => v >= 65 ? '#16a34a' : v >= 45 ? '#6a3fb5' : v >= 30 ? '#d97706' : '#dc2626'
const ym = (y: number, m: number) => `${y}-${String(m).padStart(2, '0')}`
const daysIn = (y: number, m: number) => new Date(y, m, 0).getDate()

export default function Calendar() {
  const { month: mp } = useParams()
  const navigate = useNavigate()
  const { settings } = useApp()
  const aiOn = useAI()
  const today = todayISO()
  const month = mp && /^\d{4}-\d{2}$/.test(mp) ? mp : today.slice(0, 7)
  const [y, m] = month.split('-').map(Number)
  const { data, loading, error } = useAsync(() => api.get<{ days: CalendarDay[] }>(`${V1}/calendar?month=${month}`), [month])
  const { data: yearData } = useAsync(() => api.get<YearRecap>(`${V1}/years/${y}`), [y])
  const [layer, setLayerRaw] = useState<Layer>(() => (new URLSearchParams(window.location.search).get('layer') as Layer) || (localStorage.getItem('pd-cal-layer') as Layer) || 'photos')
  const setLayer = (l: Layer) => { setLayerRaw(l); localStorage.setItem('pd-cal-layer', l) }
  const [sel, setSel] = useState<string>('')
  const wide = useWide()
  useEffect(() => { setSel(month === today.slice(0, 7) ? today : '') }, [month, today])
  const go = (n: number) => { const d = new Date(y, m - 1 + n, 1); navigate(`/calendar/${ym(d.getFullYear(), d.getMonth() + 1)}`) }
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA')) return
      if (e.key === 'ArrowLeft' && e.shiftKey) go(-1)
      else if (e.key === 'ArrowRight' && e.shiftKey && month < today.slice(0, 7)) go(1)
      else if ((e.key === 'ArrowLeft' || e.key === 'ArrowRight') && sel) {
        const d = new Date(sel); d.setDate(d.getDate() + (e.key === 'ArrowLeft' ? -1 : 1))
        const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
        if (iso.slice(0, 7) === month && iso <= today) setSel(iso)
      } else if (e.key === 'Enter' && sel) navigate(`/day/${sel}`)
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const days = data?.days || []
  const past = days.filter(d => d.date <= today)
  const sum = (k: string) => past.reduce((a, d) => a + (d.headlines?.[k] || 0), 0)
  const written = past.filter(d => d.entry).length
  const moods = past.filter(d => d.mood > 0)
  const mood = moods.length ? moods.reduce((a, d) => a + d.mood, 0) / moods.length : 0
  const available = LAYERS.filter(l => l.id === 'mood' || (l.id === 'photos' ? sum('immich') > 0 : sum(l.id) > 0))
  const active = available.find(l => l.id === layer)?.id || available[0]?.id || 'mood'
  const max = useMemo(() => Math.max(1, ...days.map(d => d.headlines?.[active] || 0)), [days, active])
  const weekStart = settings?.week_start === 'sun' ? 0 : 1
  const lead = (new Date(y, m - 1, 1).getDay() - weekStart + 7) % 7
  const labels = weekStart === 1 ? ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'] : ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
  const unit = settings?.distance_unit || 'km'
  const cur = settings?.home_currency || 'EUR'
  const isCurrent = month === today.slice(0, 7)

  return (
    <div className="cal2">
      <div className="cal2-head">
        <div>
          <h1 className="cal2-title">{MONTHS[m - 1]} <span>{y}</span></h1>
          <div className="cal2-sub">{past.length ? `${written} of ${past.length} days written` : 'Not started yet'}{isCurrent ? '' : ` · ${daysIn(y, m)} days`}</div>
        </div>
        <div className="cal2-nav">
          <button type="button" className="btn icon-only" onClick={() => go(-1)} title="Previous month (Shift ←)" aria-label="Previous month"><Icon name="chevron-left" size={18} /></button>
          <Link className={`btn ${isCurrent ? 'primary' : ''}`} to="/calendar">Today</Link>
          <button type="button" className="btn icon-only" onClick={() => go(1)} disabled={month >= today.slice(0, 7)} title="Next month (Shift →)" aria-label="Next month"><Icon name="chevron-right" size={18} /></button>
        </div>
      </div>

      <div className="month-strip">
        <Link className="ms-year" to={`/year/${y}`} title={`${y} in review`}>{y}</Link>
        {MONTHS.map((name, i) => {
          const key = ym(y, i + 1)
          const r = yearData?.months?.[i]
          const total = key === today.slice(0, 7) ? Number(today.slice(8)) : daysIn(y, i + 1)
          const pct = r && total ? Math.min(100, r.entries / total * 100) : 0
          const future = key > today.slice(0, 7)
          return <Link key={key} to={future ? '#' : `/calendar/${key}`} onClick={e => future && e.preventDefault()} className={`ms ${key === month ? 'on' : ''} ${future ? 'future' : ''}`} title={r ? `${r.entries} days written${r.mood ? ` · mood ${r.mood}` : ''}` : name}>
            <span>{name.slice(0, 3)}</span><i><b style={{ width: `${pct}%`, background: r?.mood ? MOOD_COLOR[Math.round(r.mood)] : undefined }} /></i>
          </Link>
        })}
      </div>

      <div className="cal2-stats">
        <div className="tile ring-tile"><Ring value={past.length ? written / past.length : 0} /><div><b>{written}<span>/{past.length}</span></b><small>days written</small></div></div>
        {mood > 0 && <div className="tile"><span className="ti">{MOOD_ICON[Math.round(mood)]}</span><div><b>{mood.toFixed(1)}</b><small>average mood</small></div></div>}
        {sum('immich') > 0 && <div className="tile"><SourceIcon source="immich" size={36} /><div><b>{Math.round(sum('immich'))}</b><small>photos</small></div></div>}
        {sum('dawarich') > 0 && <div className="tile"><SourceIcon source="dawarich" size={36} /><div><b>{fmtKm(sum('dawarich'), unit).replace(/\.\d /, ' ')}</b><small>travelled</small></div></div>}
        {sum('maloja') > 0 && <div className="tile"><SourceIcon source="maloja" size={36} /><div><b>{Math.round(sum('maloja'))}</b><small>songs</small></div></div>}
        {sum('jellystat') > 0 && <div className="tile"><SourceIcon source="jellystat" size={36} /><div><b>{Math.round(sum('jellystat') / 60)} h</b><small>watched</small></div></div>}
        {sum('firefly') > 0 && <div className="tile"><SourceIcon source="firefly" size={36} /><div><b>{fmtAmount(Math.round(sum('firefly')), cur).replace(/[.,]00$/, '')}</b><small>spent</small></div></div>}
      </div>

      <div className="cal2-toolbar">
        <div className="layer-switch" role="radiogroup" aria-label="Colour the days by">
          {available.map(l => <button type="button" key={l.id} role="radio" aria-checked={active === l.id} className={active === l.id ? 'on' : ''} onClick={() => setLayer(l.id)}><Icon name={l.icon} size={14} />{l.label}</button>)}
        </div>
        <span className="muted small hide-sm">Click a day to preview · Enter opens it · Shift ← → months</span>
      </div>

      {loading && !data && <Loading />}
      {error && <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>}
      {data && <div className={`cal2-body ${wide && sel ? 'with-panel' : ''}`}>
        <div className="cal2-grid-wrap">
          <div className="cal2-wd">{labels.map(l => <div key={l}>{l}</div>)}</div>
          <div className="cal2-grid">
            {Array.from({ length: lead }).map((_, i) => <div key={'l' + i} className="c2 blank" />)}
            {days.map(d => <Cell key={d.date} d={d} layer={active} max={max} today={today} selected={d.date === sel} unit={unit} cur={cur}
              onClick={() => { if (d.date > today) return; if (!wide) navigate(`/day/${d.date}`); else if (sel === d.date) navigate(`/day/${d.date}`); else setSel(d.date) }} />)}
          </div>
          <Legend layer={active} max={max} unit={unit} cur={cur} />
        </div>
        {wide && sel && <Preview date={sel} onClose={() => setSel('')} />}
      </div>}

      {month <= today.slice(0, 7) && <HabitTrends month={month} today={today} />}

      {aiOn && month <= today.slice(0, 7) && <div className="mt"><StoryCard key={month} kind="month" period={month} label={`${MONTHS[m - 1]} ${y}`} /></div>}
    </div>
  )
}

function useWide() {
  const [w, setW] = useState(() => window.matchMedia('(min-width: 1100px)').matches)
  useEffect(() => {
    const q = window.matchMedia('(min-width: 1100px)')
    const h = () => setW(q.matches)
    q.addEventListener('change', h)
    return () => q.removeEventListener('change', h)
  }, [])
  return w
}

export function Ring({ value }: { value: number }) {
  const r = 16, c = 2 * Math.PI * r
  return <svg className="ring" viewBox="0 0 40 40" width="40" height="40" aria-hidden><circle cx="20" cy="20" r={r} className="ring-bg" /><circle cx="20" cy="20" r={r} className="ring-fg" strokeDasharray={`${c * value} ${c}`} transform="rotate(-90 20 20)" /></svg>
}

function layerValue(d: CalendarDay, layer: Layer, unit: 'km' | 'mi', cur: string, long = false): string {
  const v = d.headlines?.[layer] || 0
  if (!v) return ''
  switch (layer) {
    case 'dawarich': return fmtKm(v, unit).replace('.0 ', ' ')
    case 'maloja': return `${Math.round(v)} ♪`
    case 'jellystat': return fmtMinutes(Math.round(v)).replace(' min', 'm').replace(' h ', 'h')
    case 'firefly': return fmtAmount(Math.round(v), cur).replace(/[.,]00$/, '')
    case 'habits': return `${Math.round(v)}%`
    case 'horoscope': return long ? `${Math.round(v)}/100` : String(Math.round(v))
  }
  if (isActivity(layer)) {
    const m = ACTIVITY[layer]
    if (m.unit === 'count' || m.unit === 'qty') return long ? `${Math.round(v)} ${Math.round(v) === 1 ? (m.unit === 'qty' ? m.qtyOne : m.one) : (m.unit === 'qty' ? m.qtyMany : m.many)}` : String(Math.round(v))
    return fmtHeadlineShort(layer, v, cur)
  }
  return ''
}

function Cell({ d, layer, max, today, selected, unit, cur, onClick }: { d: CalendarDay; layer: Layer; max: number; today: string; selected: boolean; unit: 'km' | 'mi'; cur: string; onClick: () => void }) {
  const future = d.date > today
  const n = Number(d.date.slice(8))
  let style: React.CSSProperties = {}
  let photo = false
  if (!future) {
    if (layer === 'photos' && d.cover) {
      photo = true
      style = d.cover.startsWith('demo-') ? { backgroundImage: `linear-gradient(135deg, hsl(${(n * 37) % 360} 45% 35%), hsl(${(n * 37 + 60) % 360} 45% 22%))` } : { backgroundImage: `url(${V1}/photos/${d.cover}/thumb)` }
    } else if (layer === 'mood' && d.mood) {
      style = { background: `color-mix(in srgb, ${MOOD_COLOR[d.mood]} 32%, var(--surface))` }
    } else if (layer === 'horoscope' && d.headlines?.horoscope) {
      const v = d.headlines.horoscope
      style = { background: `color-mix(in srgb, ${skyColor(v)} ${Math.round(10 + 26 * Math.abs(v - 50) / 50)}%, var(--surface))` }
    } else if (layer !== 'photos' && layer !== 'mood' && d.headlines?.[layer]) {
      const pct = Math.round(12 + 58 * Math.min(1, (d.headlines[layer] || 0) / max))
      style = { background: `color-mix(in srgb, var(--accent) ${pct}%, var(--surface))` }
    }
  }
  const val = future ? '' : layerValue(d, layer, unit, cur)
  return (
    <button type="button" className={`c2 ${photo ? 'photo' : ''} ${future ? 'future' : ''} ${d.date === today ? 'today' : ''} ${selected ? 'sel' : ''} ${d.entry ? 'written' : ''}`} style={style} onClick={onClick} disabled={future} title={d.summary || undefined}>
      <span className="c2-top">
        <span className="c2-n">{n}</span>
        {d.entry && <span className="c2-dot" title="Written" />}
        <span className="spacer" />
        {d.pinned && <span title="Kept" style={{ display: 'inline-grid', color: photo ? '#fde68a' : 'var(--star)' }}><Icon name="star" size={12} strokeWidth={2.2} /></span>}
        {d.mood > 0 && <span className="c2-mood" title={MOODS[d.mood]}>{MOOD_ICON[d.mood]}</span>}
        {!future && layer === 'horoscope' && d.moon && <span className="c2-w" title={`${MOON_NAME[d.moon] || d.moon}${d.moon_sign ? ` in ${d.moon_sign[0].toUpperCase()}${d.moon_sign.slice(1)}` : ''}`}>{MOON_EMOJI[d.moon]}</span>}
        {!future && layer !== 'horoscope' && d.weather && <span className="c2-w" title={d.t_max != null ? `${Math.round(d.t_max)} °C` : d.weather}>{WEATHER_ICON[d.weather]}</span>}
      </span>
      {d.highlight ? <span className="c2-hl">{d.highlight}</span> : layer === 'horoscope' && d.sky && !future ? <span className="c2-hl c2-sky">{d.sky}</span> : null}
      <span className="c2-foot">{d.moments > 0 && !val && <span className="c2-mom">{d.moments} moment{d.moments === 1 ? '' : 's'}</span>}<span className="spacer" />{val && <span className="c2-val">{val}</span>}</span>
    </button>
  )
}

function Legend({ layer, max, unit, cur }: { layer: Layer; max: number; unit: 'km' | 'mi'; cur: string }) {
  if (layer === 'photos') return <p className="cal2-legend">Each day shows its favourite photo, or one from the middle of the day.</p>
  if (layer === 'horoscope') return <p className="cal2-legend"><span><i style={{ background: '#dc2626' }} />tough</span><span><i style={{ background: '#d97706' }} />mixed</span><span><i style={{ background: '#6a3fb5' }} />steady</span><span><i style={{ background: '#16a34a' }} />good</span><span className="muted">· your day's horoscope score, the Moon's phase in the corner (Pocket Horoscope)</span></p>
  if (layer === 'mood') return <p className="cal2-legend">{[1, 2, 3, 4, 5].map(i => <span key={i}><i style={{ background: MOOD_COLOR[i] }} />{MOODS[i]}</span>)}</p>
  const fake: CalendarDay = { date: '', entry: false, mood: 0, highlight: '', pinned: false, moments: 0, headlines: { [layer]: max } }
  return <p className="cal2-legend"><span className="grad" /> less → more, up to {layerValue(fake, layer, unit, cur, true)} a day</p>
}

/** The selected day at a glance, without leaving the month. */
function Preview({ date, onClose }: { date: string; onClose: () => void }) {
  const { data, loading } = useAsync(() => api.get<DayPage>(`${V1}/days/${date}`), [date])
  const photos = (data?.facts?.immich?.payload as Photos | null)?.items?.filter(p => p.type === 'IMAGE').slice(0, 6) || []
  const order = ['travel', 'dawarich', 'maloja', 'podcasts', 'pinepods', 'audiobookshelf', 'jellystat', 'jellyseerr', 'habits', 'health', 'still', 'horoscope', 'nextcloud', 'tasks', 'birthdays', 'projects', 'forgejo', 'firefly', 'split', 'invoicing', 'documents', 'bookshelf', 'kitchen', 'inventory', 'garage', 'weather']
  return (
    <aside className="cal2-panel">
      <div className="row"><div className="grow"><div className="muted small">{weekday(date, true)}</div><h2>{fmtDate(date)}</h2></div><button type="button" className="btn ghost icon-only" onClick={onClose} aria-label="Close" title="Close"><Icon name="x" size={17} /></button></div>
      {loading && !data && <Loading />}
      {data && <>
        {data.entry ? <div className="pv-entry">
          <div className="row" style={{ gap: 8 }}>{data.entry.mood > 0 && <span className="pv-mood" style={{ background: `color-mix(in srgb, ${MOOD_COLOR[data.entry.mood]} 25%, transparent)` }}>{MOOD_ICON[data.entry.mood]} {MOODS[data.entry.mood]}</span>}{data.entry.pinned && <span className="badge"><Icon name="star" size={11} />kept</span>}{data.entry.origin === 'habits' && <span className="badge" title="Taken from the Pocket Habits journal"><Icon name="check-circle" size={11} />from Habits</span>}</div>
          {data.entry.highlight && <h3>{data.entry.highlight}</h3>}
          {data.entry.text && <p className="pv-text">{data.entry.text}</p>}
        </div> : <p className="muted">Nothing written for this day yet.</p>}
        {photos.length > 0 && <div className="pv-photos">{photos.map(p => p.id.startsWith('demo-') ? <span key={p.id} className="pv-ph demo" /> : <img key={p.id} className="pv-ph" src={`${V1}/photos/${p.id}/thumb`} alt="" loading="lazy" />)}</div>}
        <div className="pv-facts">{order.filter(s => data.facts[s]?.summary).map(s => <div key={s} className="pv-fact"><SourceIcon source={s} size={22} /><span>{data.facts[s].summary}</span></div>)}</div>
        {data.moments.length > 0 && <div className="pv-moments">{data.moments.slice(0, 4).map(mo => <div key={mo.id}><span className="muted mono small">{mo.time}</span> {mo.text}</div>)}{data.moments.length > 4 && <div className="muted small">+{data.moments.length - 4} more</div>}</div>}
        <Link className="btn primary block" to={`/day/${date}`}>{data.entry ? 'Open the day' : 'Write about it'}<Icon name="chevron-right" size={15} /></Link>
      </>}
    </aside>
  )
}
