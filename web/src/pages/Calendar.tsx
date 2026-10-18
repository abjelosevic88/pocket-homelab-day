import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { todayISO } from '../lib/format'
import { MOOD_COLOR, MOOD_ICON, WEATHER_ICON, type CalendarDay } from '../lib/types'
import { Loading, PageHeader, useAsync } from '../components/ui'
import { StoryCard, useAI } from '../components/ai'

const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']

export default function Calendar() {
  const { month: mp } = useParams()
  const navigate = useNavigate()
  const { settings } = useApp()
  const aiOn = useAI()
  const today = todayISO()
  const month = mp && /^\d{4}-\d{2}$/.test(mp) ? mp : today.slice(0, 7)
  const [y, m] = month.split('-').map(Number)
  const { data, loading, error } = useAsync(() => api.get<{ days: CalendarDay[] }>(`${V1}/calendar?month=${month}`), [month])
  const shift = (n: number) => { const d = new Date(y, m - 1 + n, 1); navigate(`/calendar/${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`) }
  const weekStart = settings?.week_start === 'sun' ? 0 : 1
  const firstDow = new Date(y, m - 1, 1).getDay() // 0 Sun
  const lead = (firstDow - weekStart + 7) % 7
  const labels = weekStart === 1 ? ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'] : ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
  const days = data?.days || []
  const written = days.filter(d => d.entry).length
  const moods = days.filter(d => d.mood > 0)
  const avgMood = moods.length ? moods.reduce((a, d) => a + d.mood, 0) / moods.length : 0
  const sum = (k: string) => days.reduce((a, d) => a + (d.headlines?.[k] || 0), 0)
  return (
    <>
      <PageHeader title={`${MONTHS[m - 1]} ${y}`} sub={data ? `${written} of ${days.filter(d => d.date <= today).length} days written${avgMood ? ` · mood ${avgMood.toFixed(1)}` : ''}${sum('immich') ? ` · ${sum('immich')} photos` : ''}${sum('dawarich') ? ` · ${Math.round(sum('dawarich'))} km` : ''}${sum('maloja') ? ` · ${sum('maloja')} listens` : ''}` : ''}
        actions={<><button className="btn" onClick={() => shift(-1)}>‹</button><Link className="btn" to="/calendar">This month</Link><button className="btn" onClick={() => shift(1)} disabled={month >= today.slice(0, 7)}>›</button><Link className="btn ghost" to={`/year/${y}`}>Year</Link></>} />
      {loading && !data && <Loading />}
      {error && <div className="callout danger">{error}</div>}
      {data && <div className="cal">
        <div className="cal-head">{labels.map(l => <div key={l}>{l}</div>)}</div>
        <div className="cal-grid">
          {Array.from({ length: lead }).map((_, i) => <div key={'l' + i} className="cal-cell empty" />)}
          {days.map(d => {
            const future = d.date > today
            const n = Number(d.date.slice(8))
            const h = d.headlines || {}
            return <Link key={d.date} to={future ? '#' : `/day/${d.date}`} className={`cal-cell ${d.entry ? 'written' : ''} ${d.date === today ? 'today' : ''} ${future ? 'future' : ''}`} style={d.mood ? { borderTopColor: MOOD_COLOR[d.mood] } : undefined} onClick={e => future && e.preventDefault()}>
              <div className="cal-top"><span className="n">{n}</span>{d.weather && <span className="w" title={d.weather}>{WEATHER_ICON[d.weather]}</span>}{d.mood > 0 && <span className="mood">{MOOD_ICON[d.mood]}</span>}{d.pinned && <span title="Kept">★</span>}</div>
              {d.highlight && <div className="cal-hl">{d.highlight}</div>}
              {!future && <div className="cal-dots">
                {h.immich > 0 && <span title={`${h.immich} photos`}>📷{h.immich >= 1 ? Math.round(h.immich) : ''}</span>}
                {h.dawarich > 0 && <span title={`${h.dawarich} km`}>🗺{Math.round(h.dawarich)}</span>}
                {h.maloja > 0 && <span title={`${h.maloja} listens`}>🎧{Math.round(h.maloja)}</span>}
                {h.jellystat > 0 && <span title={`${Math.round(h.jellystat)} min watched`}>📺</span>}
                {h.kitchen > 0 && <span title="cooked">🍳</span>}
                {h.bookshelf > 0 && <span title="finished a book">📚</span>}
                {h.travel > 0 && <span title="on a trip">✈️</span>}
                {d.moments > 0 && <span title={`${d.moments} moments`}>• {d.moments}</span>}
              </div>}
            </Link>
          })}
        </div>
      </div>}
      {aiOn && month <= today.slice(0, 7) && <div className="mt"><StoryCard key={month} kind="month" period={month} label={`${MONTHS[m - 1]} ${y}`} /></div>}
      <p className="muted small mt">The top edge takes the day's mood colour. Numbers: photos, kilometres, listens. Click a day to read or write it.</p>
    </>
  )
}
