import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { addDays, fmtDate, todayISO, weekday } from '../lib/format'
import { ACTIVITY, MOODS, MOOD_ICON, SOURCE_LABEL, fmtHeadlineShort, type Activity, fmtAmount, fmtKm, fmtMinutes, tagList, type Checkin, type DayPage, type Entry, type Money, type Music, type Photos, type Pin, type SourceName, type Watched, type Where } from '../lib/types'
import { Card, Confirm, Loading, Segmented, modalOpen, useAsync, useToast } from '../components/ui'
import { Icon, SourceIcon } from '../components/icons'
import { announceChange } from '../components/Layout'
import { FactCard, WeatherChip, factHasContent, type PinFn } from '../components/facts'
import { Timeline } from '../components/Timeline'
import { DictateButton, useAI } from '../components/ai'

const ORDER: SourceName[] = ['travel', 'immich', 'dawarich', 'maloja', 'podcasts', 'pinepods', 'audiobookshelf', 'jellystat', 'jellyseerr', 'habits', 'health', 'nextcloud', 'tasks', 'projects', 'forgejo', 'firefly', 'split', 'invoicing', 'documents', 'bookshelf', 'kitchen', 'inventory', 'garage']

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
      if (e.metaKey || e.ctrlKey || e.altKey || modalOpen()) return
      if (e.key === 'ArrowLeft') navigate(`/day/${addDays(date, -1)}`)
      if (e.key === 'ArrowRight' && date < today) navigate(`/day/${addDays(date, 1)}`)
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [date, navigate, today])

  // the shell's Write menu, n and m land here with the entry or the moment field to focus
  const loc = useLocation()
  const focus = (loc.state as { focus?: 'entry' | 'moment' } | null)?.focus
  useEffect(() => {
    if (!focus || !data || data.date !== date) return
    const el = document.querySelector<HTMLElement>(focus === 'moment' ? '.moment-add input[name="moment"]' : '.highlight-input')
    if (!el) return
    el.scrollIntoView({ block: 'center', behavior: 'smooth' })
    el.focus({ preventScroll: true })
    navigate(loc.pathname + loc.search, { replace: true, state: null })
  }, [loc.key, focus, data, date]) // eslint-disable-line react-hooks/exhaustive-deps

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
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return null
  const isToday = data.today
  const first = (user?.name || '').split(' ')[0]
  const hour = new Date().getHours()
  const greet = hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening'
  const unit = settings?.distance_unit || 'km'
  const have = new Set([...data.sources, ...Object.keys(data.facts)])
  const cards = ORDER.filter(s => have.has(s) && factHasContent(s, data.facts[s]))
  const quiet = ORDER.filter(s => data.sources.includes(s) && !cards.includes(s))
  const trip = data.facts.travel?.payload as { trips?: { name: string; day_no: number; days: number }[] } | undefined
  return (
    <>
      <div className="day-head">
        <div className="day-nav">
          <Link className="btn ghost icon-only" to={`/day/${addDays(date, -1)}`} title="Previous day (←)" aria-label="Previous day"><Icon name="chevron-left" size={20} /></Link>
          <div className="day-title">
            <h1>{isToday ? `${greet}${first ? ', ' + first : ''}` : weekday(date, true)}</h1>
            <div className="sub row wrap" style={{ gap: 8 }}>
              <span>{isToday ? `${weekday(date, true)}, ${fmtDate(date)}` : fmtDate(date)}</span>
              {!isToday && <Link className="status-pill accent" to="/"><Icon name="today" size={13} />Today</Link>}
              <WeatherChip fact={data.facts.weather} />
              {trip?.trips?.[0] && <span className="status-pill"><Icon name="plane" size={13} />{trip.trips[0].name} · day {trip.trips[0].day_no}/{trip.trips[0].days}</span>}
              {data.streak > 0 && isToday && <span className="status-pill" title="Days in a row with an entry"><Icon name="flame" size={13} />{data.streak} day streak</span>}
            </div>
          </div>
          {date < today ? <Link className="btn ghost icon-only" to={`/day/${addDays(date, 1)}`} title="Next day (→)" aria-label="Next day"><Icon name="chevron-right" size={20} /></Link> : <span className="btn ghost icon-only" style={{ visibility: 'hidden' }} aria-hidden="true" />}
        </div>
        <div className="actions row">
          <input type="date" className="date-jump" value={date} max={today} onChange={e => e.target.value && navigate(`/day/${e.target.value}`)} aria-label="Jump to a date" />
          {data.sources.length > 0 && <button type="button" className="btn" onClick={refresh} disabled={refreshing || data.future} title="Pull the day again from every connected app"><Icon name="refresh" size={15} className={refreshing ? 'spin' : ''} />{refreshing ? 'Pulling…' : 'Pull again'}</button>}
        </div>
      </div>

      {!data.future && <Glance facts={data.facts} unit={unit} />}

      {data.future ? <Card><p className="muted" style={{ margin: 0 }}>That day has not happened yet.</p></Card> : (
        <div className="day-grid">
          <div className="day-main">
            <EntryEditor date={date} entry={data.entry} isToday={isToday} prompt={isToday ? data.prompt : 'What happened on this day?'} onSaved={e => setData(d => d ? { ...d, entry: e } : d)} onDelete={() => setConfirmDel(true)} />
            <Timeline date={date} facts={data.facts} moments={data.moments} isToday={isToday} unit={unit} onMoments={ms => setData(d => d ? { ...d, moments: ms } : d)} />
            {data.pins.length > 0 && <Card title={<><Icon name="star" size={16} />Kept from this day</>} className="mb">
              <div className="pins">{data.pins.map(p => <div key={p.id} className="pin">{p.image ? <img src={p.image} alt="" /> : <span className="pin-ico"><SourceIcon source={p.source} size={34} /></span>}<div className="grow"><b>{p.title}</b>{p.subtitle && <div className="muted small">{p.subtitle}</div>}</div><button type="button" className="btn ghost icon-only sm" onClick={() => onUnpin(p.id)} title="Unpin" aria-label="Unpin"><Icon name="x" size={15} /></button></div>)}</div>
            </Card>}
            {data.memories.length > 0 && <Card title={<><Icon name="repeat" size={16} />On this day</>} className="mb">
              <div className="inline-list">{data.memories.map(m => <Link key={m.id} className="li" to={`/day/${m.date}`} style={{ color: 'inherit' }}><span className="badge">{m.date.slice(0, 4)}</span><span className="grow"><b>{m.highlight || firstLine(m.text)}</b>{m.highlight && m.text && <div className="muted small clamp2">{m.text}</div>}</span>{m.mood > 0 && <span title={MOODS[m.mood]}>{MOOD_ICON[m.mood]}</span>}</Link>)}</div>
            </Card>}
            <div className="row muted small" style={{ justifyContent: 'space-between' }}>
              <span>{data.prev_entry ? <Link className="row" style={{ gap: 2 }} to={`/day/${data.prev_entry}`} title="The entry before"><Icon name="chevron-left" size={14} />{fmtDate(data.prev_entry)}</Link> : ''}</span>
              <span>{data.next_entry ? <Link className="row" style={{ gap: 2 }} to={`/day/${data.next_entry}`} title="The entry after">{fmtDate(data.next_entry)}<Icon name="chevron-right" size={14} /></Link> : ''}</span>
            </div>
          </div>
          <div className="day-side">
            {data.sources.length === 0 && cards.length === 0 && <Card title={<><Icon name="layers" size={16} />Nothing connected yet</>}><p className="muted" style={{ margin: 0 }}>Connect Dawarich, Immich, Maloja, Jellystat, Pocket Money and the other Pocket apps under <Link to="/settings/sources">Settings → Sources</Link>, and every day fills itself in.</p></Card>}
            {cards.map(s => <div key={s} id={`src-${s}`}><FactCard source={s} fact={data.facts[s]} link={data.links[s]} unit={unit} pins={data.pins} onPin={onPin} onUnpin={onUnpin} date={date} /></div>)}
            {quiet.length > 0 && <p className="quiet-sources">Nothing from {quiet.map(q => SOURCE_LABEL[q].toLowerCase()).join(', ')}{data.facts[quiet[0]]?.error ? '' : ' on this day'}.</p>}
          </div>
        </div>
      )}
      {confirmDel && <Confirm title="Remove this day's entry?" message="The words, mood and tags go; moments and pins stay." onCancel={() => setConfirmDel(false)} onConfirm={deleteEntry} />}
    </>
  )
}

function firstLine(t: string) { return (t || '').split('\n')[0] || '(no words)' }

/** The user's words: highlight, text, mood, tags. Autosaves a second after typing stops. */
function EntryEditor({ date, entry, prompt, isToday, onSaved, onDelete }: { date: string; entry: Entry | null; prompt: string; isToday: boolean; onSaved: (e: Entry | null) => void; onDelete: () => void }) {
  const { toast } = useToast()
  const { settings } = useApp()
  const aiOn = useAI()
  const [f, setF] = useState({ highlight: entry?.highlight || '', text: entry?.text || '', mood: entry?.mood || 0, tags: entry?.tags || '', pinned: entry?.pinned || false })
  const [state, setState] = useState<'idle' | 'dirty' | 'saving' | 'saved'>('idle')
  const [question, setQuestion] = useState('')
  const [draft, setDraft] = useState<{ highlight: string; text: string } | null>(null)
  const [sugg, setSugg] = useState<{ highlight: string; tags: string[]; mood: number; why: string } | null>(null)
  const [busy, setBusy] = useState('')
  const timer = useRef<number | null>(null)
  const latest = useRef(f)
  latest.current = f
  useEffect(() => { setF({ highlight: entry?.highlight || '', text: entry?.text || '', mood: entry?.mood || 0, tags: entry?.tags || '', pinned: entry?.pinned || false }); setState('idle'); setDraft(null); setSugg(null); setQuestion('') }, [date]) // eslint-disable-line react-hooks/exhaustive-deps
  const loadQuestion = useCallback((refresh = false) => {
    if (!aiOn || !settings?.ai_question) return
    api.get<{ question: string }>(`${V1}/ai/days/${date}/question${refresh ? '?refresh=1' : ''}`).then(r => setQuestion(r.question)).catch(() => {})
  }, [aiOn, settings?.ai_question, date])
  useEffect(() => { if (isToday || !entry) loadQuestion() }, [date]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = useCallback(async () => {
    setState('saving')
    try {
      const r = await api.put<{ entry: Entry | null }>(`${V1}/days/${date}`, latest.current)
      onSaved(r.entry)
      setState('saved')
      announceChange()
    } catch (e) { toast((e as Error).message, 'err'); setState('dirty') }
  }, [date, onSaved, toast])
  const change = (patch: Partial<typeof f>) => {
    setF(x => ({ ...x, ...patch })); setState('dirty')
    if (timer.current) window.clearTimeout(timer.current)
    timer.current = window.setTimeout(save, 900)
  }
  useEffect(() => () => { if (timer.current) window.clearTimeout(timer.current) }, [])
  const run = async (what: 'draft' | 'suggest') => {
    setBusy(what)
    try {
      if (what === 'draft') { setSugg(null); setDraft(await api.post(`${V1}/ai/days/${date}/draft`)) }
      else { setDraft(null); setSugg(await api.post(`${V1}/ai/days/${date}/suggest`, { highlight: f.highlight, text: f.text })) }
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') }
  }
  const append = (t: string) => change({ text: f.text.trim() ? f.text.replace(/\s+$/, '') + '\n\n' + t : t })
  const words = f.text.trim() ? f.text.trim().split(/\s+/).length : 0
  const title = question
    ? <span className="ai-q" title="Tonight's question, from what the apps recorded"><Icon name="sparkles" size={15} />{question}<button type="button" className="btn ghost icon-only sm" onClick={() => loadQuestion(true)} title="Another question" aria-label="Another question"><Icon name="refresh" size={14} /></button></span>
    : <span className="muted" style={{ fontWeight: 500 }}>{prompt}</span>
  return (
    <Card className="mb entry-card" title={title} actions={<span className={`save-state ${state}`}>{state === 'saving' ? <><span className="spinner" style={{ width: 11, height: 11 }} />Saving…</> : state === 'saved' ? <><Icon name="check" size={13} />Saved</> : state === 'dirty' ? 'Unsaved' : ''}</span>}>
      {entry?.origin === 'habits' && state === 'idle' && <div className="from-habits"><Icon name="check-circle" size={15} /><span>From your Pocket Habits journal — nothing was written here, so Day uses that. Edit it and it becomes yours.</span></div>}
      <input className="highlight-input" value={f.highlight} onChange={e => change({ highlight: e.target.value })} placeholder="The one line for this day" maxLength={200} />
      <textarea className="entry-text" value={f.text} onChange={e => change({ text: e.target.value })} placeholder="And the rest, if you like…" rows={Math.max(4, Math.min(24, f.text.split('\n').length + 1))} />
      {aiOn && <div className="ai-bar">
        <button type="button" className="btn ghost sm" onClick={() => run('draft')} disabled={!!busy} title="Write a first draft from the facts, moments and pins"><Icon name="sparkles" size={14} />{busy === 'draft' ? 'Drafting…' : f.text.trim() ? 'Fill in the rest' : 'Draft my day'}</button>
        <button type="button" className="btn ghost sm" onClick={() => run('suggest')} disabled={!!busy || !(f.text.trim() || f.highlight.trim())} title="Suggest a highlight, tags and a mood from what you wrote"><Icon name="tag" size={14} />{busy === 'suggest' ? 'Thinking…' : 'Suggest highlight & tags'}</button>
      </div>}
      <DictateButton onText={append} />
      {draft && <div className="ai-panel">
        {draft.highlight && <div className="row mb-sm"><b className="grow">“{draft.highlight}”</b><button type="button" className="btn sm" onClick={() => change({ highlight: draft.highlight })}>Use as highlight</button></div>}
        {draft.text ? <div className="t">{draft.text}</div> : <p className="muted small">Your text already covers what the apps recorded.</p>}
        <div className="row mt-sm">{draft.text && <button type="button" className="btn primary sm" onClick={() => { change({ text: f.text.trim() ? f.text.replace(/\s+$/, '') + '\n\n' + draft.text : draft.text, ...(!f.highlight.trim() && draft.highlight ? { highlight: draft.highlight } : {}) }); setDraft(null) }}>{f.text.trim() ? 'Add below my text' : 'Use this draft'}</button>}<button type="button" className="btn ghost sm" onClick={() => run('draft')}>Another</button><button type="button" className="btn ghost sm" onClick={() => setDraft(null)}>Dismiss</button><span className="spacer" /><span className="muted small">Edit it — it's a start, not your voice.</span></div>
      </div>}
      {sugg && <div className="ai-panel">
        {sugg.highlight && <div className="row mb-sm"><span className="grow">“{sugg.highlight}”</span><button type="button" className="btn sm" onClick={() => change({ highlight: sugg.highlight })}>Use</button></div>}
        {sugg.tags.length > 0 && <div className="row mb-sm wrap"><span className="chips grow">{sugg.tags.map(t => <span key={t} className="chip sm">#{t}</span>)}</span><button type="button" className="btn sm" onClick={() => change({ tags: [f.tags, ...sugg.tags].filter(Boolean).join(',') })}>Add tags</button></div>}
        {sugg.mood > 0 && <div className="row"><span className="grow">{MOOD_ICON[sugg.mood]} {MOODS[sugg.mood]} <span className="muted small">· {sugg.why}</span></span><button type="button" className="btn sm" onClick={() => change({ mood: sugg.mood })}>Set mood</button></div>}
        <div className="row mt-sm"><button type="button" className="btn ghost sm" onClick={() => { change({ highlight: sugg.highlight || f.highlight, tags: [f.tags, ...sugg.tags].filter(Boolean).join(','), mood: sugg.mood || f.mood }); setSugg(null) }}>Apply all</button><button type="button" className="btn ghost sm" onClick={() => setSugg(null)}>Dismiss</button></div>
      </div>}
      <div className="entry-foot">
        <Segmented size="sm" value={f.mood} onChange={v => change({ mood: v === f.mood ? 0 : v })} options={[1, 2, 3, 4, 5].map(m => ({ value: m, label: MOOD_ICON[m], title: MOODS[m] }))} />
        <input className="tags-input" value={f.tags} onChange={e => change({ tags: e.target.value })} placeholder="tags, comma, separated" />
        <button type="button" className={`btn ghost sm ${f.pinned ? 'on' : ''}`} onClick={() => change({ pinned: !f.pinned })} title="Mark as a day worth coming back to" aria-pressed={f.pinned}><Icon name="star" size={14} />{f.pinned ? 'Kept' : 'Keep'}</button>
        <span className="spacer" />
        {words > 0 && <span className="muted small">{words} words</span>}
        {entry && <button type="button" className="btn ghost sm danger-text" onClick={onDelete}>Remove</button>}
      </div>
      {f.tags && <div className="chips mt-sm">{tagList(f.tags).map(t => <Link key={t} className="chip" to={`/journal?tag=${encodeURIComponent(t)}`}>#{t}</Link>)}</div>}
    </Card>
  )
}

/** The day in one row of numbers; each pill jumps to its card. */
function Glance({ facts, unit }: { facts: DayPage['facts']; unit: 'km' | 'mi' }) {
  const p = <T,>(src: string) => (facts[src]?.payload ?? null) as T | null
  const pills: { src: string; value: string; label: string }[] = []
  const w = p<Where>('dawarich'); if (w?.distance_km) pills.push({ src: 'dawarich', value: fmtKm(w.distance_km, unit), label: w.moving_minutes ? `${fmtMinutes(w.moving_minutes)} moving` : 'moved' })
  const ph = p<Photos>('immich'); if (ph?.count) pills.push({ src: 'immich', value: String(ph.count), label: ph.count === 1 ? 'photo' : 'photos' })
  const m = p<Music>('maloja'); if (m?.listens) pills.push({ src: 'maloja', value: String(m.listens), label: m.listens === 1 ? 'song' : 'songs' })
  const wt = p<Watched>('jellystat'); if (wt?.minutes) pills.push({ src: 'jellystat', value: fmtMinutes(wt.minutes), label: 'watched' })
  const mo = p<Money>('firefly'); if (mo?.count) pills.push({ src: 'firefly', value: fmtAmount(mo.totals[mo.main_currency] || 0, mo.main_currency), label: `${mo.count} payment${mo.count === 1 ? '' : 's'}` })
  const h = p<Checkin>('habits'); if (h?.total) pills.push({ src: 'habits', value: `${h.done}/${h.total}`, label: 'habits' })
  for (const src of ['podcasts', 'pinepods', 'audiobookshelf', 'forgejo', 'nextcloud', 'tasks', 'jellyseerr', 'documents', 'split', 'invoicing']) {
    const f = facts[src]
    if (!f || !f.headline) continue
    const a = f.payload as Activity | null
    const m = ACTIVITY[src]
    const value = m.unit === 'count' ? String(Math.round(f.headline)) : fmtHeadlineShort(src, f.headline, a?.currency)
    const label = m.unit === 'count' ? (f.headline === 1 ? m.one : m.many) : m.unit === 'qty' ? (f.headline === 1 ? m.qtyOne! : m.qtyMany!) : SOURCE_LABEL[src].toLowerCase()
    pills.push({ src, value, label: src === 'nextcloud' ? `${label} done` : label })
  }
  if (pills.length === 0) return null
  const jump = (src: string) => document.getElementById(`src-${src}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  return <div className="glance">{pills.map(x => <button type="button" key={x.src} className="glance-pill" onClick={() => jump(x.src)} title={SOURCE_LABEL[x.src]}><SourceIcon source={x.src} size={30} /><span><b>{x.value}</b><span className="gl">{x.label}</span></span></button>)}</div>
}
