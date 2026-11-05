import React, { useState } from 'react'
import { V1 } from '../lib/api'
import { locationTime } from '../lib/locationTime'
import { fmtDate } from '../lib/format'
import { Card } from './ui'
import { Icon, SourceIcon } from './icons'
import {
  SOURCE_APP, SOURCE_LABEL, WEATHER_ICON, cap, fmtAmount, fmtKm, fmtMinutes,
  type Activity, type Car, type HomeDay, type RenewalsDay, type WealthDay, type PetsDay, type ParcelsDay, type SignDay, type PlacesDay, type VoiceDay, type VoiceMemo, type WalletDay, type Checkin, type TrackerRow, ACTIVITY, isActivity, fmtHeadline, type Cooked, type Fact, type Money, type Music, type Photos, type Pin, type Reading, type Trips, type Watched, type WeatherDay, type Where, type HealthDay, type StillDay, type HoroscopeDay,
} from '../lib/types'

export interface PinFn { (p: Omit<Pin, 'id' | 'user_id' | 'date' | 'created_at'>): void }
type PinBtnProps = { ref_: string; title: string; subtitle?: string; image?: string }

interface Props { source: string; fact?: Fact; link?: string; unit: 'km' | 'mi'; pins: Pin[]; onPin: PinFn; onUnpin: (id: number) => void; date: string }

/** One source's card on the day page. */
export function FactCard({ source, fact, link, unit, pins, onPin, onUnpin, date }: Props) {
  const title = <><SourceIcon source={source} size={22} />{SOURCE_LABEL[source] || cap(source)}</>
  // the location, requests and watched sources name where the day came from (Pocket Movement or Dawarich, Pocket Requests or Jellyseerr, Pocket Watch or Jellystat)
  const app = source === 'dawarich' && (fact?.payload as Where | null)?.provider === 'movement' ? 'Pocket Movement'
    : source === 'jellyseerr' && (fact?.payload as Activity | null)?.provider === 'requests' ? 'Pocket Requests'
    : source === 'jellystat' && (fact?.payload as Watched | null)?.provider === 'watch' ? 'Pocket Watch'
    : SOURCE_APP[source] || source
  const actions = <>
    {fact?.error && <span className="badge warn" title={fact.error}>stale</span>}
    {link && <a className="btn ghost icon-only sm" href={link} target="_blank" rel="noreferrer" title={`Open ${app}`} aria-label={`Open ${app}`}><Icon name="external" size={14} /></a>}
  </>
  const empty = (msg: string) => <p className="muted small" style={{ margin: 0 }}>{msg}</p>
  if (!fact || fact.payload === null || fact.payload === undefined) {
    return <Card title={title} actions={actions} className="fact-card">{empty(fact?.error ? `Couldn't reach ${app}: ${fact.error}` : `Nothing from ${app} for this day.`)}</Card>
  }
  const pinned = (ref: string) => pins.find(p => p.source === source && p.ref === ref)
  const PinBtn = ({ ref_, title, subtitle, image }: PinBtnProps) => {
    const p = pinned(ref_)
    return <button type="button" className={`pin-btn ${p ? 'on' : ''}`} title={p ? 'Unpin from this day' : 'Keep on this day'} aria-label={p ? 'Unpin from this day' : 'Keep on this day'} aria-pressed={!!p} onClick={() => p ? onUnpin(p.id) : onPin({ source, ref: ref_, title, subtitle: subtitle || '', image: image || '' })}><Icon name="star" size={13} strokeWidth={2} /></button>
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
        {modes.length > 0 && <div className="chips">{modes.map(([m, v]) => <span key={m} className="chip" title={m}><Icon name={modeIcon(m)} size={14} />{fmtKm(v, unit)}</span>)}</div>}
        {visits.length > 0 && <div className="inline-list mt-sm">{visits.map((v, i) => {
          const timing = locationTime(v, date)
          return <div key={i} className="li"><span className="muted mono small">{timing.time}</span><span className="grow act-body">{v.name || 'Somewhere'}{timing.crossesDay && <span className="tl-sub">{timing.range}</span>}</span><span className="muted small">{fmtMinutes(v.minutes)}{timing.crossesDay ? ' total' : ''}</span><PinBtn ref_={`visit:${v.start}:${v.name}`} title={v.name || 'Somewhere'} subtitle={`${timing.start}–${timing.end}`} /></div>
        })}</div>}
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
      if (!r.finished?.length && !r.started?.length && !r.reading?.length && !r.sessions?.length) { body = empty('Nothing on the go.'); break }
      const row = (b: Reading['reading'][number], label: string) => <div key={label + b.id} className="li"><span className="badge cat">{label}</span><span className="grow"><b>{b.title}</b>{b.authors && <span className="muted"> · {b.authors}</span>}</span>{b.percent ? <span className="muted small">{b.percent}%</span> : null}{b.rating ? <span className="stars">{'★'.repeat(Math.round(b.rating))}</span> : null}<PinBtn ref_={`book:${b.id}`} title={b.title} subtitle={b.authors} /></div>
      const read = (x: NonNullable<Reading['sessions']>[number], i: number) => <div key={'s' + i} className="li"><span className="muted small tnum">{x.time}–{x.end}</span><span className="grow"><b>{x.pages ? `${x.pages} pages` : `${x.percent || 1}%`}</b> in <b>{x.estimated ? '~' : ''}{fmtMinutes(x.minutes)}</b><span className="muted"> · {x.title}</span>{x.to_page ? <span className="muted small"> · p. {x.from_page}–{x.to_page}</span> : null}</span></div>
      body = <>
        {r.sessions?.length ? <div className="inline-list" style={{ marginBottom: 8 }}>{r.sessions.map(read)}{r.sessions.length > 1 && <div className="muted small">{r.pages ? `${r.pages} pages in ` : ''}{fmtMinutes(r.minutes || 0)} altogether</div>}</div> : null}
        <div className="inline-list">{(r.finished || []).map(b => row(b, 'finished'))}{(r.started || []).map(b => row(b, 'started'))}{(r.reading || []).map(b => row(b, 'reading'))}</div>
      </>
      break
    }
    case 'travel': {
      const t = fact.payload as Trips
      if (!t.trips?.length) { body = empty('Not travelling.'); break }
      body = <div className="inline-list">{t.trips.map(tr => <div key={tr.id} className="li"><span className="swatch" style={{ background: tr.color || 'var(--accent)' }} /><span className="grow"><b>{tr.name}</b>{tr.destination && <span className="muted"> · {tr.destination}</span>}</span><span className="badge">day {tr.day_no} of {tr.days}</span>{link && <a className="btn ghost icon-only sm" href={`${link}/trips/${tr.id}`} target="_blank" rel="noreferrer" title="Open the trip" aria-label="Open the trip"><Icon name="external" size={14} /></a>}</div>)}</div>
      break
    }
    case 'kitchen': {
      const c = fact.payload as Cooked
      if (!c.cooks?.length) { body = empty('Nothing cooked.'); break }
      body = <div className="inline-list">{c.cooks.map(ck => <div key={ck.id} className="li"><Icon name="utensils" size={15} className="muted" /><span className="grow"><b>{ck.title}</b>{ck.servings ? <span className="muted"> · {ck.servings} servings</span> : null}{ck.by && <span className="muted small"> · {ck.by}</span>}</span>{ck.rating > 0 && <span className="stars">{'★'.repeat(ck.rating)}</span>}<PinBtn ref_={`cook:${ck.id}`} title={ck.title} subtitle="cooked" /></div>)}</div>
      break
    }
    case 'health': {
      const h = fact.payload as HealthDay
      const chips: string[] = []
      if (h.steps !== undefined) chips.push(`${Math.round(h.steps).toLocaleString()} steps`)
      if (h.distance_km !== undefined) chips.push(`${h.distance_km.toFixed(1)} km`)
      if (h.active_kcal !== undefined) chips.push(`${Math.round(h.active_kcal)} kcal`)
      if (h.exercise_min !== undefined) chips.push(`${Math.round(h.exercise_min)} min exercise`)
      if (h.resting_hr !== undefined) chips.push(`resting HR ${Math.round(h.resting_hr)}`)
      if (h.hrv !== undefined) chips.push(`HRV ${Math.round(h.hrv)} ms`)
      if (h.weight_kg !== undefined) chips.push(`${h.weight_kg.toFixed(1)} kg`)
      if (!chips.length && !h.sleep && !h.workouts?.length) { body = empty('Nothing from the phone this day.'); break }
      body = <>
        {chips.length > 0 && <div className="fact-head">{chips.map(c => <span key={c} className="badge">{c}</span>)}</div>}
        {h.sleep && <div className="li"><Icon name="moon" size={15} className="muted" /><span className="grow"><b>Slept {fmtMinutes(h.sleep.asleep_min)}</b><span className="muted"> · {h.sleep.bed} → {h.sleep.wake}{h.sleep.deep_min ? ` · deep ${fmtMinutes(h.sleep.deep_min)}` : ''}{h.sleep.rem_min ? ` · REM ${fmtMinutes(h.sleep.rem_min)}` : ''}</span></span></div>}
        {h.workouts?.length ? <div className="inline-list">{h.workouts.map(w => <div key={w.id} className="li"><Icon name="activity" size={15} className="muted" /><span className="grow"><b>{w.name || w.kind.replace('_', ' ')}</b><span className="muted"> · {w.time} · {fmtMinutes(w.minutes)}{w.distance_km ? ` · ${w.distance_km} km` : ''}{w.kcal ? ` · ${w.kcal} kcal` : ''}{w.avg_hr ? ` · ♥ ${w.avg_hr}` : ''}</span></span>{link && <a className="btn ghost icon-only sm" href={`${link}/workouts/${w.id}`} target="_blank" rel="noreferrer" title="Open in Pocket Health" aria-label="Open in Pocket Health"><Icon name="external" size={14} /></a>}<PinBtn ref_={`workout:${w.id}`} title={w.name || w.kind} subtitle="workout" /></div>)}</div> : null}
        {h.goals_met?.length ? <div className="muted small" style={{ marginTop: 6 }}>Goals met: {h.goals_met.map(g => g.replace('_', ' ')).join(', ')}</div> : null}
      </>
      break
    }
    case 'horoscope': {
      const h = fact.payload as HoroscopeDay
      if (!h.moon_sign) { body = empty('No sky for this day.'); break }
      const phase: Record<string, string> = { new: '🌑 New Moon', waxing_crescent: '🌒 Waxing crescent', first_quarter: '🌓 First quarter', waxing_gibbous: '🌔 Waxing gibbous', full: '🌕 Full Moon', waning_gibbous: '🌖 Waning gibbous', last_quarter: '🌗 Last quarter', waning_crescent: '🌘 Waning crescent' }
      const sign = h.moon_sign[0].toUpperCase() + h.moon_sign.slice(1)
      const chips = [`${phase[h.phase] || 'Moon'} in ${sign}`, `${Math.round(h.illumination * 100)}% lit`]
      if (h.overall) chips.push(`your day ${h.overall}/100`)
      const faces = ['', '😞', '🙁', '😐', '🙂', '😄']
      body = <>
        <div className="fact-head">{chips.map(c => <span key={c} className="badge">{c}</span>)}</div>
        {h.headline && <div className="li"><Icon name="moon" size={15} className="muted" /><span className="grow"><b>{h.headline}</b>{h.scores && Object.keys(h.scores).length > 0 && <span className="muted"> · {['love', 'work', 'mood', 'energy'].filter(k => h.scores[k] !== undefined).map(k => `${k} ${h.scores[k]}`).join(' · ')}</span>}</span>{h.url && <a className="btn ghost icon-only sm" href={h.url} target="_blank" rel="noreferrer" title="Open in Pocket Horoscope" aria-label="Open in Pocket Horoscope"><Icon name="external" size={14} /></a>}</div>}
        {h.events?.length ? <div className="inline-list">{h.events.map((e, i) => <div key={i} className="li"><span className="muted" style={{ width: 15, textAlign: 'center' }}>{e.icon || '✦'}</span><span className="grow"><b>{e.title}</b>{e.time && <span className="muted"> · {e.time}</span>}</span></div>)}</div> : null}
        {h.transits?.length ? <div className="muted small" style={{ marginTop: 6 }}>Your transits: {h.transits.slice(0, 4).join(', ')}</div> : null}
        {h.checkin && (h.checkin.mood > 0 || h.checkin.energy > 0 || h.checkin.note) ? <div className="muted small" style={{ marginTop: 4 }}>Check-in: {h.checkin.mood > 0 ? `${faces[h.checkin.mood]} mood ${h.checkin.mood}/5` : ''}{h.checkin.energy > 0 ? ` · energy ${h.checkin.energy}/5` : ''}{h.checkin.note ? ` · “${h.checkin.note}”` : ''}</div> : null}
      </>
      break
    }
    case 'still': {
      const s = fact.payload as StillDay
      if (!s.practices?.length) { body = empty('No practice this day.'); break }
      const chips = [`${Math.round(s.minutes)} min`, `${s.sessions} ${s.sessions === 1 ? 'practice' : 'practices'}`]
      if (s.streak > 1) chips.push(`day ${s.streak} of a streak`)
      if (s.goal_met) chips.push('goal met')
      body = <>
        <div className="fact-head">{chips.map(c => <span key={c} className="badge">{c}</span>)}</div>
        <div className="inline-list">{s.practices.map(x => <div key={x.id} className="li"><Icon name="sparkles" size={15} className="muted" /><span className="grow"><b>{x.title}</b><span className="muted"> · {x.time} · {fmtMinutes(Math.round(x.minutes))}{x.calm_before && x.calm_after ? ` · calm ${x.calm_before} → ${x.calm_after}` : ''}</span>{x.lesson && <div className="muted small">{x.lesson}</div>}</span>{link && <a className="btn ghost icon-only sm" href={`${link}/journal/${x.id}`} target="_blank" rel="noreferrer" title="Open in Pocket Still" aria-label="Open in Pocket Still"><Icon name="external" size={14} /></a>}</div>)}</div>
        {s.lessons?.length ? <div className="muted small" style={{ marginTop: 6 }}>Lessons: {s.lessons.map(l => `${l.title} (${l.program})`).join(', ')}</div> : null}
        {s.milestones?.length ? <div className="muted small" style={{ marginTop: 4 }}>Milestones: {s.milestones.join(', ')}</div> : null}
      </>
      break
    }
    case 'home': {
      const h = fact.payload as HomeDay
      if (!h.work?.length && !h.readings?.length) { body = empty('Nothing done on the house.'); break }
      body = <div className="inline-list">
        {(h.work || []).map(w => <div key={`w${w.id}`} className="li"><Icon name="wrench" size={15} className="muted" /><span className="grow"><b>{w.title}</b><span className="muted">{[w.thing, w.pro].filter(Boolean).map(x => ` · ${x}`).join('')}</span></span>{w.cost ? <span className="money">{fmtAmount(w.cost / 100, w.currency)}</span> : null}{link && <a className="btn ghost icon-only sm" href={`${link}/log/${w.id}`} target="_blank" rel="noreferrer" title="Open in Pocket Home" aria-label="Open in Pocket Home"><Icon name="external" size={14} /></a>}</div>)}
        {(h.readings || []).map(r => <div key={`r${r.meter_id}`} className="li"><Icon name="activity" size={15} className="muted" /><span className="grow"><b>{r.meter}</b><span className="muted"> · {r.value.toLocaleString()} {r.unit}{r.usage != null ? ` · +${Math.round(r.usage).toLocaleString()} ${r.unit} in ${r.days} days` : ''}</span></span></div>)}
      </div>
      break
    }
    case 'wealth': {
      const wd = fact.payload as WealthDay
      if (!wd.items?.length) { body = empty('Nothing bought, sold or updated.'); break }
      body = <div className="inline-list">
        {wd.items.map(it => <div key={`${it.type}${it.id}`} className="li"><Icon name={it.type === 'activity' ? 'trending-up' : 'edit'} size={15} className="muted" /><span className="grow"><b>{it.title}</b>{it.asset && !it.title.includes(it.asset) && <span className="muted"> · {it.asset}</span>}</span>{it.type === 'activity' && it.amount ? <span className="money">{fmtAmount(it.amount / 100, it.currency)}</span> : null}{(link || it.url) && <a className="btn ghost icon-only sm" href={link ? `${link}/a/${it.asset_id}` : it.url} target="_blank" rel="noreferrer" title="Open in Pocket Wealth" aria-label="Open in Pocket Wealth"><Icon name="external" size={14} /></a>}</div>)}
      </div>
      break
    }
    case 'pets': {
      const pd = fact.payload as PetsDay
      if (!pd.entries?.length && !pd.weights?.length && !pd.birthdays?.length) { body = empty('Nothing with the pets.'); break }
      const go = (path: string) => link && <a className="btn ghost icon-only sm" href={`${link}${path}`} target="_blank" rel="noreferrer" title="Open in Pocket Pets" aria-label="Open in Pocket Pets"><Icon name="external" size={14} /></a>
      body = <div className="inline-list">
        {(pd.birthdays || []).map(b => <div key={`b${b.pet_id}`} className="li"><Icon name="sparkles" size={15} className="muted" /><span className="grow"><b>{b.line}</b></span>{go(`/pets/${b.pet_id}`)}</div>)}
        {(pd.entries || []).map(e => <div key={`e${e.id}`} className="li"><Icon name="paw" size={15} className="muted" /><span className="grow"><b>{e.line}</b>{(e.notes || e.valid_until) && <span className="muted">{[e.valid_until ? `valid until ${fmtDate(e.valid_until)}` : '', e.notes || ''].filter(Boolean).map(x => ` · ${x}`).join('')}</span>}</span>{e.cost ? <span className="money">{fmtAmount(e.cost / 100, e.currency)}</span> : null}{go(`/log/${e.id}`)}</div>)}
        {(pd.weights || []).map(w => <div key={`w${w.id}`} className="li"><Icon name="activity" size={15} className="muted" /><span className="grow"><b>{w.line}</b></span>{go(`/pets/${w.pet_id}`)}</div>)}
      </div>
      break
    }
    case 'sign': {
      const sd = fact.payload as SignDay
      const rows = [...(sd.completed || []), ...(sd.signed_by_me || []), ...(sd.sent || []), ...(sd.signed || [])]
      if (!rows.length) { body = empty('Nothing signed today.'); break }
      const go = (it: SignDay['sent'][number]) => link && <a className="btn ghost icon-only sm" href={it.recipient_id ? `${link}/inbox/${it.recipient_id}` : `${link}/documents/${it.document_id}`} target="_blank" rel="noreferrer" title="Open in Pocket Sign" aria-label="Open in Pocket Sign"><Icon name="external" size={14} /></a>
      body = <div className="inline-list">
        {rows.map(it => <div key={`${it.document_id}${it.recipient_id || ''}${it.line}`} className="li"><Icon name="pen" size={15} className="muted" /><span className="grow"><b>{it.line}</b>{it.time && <span className="muted"> · {it.time}</span>}</span>{go(it)}</div>)}
      </div>
      break
    }
    case 'places': {
      const pl = fact.payload as PlacesDay
      if (!pl.visits?.length && !pl.planned?.length && !pl.saved?.length) { body = empty('No places today.'); break }
      const go = (id: number) => link && <a className="btn ghost icon-only sm" href={`${link}/places/${id}`} target="_blank" rel="noreferrer" title="Open in Pocket Places" aria-label="Open in Pocket Places"><Icon name="external" size={14} /></a>
      const food = (k: string) => ['restaurant', 'street_food', 'cafe', 'bar', 'bakery', 'dessert'].includes(k)
      const spent = (pl.spent || []).filter(s => s.total > 0 && s.visits > 1)
      body = <div className="inline-list">
        {(pl.visits || []).map(v => <div key={`v${v.id}`} className="li"><Icon name={food(v.kind) ? 'utensils' : 'pin'} size={15} className="muted" /><span className="grow"><b>{v.line}</b>{(v.time || v.city || v.trip || v.notes) && <span className="muted">{[v.time, v.city, v.trip, v.notes].filter(Boolean).map(x => ` · ${x}`).join('')}</span>}</span>{go(v.place_id)}</div>)}
        {(pl.planned || []).map(v => <div key={`p${v.id}`} className="li"><Icon name="calendar" size={15} className="muted" /><span className="grow"><span className="muted">{v.line}{[v.time, v.city].filter(Boolean).map(x => ` · ${x}`).join('')}</span></span>{go(v.place_id)}</div>)}
        {(pl.saved || []).map(sv => <div key={`s${sv.id}`} className="li"><Icon name="star" size={15} className="muted" /><span className="grow"><b>{sv.line}</b>{(sv.cuisine || sv.kind || sv.city) && <span className="muted">{[sv.cuisine || cap(sv.kind), sv.city].filter(Boolean).map(x => ` · ${x}`).join('')}</span>}</span>{go(sv.id)}</div>)}
        {spent.map(s => <div key={`t${s.currency}`} className="li"><Icon name="wallet" size={15} className="muted" /><span className="grow muted">Spent on {s.visits} visits</span><span className="money">{fmtAmount(s.total / 100, s.currency)}</span></div>)}
      </div>
      break
    }
    case 'wallet': {
      const wd = fact.payload as WalletDay
      if (!wd.passes?.length) { body = empty('No tickets or boarding passes for this day.'); break }
      const base = (link || wd.url || '').replace(/\/+$/, '')
      body = <div className="inline-list">
        {wd.passes.map(ps => <div key={ps.id} className="li"><Icon name={ps.kind === 'boarding' ? 'plane' : 'ticket'} size={15} className="muted" /><span className="grow"><b>{ps.line}</b>{(ps.time || ps.gate || ps.booking || ps.holder || ps.trip) && <span className="muted">{[ps.time && (ps.until ? `${ps.time}–${ps.until}` : ps.time), ps.gate && `gate ${ps.gate}`, ps.booking && `booking ${ps.booking}`, ps.holder, ps.trip].filter(Boolean).map(x => ` · ${x}`).join('')}</span>}</span>{base && <a className="btn ghost icon-only sm" href={`${base}${ps.path}`} target="_blank" rel="noreferrer" title="Open in Pocket Wallet" aria-label="Open in Pocket Wallet"><Icon name="external" size={14} /></a>}</div>)}
      </div>
      break
    }
    case 'voice': {
      const vd = fact.payload as VoiceDay
      if (!vd.memos?.length) { body = empty('No voice memos today.'); break }
      const base = link ? link.replace(/\/+$/, '') : ''
      // each memo links back to itself in Pocket Voice (its own address, or the app's when Voice gave a path)
      const href = (m: VoiceMemo) => m.url ? (m.url.startsWith('/') ? (base ? base + m.url : '') : m.url) : (base ? `${base}/memos/${m.id}` : '')
      body = <>
        {vd.memos.length > 1 && <div className="fact-head"><b>{vd.memos.length} voice memos</b>{vd.duration_ms > 0 && <span className="muted"> · {clockLength(vd.duration_ms)} in all</span>}</div>}
        <div className={`inline-list${vd.memos.length > 1 ? ' mt-sm' : ''}`}>{vd.memos.map(m => {
          const url = href(m)
          const journal = m.kind === 'journal'
          const title = m.title || (journal ? 'Voice journal' : m.summary || 'Voice memo')
          const said = journal ? (m.summary || m.text) : (m.summary !== title ? m.summary : '')
          const went = (m.sent || []).map(x => x.title).filter(Boolean).join(' · ')
          return <div key={m.id} className="li act-li">
            <span className="muted mono small act-t">{m.time || ''}</span>
            <span className="grow act-body"><b>{url ? <a href={url} target="_blank" rel="noreferrer" style={{ color: 'inherit' }} title="Open in Pocket Voice">{title}</a> : title}</b>
              {said && <span className="act-note">{said}</span>}
              {journal && m.summary && m.text && <span className="act-note muted">“{m.text}”</span>}
              {(m.kind || m.went) && <span className="act-sub" title={went || undefined}>{[m.kind ? VOICE_KIND[m.kind] || cap(m.kind) : '', m.went ? `→ ${m.went}` : ''].filter(Boolean).join(' · ')}</span>}
            </span>
            {m.length && <span className="muted small nowrap">{m.length}</span>}
          </div>
        })}</div>
      </>
      break
    }
    case 'parcels': {
      const pd = fact.payload as ParcelsDay
      const rows = [...(pd.delivered || []), ...(pd.sent || []), ...(pd.ordered || [])]
      if (!rows.length && !pd.steps?.length && !pd.expected?.length) { body = empty('No parcels today.'); break }
      const go = (id: number) => link && <a className="btn ghost icon-only sm" href={`${link}/parcels/${id}`} target="_blank" rel="noreferrer" title="Open in Pocket Parcels" aria-label="Open in Pocket Parcels"><Icon name="external" size={14} /></a>
      body = <div className="inline-list">
        {rows.map(it => <div key={`${it.id}${it.line}`} className="li"><Icon name="package" size={15} className="muted" /><span className="grow"><b>{it.line}</b>{(it.time || it.carrier) && <span className="muted">{[it.time, it.carrier, it.days != null && it.line.startsWith('Delivered') ? `${it.days} ${it.days === 1 ? 'day' : 'days'} on the way` : ''].filter(Boolean).map(x => ` · ${x}`).join('')}</span>}</span>{it.price != null && it.currency && it.line.startsWith('Delivered') ? <span className="money">{fmtAmount(it.price / 100, it.currency)}</span> : null}{go(it.id)}</div>)}
        {(pd.steps || []).map(s => <div key={`s${s.parcel_id}${s.status}`} className="li"><Icon name={s.status === 'pickup' ? 'pin' : 'alert'} size={15} className="muted" /><span className="grow"><b>{s.line}</b>{s.time && <span className="muted"> · {s.time}</span>}</span>{go(s.parcel_id)}</div>)}
        {(pd.expected || []).map(it => <div key={`x${it.id}`} className="li"><Icon name="clock" size={15} className="muted" /><span className="grow"><span className="muted">{it.line}</span></span>{go(it.id)}</div>)}
      </div>
      break
    }
    case 'renewals': {
      const r = fact.payload as RenewalsDay
      if (!r.renewed?.length && !r.expired?.length) { body = empty('Nothing renewed, nothing ran out.'); break }
      const open = (id: number, url?: string) => link ? `${link}/r/${id}` : url
      const go = (href?: string) => href && <a className="btn ghost icon-only sm" href={href} target="_blank" rel="noreferrer" title="Open in Pocket Renewals" aria-label="Open in Pocket Renewals"><Icon name="external" size={14} /></a>
      body = <div className="inline-list">
        {(r.renewed || []).map(h => <div key={`n${h.id}`} className="li"><Icon name="refresh" size={15} className="muted" /><span className="grow"><b>Renewed: {h.title}</b><span className="muted">{[h.new_expires ? `now until ${fmtDate(h.new_expires)}` : '', h.auto ? 'renewed by itself' : '', h.note || ''].filter(Boolean).map(x => ` · ${x}`).join('')}</span></span>{h.cost ? <span className="money">{fmtAmount(h.cost / 100, h.currency)}</span> : null}{go(open(h.renewal_id))}</div>)}
        {(r.expired || []).map(e => <div key={`x${e.id}`} className="li"><Icon name="hourglass" size={15} className="muted" /><span className="grow"><b>Ran out: {e.title}</b><span className="muted">{[e.holder, e.provider].filter(Boolean).map(x => ` · ${x}`).join('')}</span></span>{go(open(e.id, e.url))}</div>)}
      </div>
      break
    }
    case 'garage': {
      const c = fact.payload as Car
      if (!c.items?.length) { body = empty('Nothing in the garage.'); break }
      body = <div className="inline-list">{c.items.map((it, i) => <div key={i} className="li"><Icon name={it.kind === 'fuel' ? 'fuel' : 'wrench'} size={15} className="muted" /><span className="grow"><b>{it.vehicle}</b> <span className="muted">{it.kind === 'fuel' ? it.title : `${it.kind} · ${it.title}`}{it.station ? ` · ${it.station}` : ''}</span></span>{it.cost ? <span className="money">{fmtAmount(it.cost / 100, it.currency)}</span> : null}</div>)}</div>
      break
    }
    case 'weather': {
      const w = fact.payload as WeatherDay
      body = <>
        <div className="fact-head"><span style={{ fontSize: 22 }}>{WEATHER_ICON[w.condition] || '🌡'}</span> <b>{cap(w.condition || 'weather')}</b>{w.t_min !== null && w.t_max !== null && <span className="muted"> · {Math.round(w.t_min)}–{Math.round(w.t_max)} °C</span>}{w.precip_mm ? <span className="muted"> · {w.precip_mm} mm</span> : null}{w.sunshine_h ? <span className="muted"> · {w.sunshine_h} h sun</span> : null}{w.place && <span className="muted"> · {w.place}</span>}</div>
        {(w.warnings?.length || w.note) ? <div className="inline-list">
          {(w.warnings || []).map((x, i) => <div key={`w${i}`} className="li"><span style={{ display: 'inline-flex', color: x.level === 'warning' ? 'var(--warning)' : 'var(--muted)' }}><Icon name="alert" size={15} /></span><span className="grow"><b>{x.title}</b><span className="muted">{[x.level === 'warning' ? 'Warning' : 'Advisory', x.from && x.until ? `${x.from}–${x.until}` : x.from ? `from ${x.from}` : x.until ? `until ${x.until}` : '', x.place || '', x.detail || ''].filter(Boolean).map(s => ` · ${s}`).join('')}</span></span></div>)}
          {w.note && <div className="li"><Icon name="edit" size={15} className="muted" /><span className="grow"><span className="muted">Your note in Pocket Weather: </span>{w.note}</span></div>}
        </div> : null}
      </>
      break
    }
    default:
      if (isActivity(source)) {
        const a = fact.payload as Activity
        if (!a.count) { body = empty(`Nothing in ${app} this day.`); break }
        body = <ActivityBody source={source} a={a} link={link} PinBtn={PinBtn} />
        break
      }
      body = <pre className="small">{JSON.stringify(fact.payload, null, 1).slice(0, 800)}</pre>
  }
  return <Card title={title} actions={actions} className={`fact-card src-${source}`}>{body}</Card>
}

function modeIcon(m: string) {
  switch (m) {
    case 'walking': case 'running': return 'walk'
    case 'cycling': return 'bike'
    case 'driving': return 'car'
    case 'train': return 'train'
    case 'flying': case 'plane': return 'plane'
    case 'boat': return 'boat'
  }
  return 'route'
}

export function Expandable({ label, children, open: init = false }: { label: string; children: React.ReactNode; open?: boolean }) {
  const [open, setOpen] = useState(init)
  return <div className="mt-sm"><button type="button" className="btn ghost sm" onClick={() => setOpen(o => !o)} aria-expanded={open}><Icon name={open ? 'chevron-down' : 'chevron-right'} size={14} />{label}</button>{open && children}</div>
}

function PhotoGrid({ p, date, link, PinBtn }: { p: Photos; date: string; link?: string; PinBtn: (props: PinBtnProps) => React.ReactNode }) {
  const [all, setAll] = useState(false)
  const [big, setBig] = useState<string | null>(null)
  const items = all ? p.items : p.items.slice(0, 12)
  const isDemo = p.items[0]?.id?.startsWith('demo-')
  return <>
    <div className="fact-head"><b>{p.count - p.videos} photo{p.count - p.videos === 1 ? '' : 's'}</b>{p.videos > 0 && <span className="muted"> · {p.videos} video{p.videos === 1 ? '' : 's'}</span>}{p.favorites > 0 && <span className="muted"> · {p.favorites} favourite{p.favorites === 1 ? '' : 's'}</span>}{p.places?.length > 0 && <span className="muted"> · {p.places.join(', ')}</span>}{p.first && <span className="muted"> · {p.first}–{p.last}</span>}</div>
    <div className="photo-grid">{items.map(ph => <div key={ph.id} className="ph">
      {isDemo ? <div className="ph-img demo" /> : <img className="ph-img" src={`${V1}/photos/${ph.id}/thumb`} alt={ph.file_name || ''} loading="lazy" onClick={() => setBig(ph.id)} />}
      <span className="ph-time">{ph.time}{ph.type === 'VIDEO' ? ' ▶' : ''}</span>
      <span className="ph-pin"><PinBtn ref_={`asset:${ph.id}`} title={ph.file_name || `Photo at ${ph.time}`} subtitle={[ph.time, ph.city].filter(Boolean).join(' · ')} image={isDemo ? '' : `${V1}/photos/${ph.id}/thumb`} /></span>
    </div>)}</div>
    <div className="row mt-sm">{p.items.length > 12 && <button type="button" className="btn ghost sm" onClick={() => setAll(a => !a)}>{all ? 'Show fewer' : `All ${p.items.length}`}</button>}{link && <a className="btn ghost sm" href={`${link}/photos?at=${date}`} target="_blank" rel="noreferrer">Open in Immich<Icon name="external" size={13} /></a>}</div>
    {big && <div className="modal-backdrop lightbox-backdrop" onMouseDown={() => setBig(null)}><img className="lightbox" src={`${V1}/photos/${big}/thumb?size=preview`} alt="" /></div>}
  </>
}

/** Small weather chip for the day header. */
export function WeatherChip({ fact }: { fact?: Fact }) {
  if (!fact || !fact.payload) return null
  const w = fact.payload as WeatherDay
  const warn = w.warnings?.length ? w.warnings.map(x => x.title).join(', ') : ''
  return <>
    <span className="status-pill" title={[w.place, w.note].filter(Boolean).join(' · ')}>{WEATHER_ICON[w.condition] || '🌡'} {w.t_min !== null && w.t_max !== null ? `${Math.round(w.t_min)}–${Math.round(w.t_max)} °C` : cap(w.condition)}</span>
    {warn && <span className="status-pill" title={w.warnings!.map(x => [x.title, x.detail].filter(Boolean).join(': ')).join('\n')}><Icon name="alert" size={13} />{warn}</span>}
  </>
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
    case 'bookshelf': return len('reading') + len('started') + len('finished') + len('sessions') > 0
    case 'travel': return len('trips') > 0
    case 'kitchen': return len('cooks') > 0
    case 'health': return n('steps') > 0 || len('workouts') > 0 || !!(v as { sleep?: unknown }).sleep
    case 'still': return len('practices') > 0
    case 'home': return len('work') + len('readings') > 0
    case 'renewals': return len('renewed') + len('expired') > 0
    case 'wealth': return len('items') > 0
    case 'pets': return len('entries') + len('weights') + len('birthdays') > 0
    case 'sign': return len('completed') + len('signed_by_me') + len('sent') + len('signed') > 0
    case 'places': return len('visits') + len('planned') + len('saved') > 0
    case 'voice': return len('memos') > 0
    case 'wallet': return len('passes') > 0
    case 'parcels': return len('delivered') + len('sent') + len('ordered') + len('steps') > 0
    case 'horoscope': return !!(v as { moon_sign?: string }).moon_sign
    case 'garage': return len('items') > 0
  }
  return true
}

/** What a voice memo is about, as Pocket Voice names it. */
const VOICE_KIND: Record<string, string> = { note: 'Note', todo: 'To-do', idea: 'Idea', journal: 'Journal', meeting: 'Meeting', shopping: 'Shopping', reminder: 'Reminder' }

/** A recording's length: "0:21", "1:02:03". */
function clockLength(ms: number): string {
  const s = Math.round(ms / 1000)
  const mm = String(Math.floor(s % 3600 / 60)), ss = String(s % 60).padStart(2, '0')
  return s >= 3600 ? `${Math.floor(s / 3600)}:${mm.padStart(2, '0')}:${ss}` : `${mm}:${ss}`
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
        {h.streak > 1 && <span className="hstreak row" style={{ gap: 2 }} title={`${h.streak} days in a row`}><Icon name="flame" size={12} strokeWidth={2} />{h.streak}</span>}
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
  const [open, setOpen] = useState<Record<string, boolean>>({})
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
        <span className="grow act-body"><b>{it.done && <span className="act-done" title={m.doneLabel || 'done'}>✓ </span>}{url ? <a href={url} target="_blank" rel="noreferrer" style={{ color: 'inherit' }}>{it.title || '(untitled)'}</a> : (it.title || '(untitled)')}</b>{it.sub && <span className="act-sub">{it.sub}</span>}
          {it.note && <span className="act-note">{it.note}</span>}
          {(it.events?.length || 0) > 0 && (() => {
            const k = (it.ref || it.title) + i
            const shown = open[k]
            return <>
              <button type="button" className="act-events-toggle" aria-expanded={!!shown} onClick={() => setOpen(o => ({ ...o, [k]: !shown }))}>
                <Icon name={shown ? 'chevron-down' : 'chevron-right'} size={12} />{shown ? 'Hide' : `${it.events!.length} ${it.events!.length === 1 ? 'event' : 'events'}`}
              </button>
              {shown && <ul className="act-events">{it.events!.map((e, j) => (
                <li key={j} className={`act-ev ${e.kind || ''}`}><span className="muted mono small">{e.time || '·'}</span>
                  <span>{e.url ? <a href={e.url} target="_blank" rel="noreferrer">{e.text}</a> : e.text}</span></li>
              ))}</ul>}
            </>
          })()}
        </span>
        {(it.minutes || 0) >= 1 ? <span className="muted small nowrap">{fmtMinutes(Math.round(it.minutes || 0))}</span> : it.amount ? <span className="money">{fmtAmount(it.amount, it.currency || cur)}</span> : (it.qty || 0) > 0 ? <span className="muted small nowrap">{it.qty} {it.qty === 1 ? m.qtyOne : m.qtyMany}</span> : null}
        <PinBtn ref_={`${source}:${it.ref || it.title}`} title={it.title} subtitle={it.sub} image={it.image} />
      </div>
    })}</div>
    {a.items.length > 8 && <button type="button" className="btn ghost sm mt-sm" onClick={() => setAll(x => !x)}>{all ? 'Show fewer' : `All ${a.items.length}`}</button>}
  </>
}
