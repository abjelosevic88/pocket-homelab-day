import React, { useMemo, useState } from 'react'
import { api, V1 } from '../lib/api'
import { locationTime, timelineOrder } from '../lib/locationTime'
import { ACTIVITY, KIND_ICON, fmtAmount, fmtKm, fmtMinutes, type Activity, type Cooked, type Fact, type HealthDay, type StillDay, type HomeDay, type RenewalsDay, type WealthDay, type PetsDay, type ParcelsDay, type SignDay, type PlacesDay, type VoiceDay, type WalletDay, type HoroscopeDay, type Moment, type Money, type Music, type Photo, type Photos, type Reading, type Watched, type Where } from '../lib/types'
import { fmtDate } from '../lib/format'
import { Card, Segmented, useToast } from './ui'
import { Icon, SourceIcon } from './icons'
import { DictateButton, useAI } from './ai'

type Item = { time: string; sortKey?: string; continued?: boolean; end?: string; key: string; icon: React.ReactNode; kind: string; title: string; sub?: string; moment?: Moment; thumbs?: Photo[] }

const MODE_GLYPH: Record<string, string> = { walking: 'walk', running: 'walk', cycling: 'bike', driving: 'car', train: 'train', flying: 'plane', plane: 'plane', boat: 'boat' }
/** A moment's kind as a stroke icon (the API keeps note / win / thanks / idea / event). */
export const KIND_GLYPH: Record<string, string> = { note: 'note', win: 'trophy', thanks: 'heart', idea: 'bulb', event: 'pin' }
const g = (name: string) => <Icon name={name} size={14} strokeWidth={2} />
const src = (s: string) => <SourceIcon source={s} size={14} bare />
const MODE_VERB: Record<string, string> = { walking: 'Walked', running: 'Ran', cycling: 'Cycled', driving: 'Drove', train: 'Train', flying: 'Flew', plane: 'Flew', boat: 'Boat' }

const mins = (t: string) => t && t.length >= 5 ? Number(t.slice(0, 2)) * 60 + Number(t.slice(3, 5)) : -1

/** Group sorted timed things into sessions when the gap is at most gap minutes. */
function sessions<T extends { time: string }>(list: T[], gap: number): T[][] {
  const sorted = list.filter(x => x.time).sort((a, b) => a.time.localeCompare(b.time))
  const out: T[][] = []
  for (const x of sorted) {
    const last = out[out.length - 1]
    if (last && mins(x.time) - mins(last[last.length - 1].time) <= gap) last.push(x)
    else out.push([x])
  }
  return out
}

const metres = (a: [number, number], b: [number, number]) => {
  const r = Math.PI / 180, x = (b[1] - a[1]) * r * Math.cos(((a[0] + b[0]) / 2) * r), y = (b[0] - a[0]) * r
  return Math.sqrt(x * x + y * y) * 6371000
}

/** Purchases with a place (Pocket Money receipts) by the stop they were paid
 *  during: close by (250 m) and at most five minutes outside the stay, or —
 *  a shop's pin comes from its address and can be a street or two off — up
 *  to 1.5 km away when paid well inside the stay. */
function purchasesByStop(entries: Where['entries'], items: Money['items'], date: string): Map<number, Money['items']> {
  const out = new Map<number, Money['items']>()
  for (const sp of items) {
    if (!sp.time || sp.lat == null || sp.lng == null) continue
    const t = mins(sp.time)
    let best = -1, bestScore = Infinity
    entries.forEach((e, i) => {
      if (e.kind !== 'visit' || e.lat == null || e.lng == null) return
      const from = (e.start_date || date) < date ? 0 : mins(e.start)
      const to = (e.end_date || date) > date ? 24 * 60 : mins(e.end)
      if (t < from - 5 || t > to + 5) return
      const d = metres([e.lat, e.lng], [sp.lat!, sp.lng!])
      const near = d <= 250, inside = t >= from && t <= to
      if (!near && !(inside && d <= 1500)) return
      const score = near ? d : 10000 + d
      if (score < bestScore) { best = i; bestScore = score }
    })
    if (best >= 0) out.set(best, [...(out.get(best) || []), sp])
  }
  return out
}

function buildItems(facts: Record<string, Fact>, moments: Moment[], unit: 'km' | 'mi', date: string, isToday: boolean): { timed: Item[]; untimed: Item[] } {
  const timed: Item[] = [], untimed: Item[] = []
  const p = <T,>(src: string) => (facts[src]?.payload ?? null) as T | null
  for (const m of moments) (m.time ? timed : untimed).push({ time: m.time, key: `m${m.id}`, icon: g(KIND_GLYPH[m.kind] || 'dot'), kind: 'moment', title: m.text, moment: m })
  const where = p<Where>('dawarich')
  const money = p<Money>('firefly')
  const paidAt = purchasesByStop(where?.entries || [], money?.items || [], date)
  for (const [i, e] of (where?.entries || []).entries()) {
    const timing = locationTime(e, date, isToday)
    const duration = e.minutes ? `${fmtMinutes(e.minutes)}${timing.crossesDay ? ' total' : ''}` : ''
    const when = { time: timing.time, sortKey: timing.sortKey, continued: timing.continued, sub: [timing.range, duration].filter(Boolean).join(' · ') }
    if (e.kind === 'visit') {
      // a stop you paid during is named after the shop when it has no name
      const paid = paidAt.get(i) || []
      const shop = paid.length ? (paid[0].destination || paid[0].description) : ''
      // the shop says more than the street the stop was named after
      const own = e.name && !/^unknown location$/i.test(e.name) ? e.name : ''
      const named = shop || own || 'A stop'
      const spent = paid.map(s => `paid ${fmtAmount(s.amount, s.currency)}${s.destination && s.destination !== named ? ` at ${s.destination}` : ''}`)
      timed.push({ ...when, sub: [shop && own ? own : '', when.sub, ...spent].filter(Boolean).join(' · '), key: `v${i}`, icon: g('pin'), kind: 'place', title: named })
    }
    else if ((e.distance_km || 0) >= 0.3) timed.push({ ...when, key: `j${i}`, icon: g(MODE_GLYPH[e.mode || ''] || 'route'), kind: 'move', title: `${MODE_VERB[e.mode || ''] || 'Moved'} ${fmtKm(e.distance_km || 0, unit)}` })
  }
  const photos = p<Photos>('immich')
  for (const s of sessions(photos?.items || [], 30)) {
    const places = [...new Set(s.map(x => x.city).filter(Boolean))].join(', ')
    timed.push({ time: s[0].time, end: s.length > 1 ? s[s.length - 1].time : undefined, key: `p${s[0].id}`, icon: src('immich'), kind: 'photos', title: s.length === 1 ? (s[0].type === 'VIDEO' ? 'A video' : 'A photo') : `${s.length} photos`, sub: places || undefined, thumbs: s.slice(0, 8) })
  }
  const music = p<Music>('maloja')
  for (const s of sessions(music?.items || [], 45)) {
    const counts: Record<string, number> = {}
    s.forEach(l => { counts[l.artist] = (counts[l.artist] || 0) + 1 })
    const top = Object.entries(counts).sort((a, b) => b[1] - a[1]).slice(0, 2).map(([a]) => a)
    timed.push({ time: s[0].time, end: s.length > 1 ? s[s.length - 1].time : undefined, key: `l${s[0].time}`, icon: src('maloja'), kind: 'music', title: s.length === 1 ? `${s[0].title}` : `${s.length} songs`, sub: s.length === 1 ? s[0].artist : top.join(', ') })
  }
  const watched = p<Watched>('jellystat')
  for (const w of watched?.items || []) timed.push({ time: w.time, key: `w${w.item_id || w.title}`, icon: src('jellystat'), kind: 'watch', title: w.series || w.title, sub: [w.series ? w.title : '', fmtMinutes(w.minutes)].filter(Boolean).join(' · ') })
  const reading = p<Reading>('bookshelf')
  for (const [i, r] of (reading?.sessions || []).entries()) timed.push({ time: r.time, end: r.end !== r.time ? r.end : undefined, key: `r${i}`, icon: src('bookshelf'), kind: 'read', title: `Read ${r.title}`, sub: `${r.pages ? `${r.pages} pages` : `${r.percent || 1}% of the book`} in ${r.estimated ? '~' : ''}${fmtMinutes(r.minutes)}` })
  for (const s of money?.items || []) (s.time ? timed : untimed).push({ time: s.time, key: `t${s.id}`, icon: src('firefly'), kind: 'money', title: s.description, sub: fmtAmount(s.amount, s.currency) })
  for (const source of Object.keys(ACTIVITY)) {
    const a = p<Activity>(source)
    for (const [i, it] of (a?.items || []).entries()) {
      const extra = (it.minutes || 0) >= 1 ? fmtMinutes(Math.round(it.minutes || 0)) : it.amount ? fmtAmount(it.amount, it.currency || a?.currency || 'EUR') : ''
      const item: Item = { time: it.time || '', end: it.end && it.end !== it.time ? it.end : undefined, key: `${source}${i}`, icon: src(source), kind: 'act', title: it.title + (it.done && ACTIVITY[source].doneLabel ? ` · ${ACTIVITY[source].doneLabel}` : ''), sub: [it.sub, it.note, extra].filter(Boolean).join(' · ') || undefined }
      ;(item.time ? timed : untimed).push(item)
    }
  }
  const health = p<HealthDay>('health')
  for (const w of health?.workouts || []) (w.time ? timed : untimed).push({ time: w.time || '', key: `hw${w.id}`, icon: src('health'), kind: 'act', title: `${w.name || w.kind.replace('_', ' ')} · ${fmtMinutes(w.minutes)}`, sub: [w.distance_km ? `${w.distance_km} km` : '', w.kcal ? `${w.kcal} kcal` : '', w.avg_hr ? `♥ ${w.avg_hr}` : ''].filter(Boolean).join(' · ') || undefined })
  if (health?.sleep) timed.push({ time: health.sleep.wake, key: 'hs', icon: src('health'), kind: 'act', title: `Woke up after ${fmtMinutes(health.sleep.asleep_min)} of sleep`, sub: `in bed since ${health.sleep.bed}` })
  const still = p<StillDay>('still')
  for (const x of still?.practices || []) (x.time ? timed : untimed).push({ time: x.time || '', key: `st${x.id}`, icon: src('still'), kind: 'act', title: `${x.title} · ${fmtMinutes(Math.round(x.minutes))}`, sub: [x.calm_before && x.calm_after ? `calm ${x.calm_before} → ${x.calm_after}` : '', x.lesson || ''].filter(Boolean).join(' · ') || undefined })
  const home = p<HomeDay>('home')
  for (const w of home?.work || []) untimed.push({ time: '', key: `hm${w.id}`, icon: src('home'), kind: 'act', title: w.title, sub: [w.thing, w.pro].filter(Boolean).join(' · ') || undefined })
  const ren = p<RenewalsDay>('renewals')
  for (const r of ren?.renewed || []) untimed.push({ time: '', key: `rn${r.id}`, icon: src('renewals'), kind: 'act', title: `Renewed: ${r.title}`, sub: [r.new_expires ? `now until ${fmtDate(r.new_expires)}` : '', r.cost ? fmtAmount(r.cost / 100, r.currency) : '', r.auto ? 'renewed by itself' : ''].filter(Boolean).join(' · ') || undefined })
  for (const e of ren?.expired || []) untimed.push({ time: '', key: `rx${e.id}`, icon: src('renewals'), kind: 'act', title: `Ran out: ${e.title}`, sub: [e.holder, e.provider].filter(Boolean).join(' · ') || undefined })
  const wl = p<WealthDay>('wealth')
  for (const it of wl?.items || []) untimed.push({ time: '', key: `wl${it.type}${it.id}`, icon: src('wealth'), kind: 'act', title: it.title, sub: [it.type === 'activity' ? fmtAmount(it.amount / 100, it.currency) : '', it.asset && !it.title.includes(it.asset) ? it.asset : ''].filter(Boolean).join(' · ') || undefined })
  const pets = p<PetsDay>('pets')
  for (const b of pets?.birthdays || []) untimed.push({ time: '', key: `pb${b.pet_id}`, icon: src('pets'), kind: 'act', title: b.line })
  for (const e of pets?.entries || []) untimed.push({ time: '', key: `pe${e.id}`, icon: src('pets'), kind: 'act', title: e.line, sub: e.notes || undefined })
  for (const w of pets?.weights || []) untimed.push({ time: '', key: `pw${w.id}`, icon: src('pets'), kind: 'act', title: w.line })
  const sign = p<SignDay>('sign')
  for (const it of [...(sign?.completed || []), ...(sign?.signed_by_me || []), ...(sign?.sent || []), ...(sign?.signed || [])]) (it.time ? timed : untimed).push({ time: it.time || '', key: `sg${it.document_id}${it.recipient_id || ''}${it.line}`, icon: src('sign'), kind: 'act', title: it.line })
  const places = p<PlacesDay>('places')
  for (const v of places?.visits || []) (v.time ? timed : untimed).push({ time: v.time || '', key: `plv${v.id}`, icon: src('places'), kind: 'act', title: v.line, sub: [v.trip, v.notes].filter(Boolean).join(' · ') || undefined })
  for (const v of places?.planned || []) (v.time ? timed : untimed).push({ time: v.time || '', key: `plp${v.id}`, icon: src('places'), kind: 'act', title: v.line })
  for (const sv of places?.saved || []) untimed.push({ time: '', key: `pls${sv.id}`, icon: src('places'), kind: 'act', title: sv.line, sub: [sv.cuisine, sv.city].filter(Boolean).join(' · ') || undefined })
  const wallet = p<WalletDay>('wallet')
  for (const ps of wallet?.passes || []) (ps.time ? timed : untimed).push({ time: ps.time || '', key: `wp${ps.id}`, icon: src('wallet'), kind: 'act', title: ps.line, sub: [ps.until && `until ${ps.until}`, ps.gate && `gate ${ps.gate}`, ps.trip].filter(Boolean).join(' · ') || undefined })
  const voice = p<VoiceDay>('voice')
  for (const m of voice?.memos || []) {
    const journal = m.kind === 'journal'
    const what = m.title || m.summary || ''
    const said = journal ? (m.summary || m.text) : (m.summary !== what ? m.summary : '')
    ;(m.time ? timed : untimed).push({ time: m.time || '', key: `vm${m.id}`, icon: src('voice'), kind: 'act', title: `${journal ? 'Voice journal' : 'Voice memo'}${what ? `: ${what}` : ''}`, sub: [said, m.went ? `→ ${m.went}` : '', m.length].filter(Boolean).join(' · ') || undefined })
  }
  const parcels = p<ParcelsDay>('parcels')
  for (const it of [...(parcels?.delivered || []), ...(parcels?.sent || []), ...(parcels?.ordered || [])]) (it.time ? timed : untimed).push({ time: it.time || '', key: `pc${it.id}${it.line.split(':')[0]}`, icon: src('parcels'), kind: 'act', title: it.line, sub: [it.carrier, it.days != null && !it.out && it.line.startsWith('Delivered') ? `${it.days} ${it.days === 1 ? 'day' : 'days'} on the way` : ''].filter(Boolean).join(' · ') || undefined })
  for (const s of parcels?.steps || []) (s.time ? timed : untimed).push({ time: s.time || '', key: `ps${s.parcel_id}${s.status}`, icon: src('parcels'), kind: 'act', title: s.line })
  const sky = p<HoroscopeDay>('horoscope')
  for (const [i, e] of (sky?.events || []).entries()) if (e.kind !== 'ingress' || e.title.startsWith('Sun')) (e.time ? timed : untimed).push({ time: e.time || '', key: `sky${i}`, icon: src('horoscope'), kind: 'act', title: e.title })
  const cooked = p<Cooked>('kitchen')
  for (const c of cooked?.cooks || []) untimed.push({ time: '', key: `c${c.id}`, icon: src('kitchen'), kind: 'cook', title: `Cooked ${c.title}` })
  timed.sort((a, b) => timelineOrder(a, b, date))
  return { timed, untimed }
}

const subLine = (it: Item) => [it.end && it.end !== it.time ? `until ${it.end}` : '', it.sub || ''].filter(Boolean).join(' · ')

/** The day in time order, with the moments you add in between. */
export function Timeline({ date, facts, moments, isToday, unit, onMoments }: { date: string; facts: Record<string, Fact>; moments: Moment[]; isToday: boolean; unit: 'km' | 'mi'; onMoments: (m: Moment[]) => void }) {
  const { toast } = useToast()
  const aiOn = useAI()
  const [text, setText] = useState('')
  const [kind, setKind] = useState('note')
  const [time, setTime] = useState('')
  const [all, setAll] = useState(false)
  const { timed, untimed } = useMemo(() => buildItems(facts, moments, unit, date, isToday), [facts, moments, unit, date, isToday])
  const add = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!text.trim()) return
    try {
      const body: Partial<Moment> = { date, text, kind }
      if (time) body.time = time
      else if (!isToday) body.time = ''
      const m = await api.post<Moment>(`${V1}/moments`, body)
      onMoments([...moments, m])
      setText(''); setTime('')
    } catch (e) { toast((e as Error).message, 'err') }
  }
  const del = async (id: number) => { try { await api.del(`${V1}/moments/${id}`); onMoments(moments.filter(m => m.id !== id)) } catch (e) { toast((e as Error).message, 'err') } }
  const edit = async (m: Moment, t: string) => {
    if (!t.trim() || t === m.text) return
    try { const r = await api.put<Moment>(`${V1}/moments/${m.id}`, { text: t }); onMoments(moments.map(x => x.id === m.id ? r : x)) } catch (e) { toast((e as Error).message, 'err') }
  }
  const limit = 14
  const shown = all ? timed : timed.slice(0, limit)
  const row = (it: Item) => (
    <div key={it.key} className={`tl-item tl-${it.kind}`}>
      <span className={`tl-time${it.continued ? ' tl-time-continued' : ''}`}>{it.time || ''}</span>
      <span className="tl-dot">{it.icon}</span>
      <div className="tl-body">
        {it.moment
          ? <span className="tl-title" contentEditable suppressContentEditableWarning onBlur={e => edit(it.moment!, e.currentTarget.textContent || '')} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); (e.target as HTMLElement).blur() } }}>{it.title}</span>
          : <span className="tl-title">{it.title}</span>}
        {subLine(it) && <span className="tl-sub">{subLine(it)}</span>}
        {it.thumbs && <div className="tl-thumbs">{it.thumbs.map(ph => ph.id.startsWith('demo-') ? <span key={ph.id} className="tl-thumb demo" /> : <img key={ph.id} className="tl-thumb" src={`${V1}/photos/${ph.id}/thumb`} alt="" loading="lazy" />)}</div>}
      </div>
      {it.moment && <button type="button" className="btn ghost icon-only sm tl-del" onClick={() => del(it.moment!.id)} title="Delete moment" aria-label="Delete moment"><Icon name="x" size={14} /></button>}
    </div>
  )
  return (
    <Card title={<><Icon name="clock" size={16} />The day</>} className="mb timeline-card" actions={<span className="muted small">{timed.length ? `${timed.length} things` : ''}</span>}>
      {timed.length === 0 && untimed.length === 0 && <p className="muted small" style={{ margin: '0 0 12px' }}>{isToday ? 'Nothing yet. Moments you add and everything the apps record lands here in time order.' : 'Nothing recorded with a time on this day.'}</p>}
      {timed.length > 0 && <div className="tl">{shown.map(row)}</div>}
      {timed.length > limit && <button type="button" className="btn ghost sm tl-more" onClick={() => setAll(a => !a)}>{all ? 'Show less' : `Show all ${timed.length}`}</button>}
      {untimed.length > 0 && <><div className="tl-label">During the day</div><div className="tl">{untimed.map(row)}</div></>}
      <form onSubmit={add} className="moment-add">
        <Segmented size="sm" value={kind} onChange={setKind} options={Object.keys(KIND_ICON).map(k => ({ value: k, label: <Icon name={KIND_GLYPH[k] || 'dot'} size={15} />, title: k }))} />
        <input name="moment" value={text} onChange={e => setText(e.target.value)} aria-label="A moment" placeholder={isToday ? 'Add a moment — what just happened?' : 'Add something from that day…'} />
        <input type="time" value={time} onChange={e => setTime(e.target.value)} title="Time (optional; now for today)" className="time-input" />
        {aiOn && <DictateButton small onText={t => setText(x => (x ? x + ' ' : '') + t)} />}
        <button type="submit" className="btn primary sm" disabled={!text.trim()}><Icon name="plus" size={14} />Add</button>
      </form>
    </Card>
  )
}
