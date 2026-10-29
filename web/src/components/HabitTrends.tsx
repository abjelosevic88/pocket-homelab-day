import { useLayoutEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useAsync } from './ui'
import { Icon } from './icons'

interface TrendHabit { id: number; name: string; color: string; entries: Record<string, number> }
interface TrendTracker { id: number; name: string; kind: string; unit: string; min?: number; max?: number; higher_is_better?: boolean; entries: Record<string, number> }
interface Trend { from: string; to: string; live: boolean; habits: TrendHabit[]; trackers: TrendTracker[] }

const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const RANGES = [{ n: 1, label: 'Month' }, { n: 3, label: '3 months' }, { n: 6, label: '6 months' }, { n: 12, label: 'Year' }]
const iso = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
const addMonths = (ym: string, n: number) => { const d = new Date(Number(ym.slice(0, 4)), Number(ym.slice(5)) - 1 + n, 1); return iso(d).slice(0, 7) }
const lastDay = (ym: string) => iso(new Date(Number(ym.slice(0, 4)), Number(ym.slice(5)), 0))
function datesBetween(a: string, b: string): string[] {
  const out: string[] = []
  for (let d = new Date(a + 'T12:00:00'); iso(d) <= b; d.setDate(d.getDate() + 1)) out.push(iso(d))
  return out
}
const avg = (xs: number[]) => xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : null

/** The days of a period grouped into points: one per day for a month, one per week (Monday on) beyond. */
function buckets(dates: string[], daily: boolean): string[][] {
  if (daily) return dates.map(d => [d])
  const out: string[][] = []
  for (const d of dates) {
    if (!out.length || new Date(d + 'T12:00:00').getDay() === 1) out.push([])
    out[out.length - 1].push(d)
  }
  return out
}

function useWidth<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  const [w, setW] = useState(0)
  useLayoutEffect(() => {
    if (!ref.current) return
    setW(Math.round(ref.current.getBoundingClientRect().width))
    const ro = new ResizeObserver(e => setW(Math.round(e[0].contentRect.width)))
    ro.observe(ref.current)
    return () => ro.disconnect()
  }, [])
  return [ref, w] as const
}

/**
 * Habits over time under the calendar: each tracker as a line over its
 * good/ok/bad bands and each habit's success rate, for the shown month or the
 * months up to it, compared with the period before.
 */
export default function HabitTrends({ month, today }: { month: string; today: string }) {
  const [range, setRangeRaw] = useState<number>(() => Number(localStorage.getItem('pd-habit-range')) || 1)
  const setRange = (n: number) => { setRangeRaw(n); localStorage.setItem('pd-habit-range', String(n)) }
  const start = addMonths(month, -(range - 1))
  const prevStart = addMonths(start, -range)
  const { data } = useAsync(() => api.get<Trend>(`${V1}/habits/trend?from=${prevStart}&to=${month}`), [prevStart, month])
  const end = lastDay(month) < today ? lastDay(month) : today
  const cur = datesBetween(start + '-01', end)
  const prev = datesBetween(prevStart + '-01', lastDay(addMonths(start, -1)))
  if (!data || cur.length === 0) return null
  const used = (e: Record<string, number>) => cur.some(d => d in e) || prev.some(d => d in e)
  const habits = data.habits.filter(h => used(h.entries))
  const trackers = data.trackers.filter(t => used(t.entries))
  if (!habits.length && !trackers.length) return null
  const daily = range === 1
  const vs = range === 1 ? `vs ${MON[Number(prevStart.slice(5)) - 1]}` : `vs the ${range} months before`
  const ctx: Ctx = { cur, prev, daily, vs, groups: buckets(cur, daily) }

  return (
    <section className="card ht mt">
      <div className="card-head">
        <h2 className="grow"><Icon name="habit" size={16} />Habits over time</h2>
        <div className="layer-switch" role="radiogroup" aria-label="Period">
          {RANGES.map(r => <button type="button" key={r.n} role="radio" aria-checked={range === r.n} className={range === r.n ? 'on' : ''} onClick={() => setRange(r.n)}>{r.label}</button>)}
        </div>
      </div>
      <div className="card-body">
        {!data.live && <p className="muted small ht-note">From the check-ins saved with each day — Pocket Habits couldn't be asked just now, so days filled in there later may be missing.</p>}
        <div className="ht-grid">
          {trackers.map(t => <TrackerChart key={t.id} t={t} c={ctx} />)}
          {habits.length > 0 && <HabitRates habits={habits} c={ctx} />}
        </div>
      </div>
    </section>
  )
}

interface Ctx { cur: string[]; prev: string[]; daily: boolean; vs: string; groups: string[][] }

function Delta({ d, better, fmt }: { d: number | null; better?: boolean; fmt: (v: number) => string }) {
  if (d == null || Math.abs(d) < 0.05) return d == null ? null : <span className="ht-delta flat">± 0</span>
  const good = better === undefined ? null : (d > 0) === better
  return <span className={`ht-delta ${good == null ? 'flat' : good ? 'up' : 'down'}`}>{d > 0 ? '↑' : '↓'} {fmt(Math.abs(d))}</span>
}

function TrackerChart({ t, c }: { t: TrendTracker; c: Ctx }) {
  const navigate = useNavigate()
  const [ref, w] = useWidth<HTMLDivElement>()
  const yesno = t.kind === 'yesno'
  const vals = (ds: string[]) => ds.filter(d => d in t.entries).map(d => t.entries[d])
  const now = avg(vals(c.cur)), before = avg(vals(c.prev))
  const hib = t.higher_is_better ?? true
  const all = [...vals(c.cur), ...vals(c.prev)]
  let lo = t.min ?? Math.min(...all), hi = t.max ?? Math.max(...all)
  if (t.kind !== 'scale' && !yesno) { const pad = (Math.max(...all) - Math.min(...all)) * .15 || 1; lo = Math.min(...all) - pad; hi = Math.max(...all) + pad }
  if (yesno) { lo = 0; hi = 1 }
  if (hi <= lo) hi = lo + 1
  const H = 120, padL = 22, padR = 6, padT = 6, padB = 18
  const iw = Math.max(0, w - padL - padR), ih = H - padT - padB
  const n = c.groups.length
  const x = (i: number) => padL + (n <= 1 ? iw / 2 : (i + .5) * iw / n)
  const y = (v: number) => padT + ih - (v - lo) / (hi - lo) * ih
  const xDay = (d: string) => { const i = c.cur.indexOf(d); return padL + (i + .5) * iw / c.cur.length }
  const fmt = (v: number) => yesno ? `${Math.round(v * 100)}%` : `${+v.toFixed(1)}${t.unit ? ' ' + t.unit : ''}`
  const series = c.groups.map(g => avg(vals(g)))
  const line = series.map((v, i) => v == null ? null : `${x(i)},${y(v)}`).filter(Boolean)
  const bands = t.kind === 'scale' && !yesno ? [0, 1, 2].map(k => ({ y0: y(lo + (hi - lo) * (k + 1) / 3), y1: y(lo + (hi - lo) * k / 3), cls: (hib ? ['bad', 'ok', 'good'] : ['good', 'ok', 'bad'])[k] })) : []
  // which band a value sits in (top third good when higher is better)
  const zone = (v: number) => { const k = Math.min(2, Math.floor((v - lo) / (hi - lo) * 3)); return (hib ? ['bad', 'ok', 'good'] : ['good', 'ok', 'bad'])[k] }
  const ticks = c.daily ? c.cur.filter(d => [1, 8, 15, 22, 29].includes(Number(d.slice(8)))) : c.cur.filter(d => d.endsWith('-01'))

  return (
    <div className="ht-card">
      <div className="ht-head">
        <div className="grow"><b>{t.name}</b>{yesno && <small className="muted"> · how often “yes”</small>}</div>
        {now != null && <span className="ht-avg">{yesno ? `${vals(c.cur).filter(v => v > 0).length} of ${vals(c.cur).length} days` : `avg ${fmt(now)}`}</span>}
        <Delta d={now != null && before != null ? now - before : null} better={hib} fmt={fmt} />
      </div>
      <div ref={ref} className="ht-chart">
        {w > 0 && <svg width={w} height={H} role="img" aria-label={`${t.name} over time`}>
          {bands.map(b => <rect key={b.cls} x={padL} width={iw} y={b.y0} height={b.y1 - b.y0} className={`ht-band ${b.cls}`} />)}
          {!yesno && [lo, hi].map(v => <text key={v} x={padL - 5} y={y(v) + 3.5} className="ht-tick" textAnchor="end">{+v.toFixed(1)}</text>)}
          {before != null && <g className="ht-prev"><line x1={padL} x2={padL + iw} y1={y(before)} y2={y(before)}><title>{`Average before: ${fmt(before)}`}</title></line></g>}
          {yesno ? series.map((v, i) => v == null ? null : <rect key={i} x={x(i) - Math.max(1.5, iw / n * .32)} width={Math.max(3, iw / n * .64)} y={y(v)} height={Math.max(1.5, y(0) - y(v))} rx={2} className={`ht-bar ${(v > 0) === hib ? 'good' : v > 0 ? 'bad' : ''}`}><title>{`${c.groups[i][0]}${c.daily ? '' : ' week'}: ${c.daily ? (v > 0 ? 'yes' : 'no') : fmt(v)}`}</title></rect>)
            : <>
              {!c.daily && c.cur.filter(d => d in t.entries).map(d => <circle key={d} cx={xDay(d)} cy={y(t.entries[d])} r={1.8} className="ht-dot-faint" />)}
              {line.length > 1 && <polyline points={line.join(' ')} className="ht-line" />}
              {series.map((v, i) => v == null ? null : <circle key={i} cx={x(i)} cy={y(v)} r={c.daily ? 3.6 : 3} className="ht-dot" style={{ fill: bands.length ? `var(--ht-${zone(v)}-ink)` : undefined }} onClick={() => c.daily && navigate(`/day/${c.groups[i][0]}`)}>
                <title>{c.daily ? `${c.groups[i][0]}: ${fmt(v)}` : `Week of ${c.groups[i][0]}: avg ${fmt(v)}`}</title></circle>)}
            </>}
          {ticks.map(d => <text key={d} x={c.daily ? xDay(d) : xDay(d) + 2} y={H - 4} className="ht-tick" textAnchor={c.daily ? 'middle' : 'start'}>{c.daily ? Number(d.slice(8)) : MON[Number(d.slice(5, 7)) - 1]}</text>)}
        </svg>}
      </div>
    </div>
  )
}

function HabitRates({ habits, c }: { habits: TrendHabit[]; c: Ctx }) {
  const navigate = useNavigate()
  const rate = (h: TrendHabit, ds: string[]) => { const m = ds.filter(d => d in h.entries); return m.length ? m.filter(d => h.entries[d] > 0).length / m.length : null }
  const pct = (v: number) => `${Math.round(v * 100)}%`
  const sumRate = (ds: string[]) => { let done = 0, all = 0; for (const h of habits) for (const d of ds) if (d in h.entries) { all++; if (h.entries[d] > 0) done++ } return all ? done / all : null }
  const tot = sumRate(c.cur), totPrev = sumRate(c.prev)
  return (
    <div className="ht-card ht-habits">
      <div className="ht-head">
        <div className="grow"><b>Habits kept</b><small className="muted"> · share of marked days done</small></div>
        {tot != null && <span className="ht-avg">{pct(tot)}</span>}
        <Delta d={tot != null && totPrev != null ? tot - totPrev : null} better fmt={v => `${Math.round(v * 100)} pts`} />
      </div>
      <div className="ht-rows">
        {habits.map(h => {
          const r = rate(h, c.cur), rp = rate(h, c.prev)
          return <div key={h.id} className="ht-row">
            <span className="ht-name"><i style={{ background: h.color }} />{h.name}</span>
            <span className="ht-strip" style={{ gridTemplateColumns: `repeat(${c.groups.length}, minmax(0, 1fr))` }}>
              {c.groups.map(g => {
                if (c.daily) {
                  const v = h.entries[g[0]]
                  return <button type="button" key={g[0]} className={`ht-cell ${v > 0 ? 'done' : v < 0 ? 'missed' : ''}`} title={`${g[0]}: ${v > 0 ? 'done' : v < 0 ? 'missed' : 'not marked'}`} onClick={() => navigate(`/day/${g[0]}`)} />
                }
                const wr = rate(h, g)
                return <span key={g[0]} className="ht-wk" title={`Week of ${g[0]}: ${wr == null ? 'not marked' : pct(wr)}`}><b style={{ height: wr == null ? 0 : `${Math.max(8, wr * 100)}%`, background: wr == null ? undefined : `color-mix(in srgb, var(--ht-good-ink) ${Math.round(wr * 100)}%, var(--ht-bad-ink))` }} /></span>
              })}
            </span>
            <span className="ht-rate">{r == null ? '–' : pct(r)}</span>
            <span className="ht-d"><Delta d={r != null && rp != null ? r - rp : null} better fmt={v => `${Math.round(v * 100)}`} /></span>
          </div>
        })}
      </div>
      <p className="muted small ht-foot">Arrows compare with {c.vs.replace(/^vs /, '')}{c.daily ? '' : '; bars are weeks'}.</p>
    </div>
  )
}
