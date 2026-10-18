import React, { useState } from 'react'
import { V1 } from '../lib/api'
import { Card } from './ui'
import {
  SOURCE_APP, SOURCE_ICON, SOURCE_LABEL, WEATHER_ICON, cap, fmtAmount, fmtKm, fmtMinutes,
  type Activity, type Car, type Checkin, type TrackerRow, ACTIVITY, isActivity, fmtHeadline, type Cooked, type Fact, type Money, type Music, type Photos, type Pin, type Reading, type Trips, type Watched, type WeatherDay, type Where,
} from '../lib/types'

export interface PinFn { (p: Omit<Pin, 'id' | 'user_id' | 'date' | 'created_at'>): void }
type PinBtnProps = { ref_: string; title: string; subtitle?: string; image?: string }

interface Props { source: string; fact?: Fact; link?: string; unit: 'km' | 'mi'; pins: Pin[]; onPin: PinFn; onUnpin: (id: number) => void; date: string }

/** One source's card on the day page. */
export function FactCard({ source, fact, link, unit, pins, onPin, onUnpin, date }: Props) {
  const title = <span className="row" style={{ gap: 8 }}><span>{SOURCE_ICON[source]}</span>{SOURCE_LABEL[source] || cap(source)}</span>
  const actions = <>
    {fact?.error && <span className="badge warn" title={fact.error}>stale</span>}
    {link && <a className="btn ghost sm" href={link} target="_blank" rel="noreferrer" title={`Open ${SOURCE_APP[source] || source}`}>↗</a>}
  </>
  const empty = (msg: string) => <p className="muted small" style={{ margin: 0 }}>{msg}</p>
  if (!fact || fact.payload === null || fact.payload === undefined) {
    return <Card title={title} actions={actions} className="fact-card">{empty(fact?.error ? `Couldn't reach ${SOURCE_APP[source]}: ${fact.error}` : `Nothing from ${SOURCE_APP[source] || source} for this day.`)}</Card>
  }
  const pinned = (ref: string) => pins.find(p => p.source === source && p.ref === ref)
  const PinBtn = ({ ref_, title, subtitle, image }: PinBtnProps) => {
    const p = pinned(ref_)
    return <button className={`pin-btn ${p ? 'on' : ''}`} title={p ? 'Unpin from this day' : 'Keep on this day'} onClick={() => p ? onUnpin(p.id) : onPin({ source, ref: ref_, title, subtitle: subtitle || '', image: image || '' })}>{p ? '★' : '☆'}</button>
  }
  let body: React.ReactNode = null
  switch (source) {
    case 'dawarich': {
      const w = fact.payload as Where
      if (!w.distance_km && !w.entries?.length) { body = empty('No movement recorded.'); break }
      const modes = Object.entries(w.modes || {}).filter(([, v]) => v > 0).sort((a, b) => b[1] - a[1])
      const visits = (w.entries || []).filter(e => e.kind === 'visit')
      body = <>
        <div className="fact-head"><b>{fmtKm(w.distance_km, unit)}</b>{w.moving_minutes > 0 && <span className="muted"> · {fmtMinutes(w.moving_minutes)} moving</span>}{w.first_move && <span className="muted"> · {w.first_move}–{w.last_move}</span>}</div>
        {modes.length > 0 && <div className="chips">{modes.map(([m, v]) => <span key={m} className="chip">{modeIcon(m)} {fmtKm(v, unit)}</span>)}</div>}
        {visits.length > 0 && <div className="inline-list mt-sm">{visits.map((v, i) => <div key={i} className="li"><span className="muted mono small">{v.start}</span><span className="grow">{v.name || 'Somewhere'}</span><span className="muted small">{fmtMinutes(v.minutes)}</span><PinBtn ref_={`visit:${v.start}:${v.name}`} title={v.name || 'Somewhere'} subtitle={`${v.start}–${v.end}`} /></div>)}</div>}
      </>
      break
    }
    case 'immich': {
      const p = fact.payload as Photos
      if (!p.count) { body = empty('No photos taken.'); break }
      body = <PhotoGrid p={p} date={date} link={link} PinBtn={PinBtn} />
      break
    }
    case 'maloja': {
      const m = fact.payload as Music
      if (!m.listens) { body = empty('Nothing played.'); break }
      body = <>
        <div className="fact-head"><b>{m.listens} listens</b>{m.minutes > 0 && <span className="muted"> · ~{fmtMinutes(m.minutes)}</span>}{m.first && <span className="muted"> · {m.first}–{m.last}</span>}</div>
        {m.artists?.length > 0 && <div className="chips">{m.artists.map(a => <span key={a.name} className="chip">{a.name} <span className="muted">{a.count}</span></span>)}</div>}
        <Expandable label={`${m.items.length} tracks`}>
          <div className="inline-list mt-sm">{m.items.map((l, i) => <div key={i} className="li"><span className="muted mono small">{l.time}</span><span className="grow"><b>{l.title}</b> <span className="muted">{l.artist}</span></span><PinBtn ref_={`track:${l.artist}:${l.title}`} title={l.title} subtitle={l.artist} /></div>)}</div>
        </Expandable>
      </>
      break
    }
    case 'jellystat': {
      const w = fact.payload as Watched
      if (!w.count) { body = empty('Nothing watched.'); break }
      body = <>
        <div className="fact-head"><b>{fmtMinutes(w.minutes) || `${w.count} items`}</b><span className="muted"> · {w.episodes ? `${w.episodes} episode${w.episodes === 1 ? '' : 's'}` : ''}{w.episodes && w.movies ? ', ' : ''}{w.movies ? `${w.movies} film${w.movies === 1 ? '' : 's'}` : ''}</span></div>
        <div className="inline-list">{w.items.map((it, i) => <div key={i} className="li"><span className="muted mono small">{it.time}</span><span className="grow"><b>{it.series || it.title}</b>{it.series && <span className="muted"> · {it.title}</span>}</span><span className="muted small">{fmtMinutes(it.minutes)}</span><PinBtn ref_={`item:${it.item_id || it.title}`} title={it.series || it.title} subtitle={it.series ? it.title : fmtMinutes(it.minutes)} /></div>)}</div>
      </>
      break
    }
    case 'firefly': {
      const m = fact.payload as Money
      if (!m.count) { body = empty('Nothing spent.'); break }
      const totals = Object.entries(m.totals || {})
      body = <>
        <div className="fact-head"><b>{totals.map(([c, v]) => fmtAmount(v, c)).join(' + ')}</b><span className="muted"> · {m.count} transaction{m.count === 1 ? '' : 's'}</span></div>
        {m.categories?.length > 0 && <div className="chips">{m.categories.map(c => <span key={c.name} className="chip">{c.name} <span className="muted">{c.count}</span></span>)}</div>}
        <div className="inline-list mt-sm">{m.items.map(s => <div key={s.id} className="li"><span className="muted mono small">{s.time}</span><span className="grow">{s.description}{s.category && <span className="muted small"> · {s.category}</span>}</span><span className="money">{fmtAmount(s.amount, s.currency)}</span><PinBtn ref_={`tx:${s.id}`} title={s.description} subtitle={fmtAmount(s.amount, s.currency)} /></div>)}</div>
      </>
      break
    }
    case 'habits': {
      const c = fact.payload as Checkin
      if (!c.total && !c.journal && !c.trackers?.length) { body = empty('No check-in.'); break }
      body = <HabitsBody c={c} />
      break
    }
    case 'bookshelf': {
      const r = fact.payload as Reading
      if (!r.finished?.length && !r.started?.length && !r.reading?.length) { body = empty('Nothing on the go.'); break }
      const row = (b: Reading['reading'][number], label: string) => <div key={label + b.id} className="li"><span className="badge cat">{label}</span><span className="grow"><b>{b.title}</b>{b.authors && <span className="muted"> · {b.authors}</span>}</span>{b.percent ? <span className="muted small">{b.percent}%</span> : null}{b.rating ? <span className="stars">{'★'.repeat(Math.round(b.rating))}</span> : null}<PinBtn ref_={`book:${b.id}`} title={b.title} subtitle={b.authors} /></div>
      body = <div className="inline-list">{(r.finished || []).map(b => row(b, 'finished'))}{(r.started || []).map(b => row(b, 'started'))}{(r.reading || []).map(b => row(b, 'reading'))}</div>
      break
    }
    case 'travel': {
      const t = fact.payload as Trips
      if (!t.trips?.length) { body = empty('Not travelling.'); break }
      body = <div className="inline-list">{t.trips.map(tr => <div key={tr.id} className="li"><span className="swatch" style={{ background: tr.color || 'var(--accent)' }} /><span className="grow"><b>{tr.name}</b>{tr.destination && <span className="muted"> · {tr.destination}</span>}</span><span className="badge">day {tr.day_no} of {tr.days}</span>{link && <a className="btn ghost sm" href={`${link}/trips/${tr.id}`} target="_blank" rel="noreferrer">↗</a>}</div>)}</div>
      break
    }
    case 'kitchen': {
      const c = fact.payload as Cooked
      if (!c.cooks?.length) { body = empty('Nothing cooked.'); break }
      body = <div className="inline-list">{c.cooks.map(ck => <div key={ck.id} className="li"><span>🍳</span><span className="grow"><b>{ck.title}</b>{ck.servings ? <span className="muted"> · {ck.servings} servings</span> : null}{ck.by && <span className="muted small"> · {ck.by}</span>}</span>{ck.rating > 0 && <span className="stars">{'★'.repeat(ck.rating)}</span>}<PinBtn ref_={`cook:${ck.id}`} title={ck.title} subtitle="cooked" /></div>)}</div>
      break
    }
    case 'garage': {
      const c = fact.payload as Car
      if (!c.items?.length) { body = empty('Nothing in the garage.'); break }
      body = <div className="inline-list">{c.items.map((it, i) => <div key={i} className="li"><span>{it.kind === 'fuel' ? '⛽' : '🔧'}</span><span className="grow"><b>{it.vehicle}</b> <span className="muted">{it.kind === 'fuel' ? it.title : `${it.kind} · ${it.title}`}{it.station ? ` · ${it.station}` : ''}</span></span>{it.cost ? <span className="money">{fmtAmount(it.cost / 100, it.currency)}</span> : null}</div>)}</div>
      break
    }
    case 'weather': {
      const w = fact.payload as WeatherDay
      body = <div className="fact-head"><span style={{ fontSize: 22 }}>{WEATHER_ICON[w.condition] || '🌡'}</span> <b>{cap(w.condition || 'weather')}</b>{w.t_min !== null && w.t_max !== null && <span className="muted"> · {Math.round(w.t_min)}–{Math.round(w.t_max)} °C</span>}{w.precip_mm ? <span className="muted"> · {w.precip_mm} mm</span> : null}{w.sunshine_h ? <span className="muted"> · {w.sunshine_h} h sun</span> : null}{w.place && <span className="muted"> · {w.place}</span>}</div>
      break
    }
    default:
      if (isActivity(source)) {
        const a = fact.payload as Activity
        if (!a.count) { body = empty(`Nothing in ${SOURCE_APP[source] || source} this day.`); break }
        body = <ActivityBody source={source} a={a} link={link} PinBtn={PinBtn} />
        break
      }
      body = <pre className="small">{JSON.stringify(fact.payload, null, 1).slice(0, 800)}</pre>
  }
  return <Card title={title} actions={actions} className={`fact-card src-${source}`}>{body}</Card>
}

function modeIcon(m: string) {
  switch (m) {
    case 'walking': return '🚶'
    case 'running': return '🏃'
    case 'cycling': return '🚴'
    case 'driving': return '🚗'
    case 'train': return '🚆'
    case 'flying': case 'plane': return '✈️'
    case 'boat': return '⛵'
  }
  return '•'
}

export function Expandable({ label, children, open: init = false }: { label: string; children: React.ReactNode; open?: boolean }) {
  const [open, setOpen] = useState(init)
  return <div className="mt-sm"><button className="btn ghost sm" onClick={() => setOpen(o => !o)}>{open ? '▾' : '▸'} {label}</button>{open && children}</div>
}

function PhotoGrid({ p, date, link, PinBtn }: { p: Photos; date: string; link?: string; PinBtn: (props: PinBtnProps) => React.ReactNode }) {
  const [all, setAll] = useState(false)
  const [big, setBig] = useState<string | null>(null)
  const items = all ? p.items : p.items.slice(0, 12)
  const isDemo = p.items[0]?.id?.startsWith('demo-')
  return <>
    <div className="fact-head"><b>{p.count - p.videos} photo{p.count - p.videos === 1 ? '' : 's'}</b>{p.videos > 0 && <span className="muted"> · {p.videos} video{p.videos === 1 ? '' : 's'}</span>}{p.favorites > 0 && <span className="muted"> · {p.favorites} ★</span>}{p.places?.length > 0 && <span className="muted"> · {p.places.join(', ')}</span>}{p.first && <span className="muted"> · {p.first}–{p.last}</span>}</div>
    <div className="photo-grid">{items.map(ph => <div key={ph.id} className="ph">
      {isDemo ? <div className="ph-img demo" /> : <img className="ph-img" src={`${V1}/photos/${ph.id}/thumb`} alt={ph.file_name || ''} loading="lazy" onClick={() => setBig(ph.id)} />}
      <span className="ph-time">{ph.time}{ph.type === 'VIDEO' ? ' ▶' : ''}</span>
      <span className="ph-pin"><PinBtn ref_={`asset:${ph.id}`} title={ph.file_name || `Photo at ${ph.time}`} subtitle={[ph.time, ph.city].filter(Boolean).join(' · ')} image={isDemo ? '' : `${V1}/photos/${ph.id}/thumb`} /></span>
    </div>)}</div>
    <div className="row mt-sm">{p.items.length > 12 && <button className="btn ghost sm" onClick={() => setAll(a => !a)}>{all ? 'Show fewer' : `All ${p.items.length}`}</button>}{link && <a className="btn ghost sm" href={`${link}/photos?at=${date}`} target="_blank" rel="noreferrer">Open in Immich ↗</a>}</div>
    {big && <div className="modal-backdrop" onMouseDown={() => setBig(null)}><img className="lightbox" src={`${V1}/photos/${big}/thumb?size=preview`} alt="" /></div>}
  </>
}

/** Small weather chip for the day header. */
export function WeatherChip({ fact }: { fact?: Fact }) {
  if (!fact || !fact.payload) return null
  const w = fact.payload as WeatherDay
  return <span className="status-pill" title={w.place}>{WEATHER_ICON[w.condition] || '🌡'} {w.t_min !== null && w.t_max !== null ? `${Math.round(w.t_min)}–${Math.round(w.t_max)} °C` : cap(w.condition)}</span>
}

/** Whether a source recorded anything worth a card on this day. */
export function factHasContent(source: string, fact?: Fact): boolean {
  if (!fact) return false
  const v = fact.payload as Record<string, unknown> | null
  if (!v) return !!fact.error
  if (isActivity(source)) return Number((v as { count?: number }).count || 0) > 0
  const n = (k: string) => Number((v as Record<string, unknown>)[k] || 0)
  const len = (k: string) => ((v as Record<string, unknown[]>)[k] || []).length
  switch (source) {
    case 'dawarich': return n('distance_km') > 0 || len('entries') > 0
    case 'immich': return n('count') > 0
    case 'maloja': return n('listens') > 0
    case 'jellystat': return n('count') > 0
    case 'firefly': return n('count') > 0
    case 'habits': return n('total') > 0 || !!(v as { journal?: string }).journal || len('trackers') > 0
    case 'bookshelf': return len('reading') + len('started') + len('finished') > 0
    case 'travel': return len('trips') > 0
    case 'kitchen': return len('cooks') > 0
    case 'garage': return len('items') > 0
  }
  return true
}

type Tone = 'good' | 'mid' | 'bad' | ''

/** Where a tracker value sits: good, middling or bad, given its range and direction. */
function trackerTone(t: TrackerRow): Tone {
  if (t.value === null || t.higher_is_better === undefined) return ''
  if (t.kind === 'yesno') return (t.value > 0) === t.higher_is_better ? 'good' : 'bad'
  if (t.min === undefined || t.max === undefined || t.max <= t.min) return ''
  let f = (t.value - t.min) / (t.max - t.min)
  if (!t.higher_is_better) f = 1 - f
  return f >= 0.7 ? 'good' : f >= 0.4 ? 'mid' : 'bad'
}

function Trend({ t }: { t: TrackerRow }) {
  if (t.kind === 'yesno' || t.value === null || t.previous === undefined || t.previous === null || t.previous === t.value) return null
  const up = t.value > t.previous
  const better = t.higher_is_better === undefined ? undefined : up === t.higher_is_better
  return <span className={`trend ${better === undefined ? '' : better ? 'good' : 'bad'}`} title={`Day before: ${t.previous}`}>{up ? '▲' : '▼'} {Math.abs(t.value - t.previous)}</span>
}

function TrackerValue({ t }: { t: TrackerRow }) {
  const tone = trackerTone(t)
  if (t.value === null) return null
  if (t.kind === 'yesno') return <span className={`yn ${tone}`}>{t.value > 0 ? 'Yes' : 'No'}</span>
  if (t.kind === 'scale' && t.min !== undefined && t.max !== undefined && t.max - t.min <= 9) {
    const steps = Array.from({ length: t.max - t.min + 1 }, (_, i) => t.min! + i)
    return <span className={`scale ${tone}`} title={`${t.value} of ${t.max}`}>{steps.map(v => <i key={v} className={v <= t.value! ? 'on' : ''} />)}<b>{t.value}<span className="muted">/{t.max}</span></b></span>
  }
  return <span className={`num ${tone}`}><b>{t.value}</b>{t.unit && <span className="muted"> {t.unit}</span>}</span>
}

function HabitsBody({ c }: { c: Checkin }) {
  const sched = (c.habits || []).filter(h => h.scheduled)
  const done = sched.filter(h => h.status === 1), missed = sched.filter(h => h.status === -1), open = sched.filter(h => h.status === 0)
  const ordered = [...done, ...missed, ...open]
  const trackers = (c.trackers || []).filter(t => t.value !== null)
  const entered = trackers.filter(t => !t.defaulted), defaulted = trackers.filter(t => t.defaulted)
  const pct = (n: number) => `${sched.length ? (n / sched.length) * 100 : 0}%`
  return <>
    {sched.length > 0 && <>
      <div className="habit-summary"><b>{done.length}</b><span className="muted"> of {sched.length} done</span>{missed.length > 0 && <span className="hs-missed"> · {missed.length} missed</span>}{open.length > 0 && <span className="muted"> · {open.length} not marked</span>}</div>
      <div className="habit-bar" aria-hidden><i className="d" style={{ width: pct(done.length) }} /><i className="m" style={{ width: pct(missed.length) }} /></div>
      <div className="habit-list">{ordered.map(h => <div key={h.id} className={`habit-row s${h.status}`}>
        <span className="hmark">{h.status === 1 ? '✓' : h.status === -1 ? '✕' : ''}</span>
        <span className="hname">{h.name}</span>
        {h.streak > 1 && <span className="hstreak" title={`${h.streak} days in a row`}>🔥{h.streak}</span>}
      </div>)}</div>
    </>}
    {trackers.length > 0 && <>
      <div className="sub-head">Check-in</div>
      <div className="tracker-list">
        {entered.map(t => <div key={t.id} className="tracker-row"><span className="tname">{t.name}</span><TrackerValue t={t} /><Trend t={t} /></div>)}
        {defaulted.length > 0 && <div className="tracker-default muted small">Not entered, default: {defaulted.map(t => `${t.name} ${t.kind === 'yesno' ? (t.value ? 'yes' : 'no') : t.value}`).join(' · ')}</div>}
      </div>
    </>}
    {c.journal && <p className="quote mt-sm">“{c.journal}”</p>}
    {c.journal_bad && <p className="muted small" style={{ margin: '6px 0 0' }}>Bad: {c.journal_bad}</p>}
    {c.important && <p className="callout warning small">{c.important}</p>}
  </>
}

/** Any activity source: a headline, the top groups, the items with their time. */
function ActivityBody({ source, a, link, PinBtn }: { source: string; a: Activity; link?: string; PinBtn: (props: PinBtnProps) => React.ReactNode }) {
  const m = ACTIVITY[source]
  const cur = a.currency || 'EUR'
  const headline = m.unit === 'min' ? fmtHeadline(source, a.minutes) : m.unit === 'qty' ? fmtHeadline(source, a.qty || 0) : m.unit === 'amount' ? fmtHeadline(source, a.amount || 0, cur) : fmtHeadline(source, a.count)
  const second = m.unit === 'count' ? '' : `${a.count} ${a.count === 1 ? m.one : m.many}`
  const [all, setAll] = useState(false)
  const items = all ? a.items : a.items.slice(0, 8)
  const base = link ? link.replace(/\/+$/, '') : ''
  return <>
    <div className="fact-head"><b>{headline}</b>{second && <span className="muted"> · {second}</span>}{a.done > 0 && m.doneLabel && <span className="muted"> · {a.done} {m.doneLabel}</span>}</div>
    {a.top?.length > 1 && <div className="chips">{a.top.slice(0, 5).map(t => <span key={t.name} className="chip">{t.name} <span className="muted">{t.count}</span></span>)}</div>}
    <div className="inline-list mt-sm">{items.map((it, i) => {
      const url = it.url ? (it.url.startsWith('/') ? (base ? base + it.url : '') : it.url) : ''
      return <div key={(it.ref || it.title) + i} className="li act-li">
        <span className="muted mono small act-t">{it.time || ''}</span>
        {it.image ? <img className="act-img" src={it.image} alt="" loading="lazy" /> : null}
        <span className="grow act-body"><b>{it.done && <span className="act-done" title={m.doneLabel || 'done'}>✓ </span>}{url ? <a href={url} target="_blank" rel="noreferrer" style={{ color: 'inherit' }}>{it.title || '(untitled)'}</a> : (it.title || '(untitled)')}</b>{it.sub && <span className="act-sub">{it.sub}</span>}</span>
        {(it.minutes || 0) >= 1 ? <span className="muted small nowrap">{fmtMinutes(Math.round(it.minutes || 0))}</span> : it.amount ? <span className="money">{fmtAmount(it.amount, it.currency || cur)}</span> : (it.qty || 0) > 0 ? <span className="muted small nowrap">{it.qty} {it.qty === 1 ? m.qtyOne : m.qtyMany}</span> : null}
        <PinBtn ref_={`${source}:${it.ref || it.title}`} title={it.title} subtitle={it.sub} image={it.image} />
      </div>
    })}</div>
    {a.items.length > 8 && <button className="btn ghost sm mt-sm" onClick={() => setAll(x => !x)}>{all ? 'Show fewer' : `All ${a.items.length}`}</button>}
  </>
}
