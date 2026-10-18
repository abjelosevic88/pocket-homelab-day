import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { fmtDate, todayISO } from '../lib/format'
import { MOODS, MOOD_COLOR, MOOD_ICON, WEATHER_ICON, cap, fmtAmount, fmtKm, fmtMinutes, type YearRecap } from '../lib/types'
import { Card, Empty, Loading, PageHeader, useAsync } from '../components/ui'
import { StoryCard, useAI } from '../components/ai'
import type { Pattern } from '../lib/types'

const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const WD = ['', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

export default function Year() {
  const { year: yp } = useParams()
  const navigate = useNavigate()
  const { settings } = useApp()
  const thisYear = Number(todayISO().slice(0, 4))
  const year = yp && /^\d{4}$/.test(yp) ? Number(yp) : thisYear
  const { data, loading, error } = useAsync(() => api.get<YearRecap>(`${V1}/years/${year}`), [year])
  const { data: years } = useAsync(() => api.get<{ year: number; entries: number }[]>(`${V1}/years`), [])
  const unit = settings?.distance_unit || 'km'
  const aiOn = useAI()
  const { data: patterns } = useAsync(() => api.get<Pattern[]>(`${V1}/patterns?from=${year}-01-01&to=${year}-12-31`), [year])
  if (loading && !data) return <Loading />
  if (error) return <div className="callout danger">{error}</div>
  if (!data) return null
  const d = data
  const spend = Object.entries(d.spend || {}).sort((a, b) => b[1] - a[1])
  const mainCur = spend[0]?.[0]
  const moodsTotal = d.moods.slice(1).reduce((a, b) => a + b, 0)
  const empty = d.entries === 0 && d.days_with_facts === 0
  return (
    <>
      <PageHeader title={`${year} in review`} sub={empty ? 'Nothing yet for this year.' : `${d.entries} day${d.entries === 1 ? '' : 's'} written · ${d.days_with_facts} days with facts${d.longest_streak ? ` · longest streak ${d.longest_streak}` : ''}`}
        actions={<><button className="btn" onClick={() => navigate(`/year/${year - 1}`)}>‹ {year - 1}</button>{years && years.length > 2 && <select className="sel-native" value={year} onChange={e => navigate(`/year/${e.target.value}`)}>{years.map(y => <option key={y.year} value={y.year}>{y.year}</option>)}</select>}<button className="btn" onClick={() => navigate(`/year/${year + 1}`)} disabled={year >= thisYear}>{year + 1} ›</button></>} />
      {empty && <Card><Empty title="An empty year">Write a day or connect a source and come back.</Empty></Card>}
      {!empty && <>
        <div className="stats">
          <div className="stat"><div className="n">{d.entries}</div><div className="l">days written · {d.words.toLocaleString()} words · {d.moments} moments</div></div>
          {d.mood > 0 && <div className="stat"><div className="n">{d.mood.toFixed(1)} {MOOD_ICON[Math.round(d.mood)]}</div><div className="l">average mood</div></div>}
          {d.photos > 0 && <div className="stat"><div className="n">{d.photos.toLocaleString()}</div><div className="l">photos{d.videos ? ` · ${d.videos} videos` : ''}{d.busiest_day ? ` · most on ${fmtDate(d.busiest_day, false)}` : ''}</div></div>}
          {d.km > 0 && <div className="stat"><div className="n">{fmtKm(d.km, unit)}</div><div className="l">{Object.entries(d.modes).sort((a, b) => b[1] - a[1]).slice(0, 3).map(([m, v]) => `${m} ${fmtKm(v, unit)}`).join(' · ')}{d.farthest_day ? ` · farthest ${fmtDate(d.farthest_day, false)}` : ''}</div></div>}
          {d.listens > 0 && <div className="stat"><div className="n">{d.listens.toLocaleString()}</div><div className="l">listens{d.artists[0] ? ` · mostly ${d.artists[0].name}` : ''}</div></div>}
          {d.watch_min > 0 && <div className="stat"><div className="n">{Math.round(d.watch_min / 60)} h</div><div className="l">watched · {d.shows.length} shows · {d.movies} films</div></div>}
          {spend.length > 0 && <div className="stat"><div className="n">{fmtAmount(spend[0][1], spend[0][0])}</div><div className="l">spent{spend.length > 1 ? ` (+ ${spend.slice(1).map(([c, v]) => fmtAmount(v, c)).join(', ')})` : ''}</div></div>}
          {d.books.length > 0 && <div className="stat"><div className="n">{d.books.length}</div><div className="l">books finished</div></div>}
          {d.cooked > 0 && <div className="stat"><div className="n">{d.cooked}</div><div className="l">dishes cooked{d.dishes[0] ? ` · mostly ${d.dishes[0].name}` : ''}</div></div>}
        </div>

        {aiOn && <StoryCard key={year} kind="year" period={String(year)} label={String(year)} />}
        <Card title="Month by month" className="mb">
          <div className="months">
            {d.months.map((m, i) => {
              const written = m.entries
              return <Link key={m.month} to={`/calendar/${m.month}`} className="month" style={{ color: 'inherit' }}>
                <div className="mn">{MON[i]}</div>
                <div className="bar-wrap" title={`${written} days written`}>{written > 0 && <div className="bar" style={{ height: `${Math.min(100, written / 31 * 100)}%`, background: m.mood ? MOOD_COLOR[Math.round(m.mood)] : 'var(--accent)' }} />}</div>
                <div className="small muted">{written || '–'}</div>
                <div className="mstats small muted">
                  {m.photos > 0 && <span>📷 {m.photos}</span>}
                  {m.km > 0 && <span>🗺 {Math.round(m.km)}</span>}
                  {m.listens > 0 && <span>🎧 {m.listens}</span>}
                  {m.watch_min > 0 && <span>📺 {Math.round(m.watch_min / 60)}h</span>}
                  {mainCur && m.spend[mainCur] > 0 && <span>💸 {Math.round(m.spend[mainCur])}</span>}
                  {m.books > 0 && <span>📚 {m.books}</span>}
                  {m.sunny_pct > 0 && <span>☀️ {Math.round(m.sunny_pct)}%</span>}
                </div>
              </Link>
            })}
          </div>
          <p className="muted small mt-sm">Bars are days written; the colour is the month's mood.</p>
        </Card>

        <div className="two-col">
          <div className="grid" style={{ gap: 16 }}>
            {patterns && patterns.length > 0 && <Card title="What goes with a good day">
              <p className="muted small">Average mood on days with something versus the other days you rated. Small samples, so read them as hints.</p>
              {patterns.map(p => <div key={p.label} className="pattern"><span>Days {p.label}</span><span className="cmp"><b className={p.with > p.without ? 'up' : 'down'}>{p.with.toFixed(1)}</b><span className="muted">vs {p.without.toFixed(1)}</span><span className="muted small">n={p.n_with}</span></span></div>)}
            </Card>}
            {moodsTotal > 0 && <Card title="Moods">
              <div className="mood-bars">{[5, 4, 3, 2, 1].map(m => <div key={m} className="mb-row"><span>{MOOD_ICON[m]} {cap(MOODS[m])}</span><div className="bar-h"><div style={{ width: `${d.moods[m] / moodsTotal * 100}%`, background: MOOD_COLOR[m] }} /></div><span className="muted small">{d.moods[m]}</span></div>)}</div>
              <div className="chips mt-sm">{WD.slice(1).map((w, i) => d.weekday_mood[i + 1] > 0 && <span key={w} className="chip">{w} <b>{d.weekday_mood[i + 1].toFixed(1)}</b></span>)}</div>
            </Card>}
            {d.best.length > 0 && <Card title="Great days">
              <div className="inline-list">{d.best.map(e => <Link key={e.id} className="li" to={`/day/${e.date}`} style={{ color: 'inherit' }}><span className="muted mono small">{fmtDate(e.date, false)}</span><span className="grow">{e.highlight || e.text.split('\n')[0]}</span></Link>)}</div>
            </Card>}
            {d.pinned.length > 0 && <Card title="★ Kept days">
              <div className="inline-list">{d.pinned.map(e => <Link key={e.id} className="li" to={`/day/${e.date}`} style={{ color: 'inherit' }}><span className="muted mono small">{fmtDate(e.date, false)}</span><span className="grow">{e.highlight || e.text.split('\n')[0]}</span>{e.mood > 0 && <span>{MOOD_ICON[e.mood]}</span>}</Link>)}</div>
            </Card>}
            {d.tags.length > 0 && <Card title="Tags"><div className="chips">{d.tags.map(t => <Link key={t.tag} className="chip" to={`/journal?tag=${encodeURIComponent(t.tag)}`}>#{t.tag} <span className="muted">{t.count}</span></Link>)}</div></Card>}
            {d.trips.length > 0 && <Card title="Trips"><div className="inline-list">{d.trips.map(t => <div key={t.id} className="li"><span className="swatch" style={{ background: t.color || 'var(--accent)' }} /><span className="grow"><b>{t.name}</b>{t.destination && <span className="muted"> · {t.destination}</span>}</span><span className="muted small">{fmtDate(t.start_date, false)} – {fmtDate(t.end_date, false)}</span></div>)}</div></Card>}
          </div>
          <div className="grid" style={{ gap: 16 }}>
            {d.artists.length > 0 && <Card title="Most played"><Ranked items={d.artists} /><div className="muted small mt-sm">Tracks</div><Ranked items={d.tracks.slice(0, 5)} /></Card>}
            {d.shows.length > 0 && <Card title="Most watched"><Ranked items={d.shows} suffix=" episodes" /></Card>}
            {d.books.length > 0 && <Card title="Books finished"><div className="inline-list">{d.books.map((b, i) => <div key={i} className="li"><span className="grow"><b>{b.title}</b>{b.authors && <span className="muted"> · {b.authors}</span>}</span>{b.rating ? <span className="stars">{'★'.repeat(Math.round(b.rating))}</span> : null}<span className="muted small">{b.finished_on ? fmtDate(b.finished_on, false) : ''}</span></div>)}</div></Card>}
            {(d.visits.length > 0 || d.places.length > 0) && <Card title="Places">{d.visits.length > 0 && <Ranked items={d.visits} suffix=" visits" />}{d.places.length > 0 && <><div className="muted small mt-sm">Where the photos were taken</div><div className="chips">{d.places.map(p => <span key={p.name} className="chip">{p.name} <span className="muted">{p.count}</span></span>)}</div></>}</Card>}
            {d.categories.length > 0 && <Card title="Spending"><Ranked items={d.categories} suffix="×" />{spend.length > 0 && <div className="muted small mt-sm">{spend.map(([c, v]) => fmtAmount(v, c)).join(' · ')}</div>}</Card>}
            {d.dishes.length > 0 && <Card title="Cooked most"><Ranked items={d.dishes} suffix="×" /></Card>}
            {d.conditions.length > 0 && <Card title="Weather"><div className="chips">{d.conditions.map(c => <span key={c.name} className="chip">{WEATHER_ICON[c.name]} {cap(c.name)} <span className="muted">{c.count}</span></span>)}</div></Card>}
            {d.watch_min > 0 && <p className="muted small">{fmtMinutes(d.watch_min)} on screen in total.</p>}
          </div>
        </div>
      </>}
    </>
  )
}

function Ranked({ items, suffix = '' }: { items: { name: string; count: number }[]; suffix?: string }) {
  const max = items[0]?.count || 1
  return <div className="ranked">{items.map(it => <div key={it.name} className="rk"><span className="grow" title={it.name}>{it.name}</span><div className="bar-h"><div style={{ width: `${it.count / max * 100}%` }} /></div><span className="muted small">{it.count}{suffix}</span></div>)}</div>
}
