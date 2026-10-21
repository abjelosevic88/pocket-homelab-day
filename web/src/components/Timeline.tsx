import React, { useMemo, useState } from 'react'
import { api, V1 } from '../lib/api'
import { ACTIVITY, KIND_ICON, fmtAmount, fmtKm, fmtMinutes, type Activity, type Cooked, type Fact, type Moment, type Money, type Music, type Photo, type Photos, type Watched, type Where } from '../lib/types'
import { Card, Segmented, useToast } from './ui'
import { Icon, SourceIcon } from './icons'
import { DictateButton, useAI } from './ai'

type Item = { time: string; end?: string; key: string; icon: React.ReactNode; kind: string; title: string; sub?: string; moment?: Moment; thumbs?: Photo[] }

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

function buildItems(facts: Record<string, Fact>, moments: Moment[], unit: 'km' | 'mi'): { timed: Item[]; untimed: Item[] } {
  const timed: Item[] = [], untimed: Item[] = []
  const p = <T,>(src: string) => (facts[src]?.payload ?? null) as T | null
  for (const m of moments) (m.time ? timed : untimed).push({ time: m.time, key: `m${m.id}`, icon: g(KIND_GLYPH[m.kind] || 'dot'), kind: 'moment', title: m.text, moment: m })
  const where = p<Where>('dawarich')
  for (const [i, e] of (where?.entries || []).entries()) {
    if (e.kind === 'visit') timed.push({ time: e.start, end: e.end, key: `v${i}`, icon: g('pin'), kind: 'place', title: e.name || 'A stop', sub: fmtMinutes(e.minutes) })
    else if ((e.distance_km || 0) >= 0.3) timed.push({ time: e.start, end: e.end, key: `j${i}`, icon: g(MODE_GLYPH[e.mode || ''] || 'route'), kind: 'move', title: `${MODE_VERB[e.mode || ''] || 'Moved'} ${fmtKm(e.distance_km || 0, unit)}`, sub: e.minutes ? fmtMinutes(e.minutes) : undefined })
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
  const money = p<Money>('firefly')
  for (const s of money?.items || []) (s.time ? timed : untimed).push({ time: s.time, key: `t${s.id}`, icon: src('firefly'), kind: 'money', title: s.description, sub: fmtAmount(s.amount, s.currency) })
  for (const source of Object.keys(ACTIVITY)) {
    const a = p<Activity>(source)
    for (const [i, it] of (a?.items || []).entries()) {
      const extra = (it.minutes || 0) >= 1 ? fmtMinutes(Math.round(it.minutes || 0)) : it.amount ? fmtAmount(it.amount, it.currency || a?.currency || 'EUR') : ''
      const item: Item = { time: it.time || '', end: it.end && it.end !== it.time ? it.end : undefined, key: `${source}${i}`, icon: src(source), kind: 'act', title: it.title + (it.done && ACTIVITY[source].doneLabel ? ` · ${ACTIVITY[source].doneLabel}` : ''), sub: [it.sub, extra].filter(Boolean).join(' · ') || undefined }
      ;(item.time ? timed : untimed).push(item)
    }
  }
  const cooked = p<Cooked>('kitchen')
  for (const c of cooked?.cooks || []) untimed.push({ time: '', key: `c${c.id}`, icon: src('kitchen'), kind: 'cook', title: `Cooked ${c.title}` })
  timed.sort((a, b) => a.time.localeCompare(b.time) || (a.kind === 'moment' ? -1 : 1))
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
  const { timed, untimed } = useMemo(() => buildItems(facts, moments, unit), [facts, moments, unit])
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
      <span className="tl-time">{it.time || ''}</span>
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
