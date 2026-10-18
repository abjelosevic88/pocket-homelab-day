import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { fmtDate, todayISO } from '../lib/format'
import { ACTIVITY, MOODS, MOOD_COLOR, MOOD_ICON, SOURCE_ICON, SOURCE_LABEL, WEATHER_ICON, cap, fmtHeadline, fmtHeadlineShort, fmtKm, fmtMinutes, isActivity, type CalendarDay, type Pattern, type YearRecap } from '../lib/types'

const ACT_ORDER = ['pinepods', 'audiobookshelf', 'forgejo', 'nextcloud', 'jellyseerr', 'paperless', 'invoicing', 'split', 'inventory']
import { Card, Empty, Loading, useAsync } from '../components/ui'
import { StoryCard, useAI } from '../components/ai'
import { Ring } from './Calendar'

const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
type Metric = 'written' | 'mood' | 'immich' | 'dawarich' | 'maloja' | 'jellystat' | 'firefly' | 'pinepods' | 'audiobookshelf' | 'forgejo' | 'nextcloud' | 'jellyseerr' | 'paperless'
const METRICS: { id: Metric; label: string; icon: string }[] = [
  { id: 'immich', label: 'Photos', icon: '📷' }, { id: 'dawarich', label: 'Distance', icon: '🗺' }, { id: 'maloja', label: 'Music', icon: '🎧' },
  { id: 'pinepods', label: 'Podcasts', icon: '🎙' }, { id: 'audiobookshelf', label: 'Audiobooks', icon: '📖' },
  { id: 'jellystat', label: 'Watched', icon: '📺' }, { id: 'firefly', label: 'Spend', icon: '💸' }, { id: 'forgejo', label: 'Code', icon: '⌨️' }, { id: 'nextcloud', label: 'Tasks', icon: '☑️' },
  { id: 'jellyseerr', label: 'Requests', icon: '🎬' }, { id: 'paperless', label: 'Documents', icon: '🗂' }, { id: 'mood', label: 'Mood', icon: '🙂' }, { id: 'written', label: 'Written', icon: '✎' },
]
const compact = (n: number) => n >= 10000 ? new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(n) : Math.round(n).toLocaleString()
function money(v: number, cur: string, short = true) {
  try { return new Intl.NumberFormat(undefined, { style: 'currency', currency: cur, notation: short && v >= 10000 ? 'compact' : 'standard', maximumFractionDigits: short ? (v >= 10000 ? 1 : 0) : 2 }).format(v) } catch { return `${compact(v)} ${cur}` }
}

export default function Year() {
  const { year: yp } = useParams()
  const navigate = useNavigate()
  const { settings } = useApp()
  const aiOn = useAI()
  const today = todayISO()
  const thisYear = Number(today.slice(0, 4))
  const year = yp && /^\d{4}$/.test(yp) ? Number(yp) : thisYear
  const { data, loading, error } = useAsync(() => api.get<YearRecap>(`${V1}/years/${year}`), [year])
  const { data: cal } = useAsync(() => api.get<{ days: CalendarDay[] }>(`${V1}/calendar?from=${year}-01-01&to=${year}-12-31`), [year])
  const { data: years } = useAsync(() => api.get<{ year: number; entries: number }[]>(`${V1}/years`), [])
  const { data: patterns } = useAsync(() => api.get<Pattern[]>(`${V1}/patterns?from=${year}-01-01&to=${year}-12-31`), [year])
  const [metric, setMetricRaw] = useState<Metric>(() => (localStorage.getItem('pd-year-metric') as Metric) || 'immich')
  const setMetric = (m: Metric) => { setMetricRaw(m); localStorage.setItem('pd-year-metric', m) }
  const unit = settings?.distance_unit || 'km'
  const days = cal?.days || []
  const available = METRICS.filter(m => m.id === 'written' || m.id === 'mood' ? days.some(d => m.id === 'written' ? d.entry : d.mood > 0) : days.some(d => (d.headlines?.[m.id] || 0) > 0))
  const active = available.find(m => m.id === metric)?.id || available[0]?.id || 'written'

  if (loading && !data) return <Loading />
  if (error) return <div className="callout danger">{error}</div>
  if (!data) return null
  const d = data
  const cur = d.spend_currency || settings?.home_currency || 'EUR'
  const spendMain = d.spend[cur] || 0
  const others = Object.entries(d.spend).filter(([c]) => c !== cur)
  const moodsTotal = d.moods.slice(1).reduce((a, b) => a + b, 0)
  const empty = d.entries === 0 && d.days_with_facts === 0
  const lived = year === thisYear ? Math.round((Date.parse(today) - Date.parse(`${year}-01-01`)) / 86400000) + 1 : 365
  const yearList = (years || []).map(y => y.year)

  return (
    <div className="yr">
      <div className="cal2-head">
        <div>
          <h1 className="cal2-title">{year} <span>in review</span></h1>
          <div className="cal2-sub">{empty ? 'Nothing recorded yet.' : `${d.days_with_facts} days recorded · ${d.entries} written${d.longest_streak > 1 ? ` · longest streak ${d.longest_streak} days` : ''}`}</div>
        </div>
        <div className="cal2-nav">
          <button className="btn icon" onClick={() => navigate(`/year/${year - 1}`)} title={String(year - 1)}>‹</button>
          {yearList.length > 1 ? yearList.slice().sort().map(y => <Link key={y} className={`btn ${y === year ? 'primary' : ''}`} to={`/year/${y}`}>{y}</Link>) : <span className="btn primary">{year}</span>}
          <button className="btn icon" onClick={() => navigate(`/year/${year + 1}`)} disabled={year >= thisYear} title={String(year + 1)}>›</button>
        </div>
      </div>

      {empty ? <Card><Empty title="An empty year">Write a day or connect a source and come back.</Empty></Card> : <>
        <div className="cal2-stats">
          <div className="tile ring-tile"><Ring value={lived ? d.days_with_facts / lived : 0} /><div><b>{d.days_with_facts}<span>/{lived}</span></b><small>days recorded</small></div></div>
          <div className="tile"><span className="ti">✎</span><div><b>{d.entries}</b><small>{d.entries === 1 ? 'day' : 'days'} written{d.words ? ` · ${compact(d.words)} words` : ''}</small></div></div>
          {d.mood > 0 && <div className="tile"><span className="ti">{MOOD_ICON[Math.round(d.mood)]}</span><div><b>{d.mood.toFixed(1)}</b><small>average mood</small></div></div>}
          {d.photos > 0 && <div className="tile"><span className="ti">📷</span><div><b>{compact(d.photos)}</b><small>photos{d.videos ? ` · ${d.videos} videos` : ''}</small></div></div>}
          {d.km > 0 && <div className="tile"><span className="ti">🗺</span><div><b>{compact(unit === 'mi' ? d.km / 1.609344 : d.km)} {unit}</b><small>{Object.entries(d.modes).sort((a, b) => b[1] - a[1]).slice(0, 2).map(([m, v]) => `${m} ${compact(v)}`).join(' · ') || 'travelled'}</small></div></div>}
          {d.listens > 0 && <div className="tile"><span className="ti">🎧</span><div><b>{compact(d.listens)}</b><small>songs played</small></div></div>}
          {d.watch_min > 0 && <div className="tile"><span className="ti">📺</span><div><b>{Math.round(d.watch_min / 60)} h</b><small>{d.shows.length} shows · {d.movies} films</small></div></div>}
          {spendMain > 0 && <div className="tile" title={[money(spendMain, cur, false), ...others.map(([c, v]) => money(v, c, false))].join(' + ')}><span className="ti">💸</span><div><b>{money(spendMain, cur)}</b><small>spent{others.length ? ` (+ ${others.map(([c, v]) => money(v, c)).join(', ')})` : ''}</small></div></div>}
          {Object.entries(d.activity || {}).filter(([src]) => isActivity(src)).sort((a, b) => ACT_ORDER.indexOf(a[0]) - ACT_ORDER.indexOf(b[0])).map(([src, ay]) => {
            const m = ACTIVITY[src]
            const big = m.unit === 'min' ? `${Math.round(ay.minutes / 60)} h` : m.unit === 'qty' ? compact(ay.qty) : m.unit === 'amount' ? money(ay.amount, ay.currency || cur) : compact(ay.count)
            const small = m.unit === 'min' ? `${SOURCE_LABEL[src].toLowerCase()} · ${compact(ay.count)} ${ay.count === 1 ? m.one : m.many}` : m.unit === 'qty' ? `${m.qtyMany} · ${ay.days} days` : m.unit === 'amount' ? `${SOURCE_LABEL[src].toLowerCase()}` : `${ay.count === 1 ? m.one : m.many}${src === 'nextcloud' ? ' done' : ''}`
            return <div key={src} className="tile"><span className="ti">{SOURCE_ICON[src]}</span><div><b>{big}</b><small>{small}</small></div></div>
          })}
          {d.books.length > 0 && <div className="tile"><span className="ti">📚</span><div><b>{d.books.length}</b><small>books finished</small></div></div>}
          {d.cooked > 0 && <div className="tile"><span className="ti">🍳</span><div><b>{d.cooked}</b><small>dishes cooked</small></div></div>}
        </div>

        {aiOn && <StoryCard key={year} kind="year" period={String(year)} label={String(year)} />}

        <Card className="mb" title="Every day of the year" actions={<div className="layer-switch">{available.map(m => <button key={m.id} className={active === m.id ? 'on' : ''} onClick={() => setMetric(m.id)}><span>{m.icon}</span>{m.label}</button>)}</div>}>
          {cal ? <>
            <Heatmap year={year} days={days} metric={active} today={today} unit={unit} cur={cur} onOpen={iso => navigate(`/day/${iso}`)} />
            <MonthBars year={year} days={days} metric={active} today={today} unit={unit} cur={cur} onOpen={m => navigate(`/calendar/${m}`)} />
          </> : <Loading />}
        </Card>

        <div className="insights">
          {d.artists.length > 0 && <Card title="🎧 Most played">
            <Ranked items={d.artists.slice(0, 8)} />
            {d.tracks.length > 0 && <><div className="sub-head">Songs</div><Ranked items={d.tracks.slice(0, 5).map(t => { const [a, ...rest] = t.name.split(' – '); return { ...t, name: rest.join(' – ') || t.name, sub: rest.length ? a : '' } })} /></>}
          </Card>}
          {(patterns?.length || 0) > 0 && <Card title="🙂 What goes with a good day">
            <p className="muted small">Average mood with versus without. Small samples — hints, not rules.</p>
            {patterns!.map(p => <div key={p.label} className="pattern"><span>Days {p.label}</span><span className="cmp"><b className={p.with > p.without ? 'up' : 'down'}>{p.with.toFixed(1)}</b><span className="muted">vs {p.without.toFixed(1)}</span></span></div>)}
          </Card>}
          {moodsTotal > 0 && <Card title="Moods">
            <div className="mood-bars">{[5, 4, 3, 2, 1].map(m => <div key={m} className="mb-row"><span>{MOOD_ICON[m]} {cap(MOODS[m])}</span><div className="bar-h"><div style={{ width: `${d.moods[m] / moodsTotal * 100}%`, background: MOOD_COLOR[m] }} /></div><span className="muted small">{d.moods[m]}</span></div>)}</div>
            {moodsTotal >= 7 && <div className="wd-mood">{['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map((w, i) => { const v = d.weekday_mood[i + 1]; return <div key={w} title={v ? v.toFixed(1) : 'no data'}><i style={{ height: `${v ? v / 5 * 100 : 0}%`, background: v ? MOOD_COLOR[Math.round(v)] : undefined }} /><span>{w[0]}</span></div> })}</div>}
          </Card>}
          {d.shows.length > 0 && <Card title="📺 Most watched"><Ranked items={d.shows} unitLabel="ep" />{d.movies > 0 && <p className="muted small mt-sm" style={{ margin: '8px 0 0' }}>Plus {d.movies} film{d.movies === 1 ? '' : 's'} · {fmtMinutes(d.watch_min)} on screen in total.</p>}</Card>}
          {d.category_spend?.length > 0 && <Card title="💸 Where the money went">
            <Ranked items={d.category_spend.map(c => ({ name: c.name, count: c.amount, label: money(c.amount, cur), sub: `${c.count}×` }))} />
            <p className="muted small" style={{ margin: '8px 0 0' }}>{money(spendMain, cur, false)} in total{others.length ? `, plus ${others.map(([c, v]) => money(v, c, false)).join(', ')}` : ''}.</p>
          </Card>}
          {d.books.length > 0 && <Card title="📚 Books finished"><div className="inline-list">{d.books.map((b, i) => <div key={i} className="li"><span className="grow"><b>{b.title}</b>{b.authors && <span className="muted"> · {b.authors}</span>}</span>{b.rating ? <span className="stars">{'★'.repeat(Math.round(b.rating))}</span> : null}<span className="muted small nowrap">{b.finished_on ? fmtDate(b.finished_on, false) : ''}</span></div>)}</div></Card>}
          {(d.visits.length > 0 || d.places.length > 0) && <Card title="📍 Places">
            {d.visits.length > 0 && <Ranked items={d.visits.slice(0, 8)} unitLabel="×" />}
            {d.places.length > 0 && <>{d.visits.length > 0 && <div className="sub-head">Where the photos were taken</div>}<div className="chips">{d.places.map(p => <span key={p.name} className="chip">{p.name} <span className="muted">{p.count}</span></span>)}</div></>}
          </Card>}
          {d.trips.length > 0 && <Card title="✈️ Trips"><div className="inline-list">{d.trips.map(t => <Link key={t.id} className="li" to={`/day/${t.start_date}`} style={{ color: 'inherit' }}><span className="swatch" style={{ background: t.color || 'var(--accent)' }} /><span className="grow"><b>{t.name}</b>{t.destination && <span className="muted"> · {t.destination}</span>}</span><span className="muted small nowrap">{fmtDate(t.start_date, false)} – {fmtDate(t.end_date, false)}</span></Link>)}</div></Card>}
          {d.conditions.length > 0 && <Card title="⛅ Weather"><WeatherBar items={d.conditions} /></Card>}
          {d.dishes.length > 0 && <Card title="🍳 Cooked most"><Ranked items={d.dishes} unitLabel="×" /></Card>}
          {Object.entries(d.activity || {}).filter(([src, ay]) => isActivity(src) && (ay.top?.length || ay.titles?.length)).sort((a, b) => ACT_ORDER.indexOf(a[0]) - ACT_ORDER.indexOf(b[0])).map(([src, ay]) => {
            const m = ACTIVITY[src]
            const repeated = ['pinepods', 'audiobookshelf', 'nextcloud'].includes(src) ? (ay.titles || []).filter(t => t.count > 1) : []
            return <Card key={src} title={`${SOURCE_ICON[src]} ${SOURCE_LABEL[src]}`}>
              <p className="muted small" style={{ margin: '0 0 10px' }}>{[m.unit === 'min' ? fmtMinutes(Math.round(ay.minutes)) : '', m.unit === 'qty' ? `${compact(ay.qty)} ${m.qtyMany}` : m.unit === 'amount' ? money(ay.amount, ay.currency || cur, false) : '', m.unit === 'qty' ? '' : `${compact(ay.count)} ${ay.count === 1 ? m.one : m.many}`, `${ay.days} days`, ay.done && m.doneLabel ? `${ay.done} ${m.doneLabel}` : ''].filter(Boolean).join(' · ')}</p>
              {ay.top?.length > 0 && <Ranked items={ay.top.slice(0, 6)} />}
              {repeated.length > 0 && <><div className="sub-head">Most often</div><Ranked items={repeated.slice(0, 4)} unitLabel="×" /></>}
            </Card>
          })}
          {d.pinned.length > 0 && <Card title="★ Kept days"><DayList items={d.pinned} /></Card>}
          {d.best.length > 0 && <Card title="😄 Great days"><DayList items={d.best} /></Card>}
          {d.tags.length > 0 && <Card title="# Tags"><div className="chips">{d.tags.map(t => <Link key={t.tag} className="chip" to={`/journal?tag=${encodeURIComponent(t.tag)}`}>#{t.tag} <span className="muted">{t.count}</span></Link>)}</div></Card>}
        </div>
      </>}
    </div>
  )
}

function metricValue(d: CalendarDay | undefined, m: Metric): number {
  if (!d) return 0
  if (m === 'written') return d.entry ? 1 : 0
  if (m === 'mood') return d.mood
  return d.headlines?.[m] || 0
}
function fmtMetric(v: number, m: Metric, unit: 'km' | 'mi', cur: string): string {
  switch (m) {
    case 'immich': return `${Math.round(v)} photo${Math.round(v) === 1 ? '' : 's'}`
    case 'dawarich': return fmtKm(v, unit)
    case 'maloja': return `${Math.round(v)} songs`
    case 'jellystat': return fmtMinutes(Math.round(v)) || '0 min'
    case 'firefly': return money(v, cur)
    case 'mood': return v ? `${v.toFixed(1).replace('.0', '')} ${MOODS[Math.round(v)]}` : 'no mood'
    case 'written': return `${Math.round(v)} written`
  }
  return isActivity(m) ? fmtHeadline(m, v, cur) : String(Math.round(v))
}
function cellColor(v: number, m: Metric, max: number): string | undefined {
  if (!v) return undefined
  if (m === 'mood') return MOOD_COLOR[Math.round(v)]
  if (m === 'written') return 'var(--accent)'
  const pct = Math.round(20 + 80 * Math.min(1, Math.sqrt(v / max)))
  return `color-mix(in srgb, var(--accent) ${pct}%, var(--surface-2))`
}

/** 53 week columns × 7 weekday rows, GitHub style. */
function Heatmap({ year, days, metric, today, unit, cur, onOpen }: { year: number; days: CalendarDay[]; metric: Metric; today: string; unit: 'km' | 'mi'; cur: string; onOpen: (iso: string) => void }) {
  const byDate = useMemo(() => new Map(days.map(d => [d.date, d])), [days])
  const max = useMemo(() => {
    const vals = days.map(d => metricValue(d, metric)).filter(v => v > 0).sort((a, b) => a - b)
    return vals.length ? vals[Math.floor(vals.length * 0.95)] || vals[vals.length - 1] : 1 // ignore the top 5% outliers
  }, [days, metric])
  const first = new Date(year, 0, 1)
  const lead = (first.getDay() + 6) % 7 // Monday first
  const total = (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0)) ? 366 : 365
  const cells: (string | null)[] = [...Array(lead).fill(null)]
  for (let i = 0; i < total; i++) { const t = new Date(year, 0, 1 + i); cells.push(`${year}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`) }
  const weeks = Math.ceil(cells.length / 7)
  const wrap = useRef<HTMLDivElement>(null)
  useEffect(() => { const el = wrap.current; if (el && el.scrollWidth > el.clientWidth) el.scrollLeft = el.scrollWidth }, [year, days.length]) // phones: start at the recent weeks
  const monthCols = MON.map((_, mi) => Math.floor((lead + Math.round((Date.UTC(year, mi, 1) - Date.UTC(year, 0, 1)) / 86400000)) / 7))
  return (
    <div className="hm-wrap" ref={wrap}>
      <div className="hm" style={{ gridTemplateColumns: `28px repeat(${weeks}, minmax(9px, 1fr))` }}>
        <span />{Array.from({ length: weeks }).map((_, w) => <span key={w} className="hm-m">{monthCols.includes(w) ? MON[monthCols.lastIndexOf(w)] : ''}</span>)}
        {[0, 1, 2, 3, 4, 5, 6].map(r => <Fragment key={r}>
          <span key={'l' + r} className="hm-l">{r % 2 === 0 ? ['Mon', '', 'Wed', '', 'Fri', '', 'Sun'][r] : ''}</span>
          {Array.from({ length: weeks }).map((_, w) => {
            const iso = cells[w * 7 + r]
            if (!iso) return <span key={`${r}-${w}`} className="hm-c none" />
            const d = byDate.get(iso)
            const v = metricValue(d, metric)
            const future = iso > today
            return <button key={iso} className={`hm-c ${future ? 'future' : ''} ${iso === today ? 'today' : ''} ${d?.pinned ? 'kept' : ''}`} style={{ background: future ? undefined : cellColor(v, metric, max) }} disabled={future}
              title={`${fmtDate(iso)} · ${fmtMetric(v, metric, unit, cur)}${d?.highlight ? ` · ${d.highlight}` : ''}`} onClick={() => onOpen(iso)} />
          })}
        </Fragment>)}
      </div>
      <div className="hm-legend"><span>{metric === 'mood' ? '' : 'less'}</span>{metric === 'mood' ? [1, 2, 3, 4, 5].map(m => <span key={m} className="hm-k"><i style={{ background: MOOD_COLOR[m] }} />{MOODS[m]}</span>) : [0, .25, .5, .75, 1].map(f => <i key={f} style={{ background: f ? cellColor(f * max, metric, max) : 'var(--surface-2)' }} />)}<span>{metric === 'mood' ? '' : 'more'}</span><span className="muted small hm-tip">Hover for the day · click to open it · ★ kept days are outlined</span></div>
    </div>
  )
}

/** The same metric per month: sums, or averages for mood. */
function MonthBars({ year, days, metric, today, unit, cur, onOpen }: { year: number; days: CalendarDay[]; metric: Metric; today: string; unit: 'km' | 'mi'; cur: string; onOpen: (month: string) => void }) {
  const vals = MON.map((_, i) => {
    const key = `${year}-${String(i + 1).padStart(2, '0')}`
    const ds = days.filter(d => d.date.startsWith(key))
    if (metric === 'mood') { const ms = ds.filter(d => d.mood > 0); return { key, v: ms.length ? ms.reduce((a, d) => a + d.mood, 0) / ms.length : 0 } }
    return { key, v: ds.reduce((a, d) => a + metricValue(d, metric), 0) }
  })
  const max = metric === 'mood' ? 5 : Math.max(1, ...vals.map(x => x.v))
  const label = (v: number) => {
    if (!v) return ''
    if (metric === 'firefly') return money(v, cur)
    if (metric === 'dawarich') return `${compact(unit === 'mi' ? v / 1.609344 : v)}`
    if (metric === 'jellystat') return `${Math.round(v / 60)}h`
    if (metric === 'mood') return v.toFixed(1)
    if (isActivity(metric)) return fmtHeadlineShort(metric, v, cur)
    return compact(v)
  }
  return (
    <div className="mbars">
      {vals.map((x, i) => {
        const future = x.key > today.slice(0, 7)
        return <button key={x.key} className={`mbar ${future ? 'future' : ''} ${x.key === today.slice(0, 7) ? 'now' : ''}`} onClick={() => !future && onOpen(x.key)} disabled={future} title={`${MON[i]} · ${label(x.v) || 'nothing'}`}>
          <span className="mb-v">{label(x.v)}</span>
          <span className="mb-track"><i style={{ height: `${x.v / max * 100}%`, background: metric === 'mood' && x.v ? MOOD_COLOR[Math.round(x.v)] : undefined }} /></span>
          <span className="mb-m">{MON[i]}</span>
        </button>
      })}
    </div>
  )
}

function Ranked({ items, unitLabel = '' }: { items: { name: string; count: number; label?: string; sub?: string }[]; unitLabel?: string }) {
  const max = Math.max(1, ...items.map(i => i.count))
  return <div className="ranked2">{items.map((it, i) => <div key={it.name + i} className="rk2">
    <span className="rk-i">{i + 1}</span>
    <span className="rk-name" title={it.name}>{it.name}{it.sub && <small>{it.sub}</small>}</span>
    <span className="rk-val">{it.label ?? `${compact(it.count)}${unitLabel ? ' ' + unitLabel : ''}`}</span>
    <span className="rk-bar"><i style={{ width: `${it.count / max * 100}%` }} /></span>
  </div>)}</div>
}

function WeatherBar({ items }: { items: { name: string; count: number }[] }) {
  const total = items.reduce((a, b) => a + b.count, 0) || 1
  const color: Record<string, string> = { clear: '#f59e0b', partly: '#fbbf24', cloudy: '#94a3b8', fog: '#cbd5e1', rain: '#3b82f6', snow: '#e0f2fe', storm: '#6d28d9' }
  return <>
    <div className="wbar">{items.map(c => <i key={c.name} style={{ width: `${c.count / total * 100}%`, background: color[c.name] || 'var(--muted)' }} title={`${cap(c.name)}: ${c.count} days`} />)}</div>
    <div className="wlegend">{items.map(c => <span key={c.name}>{WEATHER_ICON[c.name]} {cap(c.name)} <b>{Math.round(c.count / total * 100)}%</b></span>)}</div>
  </>
}

function DayList({ items }: { items: YearRecap['pinned'] }) {
  return <div className="inline-list">{items.map(e => <Link key={e.id} className="li" to={`/day/${e.date}`} style={{ color: 'inherit' }}><span className="muted mono small nowrap">{fmtDate(e.date, false)}</span><span className="grow">{e.highlight || e.text.split('\n')[0] || '(no words)'}</span>{e.mood > 0 && <span>{MOOD_ICON[e.mood]}</span>}</Link>)}</div>
}
