import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, BackfillStatus, Settings as SettingsT, Source, SourceName, UsageRow, User } from '../lib/types'
import { SOURCE_APP, SOURCE_ICON, SOURCE_LABEL } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Card, Confirm, Field, Loading, Modal, PageHeader, Segmented, Tabs, useAsync, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { Avatar } from '../components/people'

type Tab = 'general' | 'sources' | 'notify' | 'ai' | 'users' | 'profile' | 'api' | 'data' | 'about'

export default function Settings() {
  const [tab, setTab] = useState<Tab>(() => (new URLSearchParams(window.location.search).get('tab') as Tab) || 'general')
  return (
    <>
      <PageHeader title="Settings" />
      <Tabs tabs={[{ id: 'general', label: 'General' }, { id: 'sources', label: 'Sources' }, { id: 'notify', label: 'Notifications' }, { id: 'ai', label: 'AI' }, { id: 'users', label: 'Users' }, { id: 'profile', label: 'Profile' }, { id: 'api', label: 'API & apps' }, { id: 'data', label: 'Backup & data' }, { id: 'about', label: 'About' }]} value={tab} onChange={setTab} />
      <div className="mt">
        {tab === 'general' && <General />}
        {tab === 'sources' && <Sources />}
        {tab === 'notify' && <Notify />}
        {tab === 'ai' && <AISettings />}
        {tab === 'users' && <Users />}
        {tab === 'profile' && <Profile />}
        {tab === 'api' && <Tokens />}
        {tab === 'data' && <Data />}
        {tab === 'about' && <About />}
      </div>
    </>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  const [q, setQ] = useState('')
  const [hits, setHits] = useState<{ name: string; region: string; lat: number; lon: number }[]>([])
  useEffect(() => {
    if (q.trim().length < 2) { setHits([]); return }
    const t = setTimeout(() => api.get<typeof hits>(`${V1}/geo/search?q=${encodeURIComponent(q)}`).then(setHits).catch(() => {}), 300)
    return () => clearTimeout(t)
  }, [q])
  if (!f) return <Loading />
  return (
    <div className="two-col">
      <Card title="Preferences">
        <div className="grid" style={{ gap: 14 }}>
          <Field label="App title"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} disabled={!isAdmin} /></Field>
          <Field label="The question above the text box" help="Shown on every day page and in the evening push."><input value={f.prompt} onChange={e => setF({ ...f, prompt: e.target.value })} maxLength={120} disabled={!isAdmin} /></Field>
          <div className="two-col tight">
            <Field label="Date format"><Select value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} disabled={!isAdmin} /></Field>
            <Field label="Time format"><Select value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} disabled={!isAdmin} /></Field>
          </div>
          <div className="two-col tight">
            <Field group label="Week starts on"><Segmented value={f.week_start} onChange={v => setF({ ...f, week_start: v as 'mon' | 'sun' })} options={[{ value: 'mon', label: 'Monday' }, { value: 'sun', label: 'Sunday' }]} size="sm" /></Field>
            <Field group label="Distances"><Segmented value={f.distance_unit} onChange={v => setF({ ...f, distance_unit: v as 'km' | 'mi' })} options={[{ value: 'km', label: 'km' }, { value: 'mi', label: 'miles' }]} size="sm" /></Field>
          </div>
          <Field label="Home location" help="Weather for days spent near home; days Dawarich places far away get the weather there.">
            <div className="grid" style={{ gap: 6 }}>
              <input value={q} onChange={e => setQ(e.target.value)} placeholder={f.home_place ? `${f.home_place} (${f.home_lat.toFixed(3)}, ${f.home_lon.toFixed(3)}) — type to change` : 'Search a town…'} disabled={!isAdmin} />
              {hits.length > 0 && <div className="result-list" style={{ maxHeight: 200 }}>{hits.map((h, i) => <button key={i} className="sl-item" onClick={() => { setF({ ...f, home_place: h.name, home_lat: h.lat, home_lon: h.lon }); setQ(''); setHits([]) }}><span className="grow">{h.name}</span><span className="muted small">{h.region}</span></button>)}</div>}
            </div>
          </Field>
          <Field label="Backfill" help="How many days back to pull facts for on first start and when a new source is connected."><input type="number" min={0} max={3650} value={f.backfill_days} onChange={e => setF({ ...f, backfill_days: Number(e.target.value) })} disabled={!isAdmin} style={{ maxWidth: 140 }} /></Field>
          {isAdmin ? <div><button className="btn primary" onClick={() => save()} disabled={busy}>Save</button></div> : <p className="muted small">Only an admin can change these.</p>}
        </div>
      </Card>
      <Card title="How it fits together">
        <table className="table compact"><tbody>
          <tr><th>Your words</th><td className="muted">One line, a longer note, a mood and tags per day. Moments are short timestamped lines through the day; a phone shortcut can post them.</td></tr>
          <tr><th>Facts</th><td className="muted">Everything the other apps know about the day, pulled in and frozen once the day is over. The journal still reads when a source is gone.</td></tr>
          <tr><th>Pins</th><td className="muted">Star a photo, a song, a place or a transaction to keep it at the top of the day.</td></tr>
          <tr><th>Memories</th><td className="muted">The same date in earlier years shows on the day page and in the morning push.</td></tr>
          <tr><th>Year</th><td className="muted">Month by month, moods, most played, most watched, books, places, trips and the days you starred.</td></tr>
          <tr><th>Keys</th><td className="muted"><kbd className="kbd">←</kbd> <kbd className="kbd">→</kbd> move a day, <kbd className="kbd">t</kbd> today, <kbd className="kbd">⌘K</kbd> search.</td></tr>
        </tbody></table>
      </Card>
    </div>
  )
}

const SOURCE_HELP: Record<SourceName, { what: string; key: string; url: string; extra?: string }> = {
  dawarich: { what: 'Distance, moving time, how you moved and the stops it recognised.', key: 'API key from Dawarich → Account', url: 'https://dawarich.example.com' },
  immich: { what: 'Photos and videos taken that day, with a thumbnail grid proxied through this app.', key: 'API key from Immich → Account settings → API keys', url: 'http://host.docker.internal:2283' },
  maloja: { what: 'What played: listens, top artists, the full track list.', key: 'no key needed (read API is open)', url: 'http://host.docker.internal:42010' },
  jellystat: { what: 'What you watched in Jellyfin: episodes, films, minutes.', key: 'API key from Jellystat → Settings → API keys', url: 'http://host.docker.internal:3000', extra: 'Jellyfin user name to keep (empty = everyone)' },
  firefly: { what: 'Withdrawals of the day with categories and totals per currency.', key: 'Personal access token from Firefly III → Profile → OAuth', url: 'http://host.docker.internal:8080' },
  habits: { what: 'The check-in: habits done, tracker values, the journal line.', key: 'Token from Pocket Habits → Settings → API tokens', url: 'http://host.docker.internal:8169' },
  bookshelf: { what: 'Books being read, started or finished.', key: 'Token from Pocket Bookshelf → Settings → API', url: 'http://host.docker.internal:8171' },
  travel: { what: 'Which trip the day fell in, and which day of it.', key: 'Token from Pocket Travel → Settings → API', url: 'http://host.docker.internal:8174' },
  kitchen: { what: 'What was cooked.', key: 'Token from Pocket Kitchen → Settings → API & apps', url: 'http://host.docker.internal:8176' },
  garage: { what: 'Fill-ups and service records.', key: 'Token from Pocket Garage → Settings → API', url: 'http://host.docker.internal:8175' },
  weather: { what: 'Daily weather from Open-Meteo for the home location, or where Dawarich says you were.', key: 'no key needed', url: '' },
}

function Sources() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  const { toast } = useToast()
  const [test, setTest] = useState<Record<string, string>>({})
  const { data: status, reload } = useAsync(() => api.get<{ backfill: BackfillStatus; configured: string[]; days_with_facts: number; first: string; last: string }>(`${V1}/facts/status`), [])
  useEffect(() => { if (!status?.backfill.running) return; const t = setInterval(reload, 2000); return () => clearInterval(t) }, [status?.backfill.running, reload])
  if (!f) return <Loading />
  const run = async (which: string, src: Source) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url: src.url, key: src.key, extra: src.extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const stat = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  const upd = (name: SourceName, patch: Partial<Source>) => setF({ ...f, sources: { ...f.sources, [name]: { ...f.sources[name], ...patch } } })
  const backfill = async (force: boolean) => {
    try { const r = await api.post<{ ok: boolean; error?: string }>(`${V1}/facts/backfill`, { force }); toast(r.ok ? 'Backfill started' : r.error || 'failed', r.ok ? 'ok' : 'err'); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const bf = status?.backfill
  return (
    <div className="grid" style={{ gap: 16 }}>
      <Card title="Pulling the days" actions={bf?.running ? <span className="badge cat">running</span> : undefined}>
        <p className="muted small">Today and yesterday are pulled every 30 minutes. Older days are frozen once fetched. {status ? <>{status.days_with_facts} days have facts{status.first ? ` (${status.first} → ${status.last})` : ''}.</> : null}</p>
        {bf?.running && <div className="progress"><div style={{ width: `${bf.total ? bf.done / bf.total * 100 : 0}%` }} /><span className="small muted">{bf.done}/{bf.total} · {bf.current}</span></div>}
        {isAdmin && <div className="row mt-sm"><button className="btn sm" onClick={() => backfill(false)} disabled={bf?.running}>Backfill missing days ({f.backfill_days} back)</button><button className="btn ghost sm" onClick={() => backfill(true)} disabled={bf?.running} title="Re-fetch even days that already have facts">Re-pull everything</button></div>}
      </Card>
      {f.source_names.map(name => {
        const src = f.sources[name]
        const help = SOURCE_HELP[name]
        return <Card key={name} title={<span className="row" style={{ gap: 8 }}><span>{SOURCE_ICON[name]}</span>{SOURCE_APP[name]} <span className="muted" style={{ fontWeight: 400 }}>— {SOURCE_LABEL[name].toLowerCase()}</span></span>}
          actions={<><span className={`badge ${src.configured ? 'cat' : ''}`}>{src.configured ? 'connected' : name === 'weather' ? 'set a home location' : 'not set'}</span><label className="row small"><input type="checkbox" checked={src.enabled} onChange={e => upd(name, { enabled: e.target.checked })} disabled={!isAdmin} style={{ width: 'auto' }} /> on</label></>}>
          <p className="muted small">{help.what}{help.key !== 'no key needed' ? ` ${help.key}.` : ''}</p>
          {name !== 'weather' && <div className="grid" style={{ gap: 12 }}>
            <div className={help.extra ? 'three-col tight' : 'two-col tight'}>
              <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={src.url} onChange={e => upd(name, { url: e.target.value })} placeholder={help.url} disabled={!isAdmin} /></Field>
              {src.needs_key && <Field label={name === 'firefly' ? 'Personal access token' : 'API key / token'}><input value={src.key} onChange={e => upd(name, { key: e.target.value })} disabled={!isAdmin} autoComplete="off" /></Field>}
              {help.extra && <Field label={help.extra}><input value={src.extra} onChange={e => upd(name, { extra: e.target.value })} disabled={!isAdmin} /></Field>}
            </div>
            <Field label="Public URL (optional)" help="What the browser opens for the ↗ links when the URL above is internal."><input value={src.public_url} onChange={e => upd(name, { public_url: e.target.value })} placeholder="https://…" disabled={!isAdmin} /></Field>
            <div className="row"><button className="btn sm" onClick={() => run(name, src)} disabled={!src.url}>Test connection</button>{stat(name)}</div>
          </div>}
          {name === 'weather' && <div className="row"><button className="btn sm" onClick={() => run(name, src)} disabled={!f.home_lat && !f.home_lon}>Test</button>{stat(name)}{!f.home_lat && !f.home_lon && <span className="muted small">Set the home location under General first.</span>}</div>}
        </Card>
      })}
      {isAdmin ? <div className="row"><button className="btn primary" onClick={() => save()} disabled={busy}>Save sources</button><span className="muted small">A newly connected source is pulled for today on the next tick; use Backfill for the past.</span></div> : <p className="muted small">Only an admin can change these.</p>}
    </div>
  )
}

function Notify() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  const { toast } = useToast()
  const [test, setTest] = useState('')
  if (!f) return <Loading />
  const run = async () => {
    setTest('testing…')
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which: 'notify', url: f.notify_url, format: f.notify_format }); setTest(r.ok ? `✓ ${r.message}` : `✕ ${r.error}`) } catch (e) { setTest(`✕ ${(e as Error).message}`) }
  }
  const send = async (which: 'evening' | 'morning' | 'weekly') => { try { const r = await api.post<{ ok: boolean; sent: number; error?: string }>(`${V1}/notify/run`, { which }); toast(r.ok ? (r.sent ? 'Sent' : 'Nothing to send') : r.error || 'failed', r.ok ? 'ok' : 'err') } catch (e) { toast((e as Error).message, 'err') } }
  const hours = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))
  return (
    <div className="two-col">
      <Card title="🔔 Where to send" actions={<span className={`badge ${f.notify_configured ? 'cat' : ''}`}>{f.notify_configured ? 'connected' : 'not set'}</span>}>
        <div className="grid" style={{ gap: 12 }}>
          <Field label="URL" help="Apprise gateway (…/notify/<key>), an ntfy topic URL, or any webhook that takes JSON."><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/day" disabled={!isAdmin} /></Field>
          <Field group label="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></Field>
          <div className="row"><button className="btn sm" onClick={run} disabled={!f.notify_url}>Send a test</button><span className={`small ${test.startsWith('✓') ? 'ok' : test.startsWith('✕') ? 'bad' : 'muted'}`}>{test}</span></div>
        </div>
      </Card>
      <Card title="When">
        <div className="grid" style={{ gap: 12 }}>
          <label className="row"><input type="checkbox" checked={f.notify_evening} onChange={e => setF({ ...f, notify_evening: e.target.checked })} style={{ width: 'auto' }} disabled={!isAdmin} /> Evening: “{f.prompt}” with the day's numbers, unless the day is already written</label>
          <Field label="Evening hour"><Select value={String(f.evening_hour)} onChange={v => setF({ ...f, evening_hour: Number(v) })} options={hours} disabled={!isAdmin} /></Field>
          <label className="row"><input type="checkbox" checked={f.notify_morning} onChange={e => setF({ ...f, notify_morning: e.target.checked })} style={{ width: 'auto' }} disabled={!isAdmin} /> Morning: yesterday in one line and “on this day” from earlier years</label>
          <Field label="Morning hour"><Select value={String(f.morning_hour)} onChange={v => setF({ ...f, morning_hour: Number(v) })} options={hours} disabled={!isAdmin} /></Field>
          <div className="row wrap">{isAdmin && <button className="btn primary" onClick={() => save()} disabled={busy}>Save</button>}<button className="btn sm" onClick={() => send('evening')} disabled={!f.notify_configured}>Send evening now</button><button className="btn sm" onClick={() => send('morning')} disabled={!f.notify_configured}>Send morning now</button>{f.ai_configured && <button className="btn sm" onClick={() => send('weekly')} disabled={!f.notify_configured}>Send week story now</button>}</div>
        </div>
      </Card>
    </div>
  )
}

const AI_MODELS = ['gpt-5.4-mini', 'gpt-5.4', 'gpt-5.4-nano', 'gpt-5.5', 'gpt-4.1-mini', 'gpt-4o-mini']
const FEATURE_LABEL: Record<string, string> = { draft: 'Draft my day', question: "Tonight's question", suggest: 'Suggest highlight & tags', dictation: 'Dictation', 'story-week': 'Week stories', 'story-month': 'Month stories', 'story-year': 'Year stories', ask: 'Ask (answers)', 'ask-plan': 'Ask (planning)' }

function AISettings() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  const [test, setTest] = useState('')
  const { data: usage } = useAsync(() => api.get<{ last_30_days: UsageRow[]; all_time: UsageRow[] }>(`${V1}/ai/usage`), [])
  if (!f) return <Loading />
  const run = async () => {
    setTest('testing…')
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which: 'ai', url: f.ai_base_url, key: f.ai_key, extra: f.ai_model }); setTest(r.ok ? `✓ ${r.message}` : `✕ ${r.error}`) } catch (e) { setTest(`✕ ${(e as Error).message}`) }
  }
  const tot = (rows: UsageRow[] = []) => rows.reduce((a, r) => ({ calls: a.calls + r.calls, p: a.p + r.prompt_tokens, c: a.c + r.completion_tokens }), { calls: 0, p: 0, c: 0 })
  const m30 = tot(usage?.last_30_days), all = tot(usage?.all_time)
  return (
    <div className="grid" style={{ gap: 16 }}>
      <div className="two-col">
        <Card title="✨ OpenAI" actions={<span className={`badge ${f.ai_configured ? 'cat' : ''}`}>{f.ai_configured ? 'connected' : 'not set'}</span>}>
          <div className="grid" style={{ gap: 12 }}>
            <Field label="API key" help="platform.openai.com → API keys. Any OpenAI-compatible endpoint works with the base URL below."><input value={f.ai_key} onChange={e => setF({ ...f, ai_key: e.target.value })} placeholder="sk-…" autoComplete="off" disabled={!isAdmin} /></Field>
            <div className="two-col tight">
              <Field label="Model" help={`Default ${f.ai_default_model}: fast and cheap, good enough for every feature here.`}><input list="ai-models" value={f.ai_model} onChange={e => setF({ ...f, ai_model: e.target.value })} placeholder={f.ai_default_model} disabled={!isAdmin} /><datalist id="ai-models">{AI_MODELS.map(m => <option key={m} value={m} />)}</datalist></Field>
              <Field label="Dictation model"><input value={f.ai_transcribe_model} onChange={e => setF({ ...f, ai_transcribe_model: e.target.value })} placeholder={f.ai_default_transcribe} disabled={!isAdmin} /></Field>
            </div>
            <div className="two-col tight">
              <Field label="Write in" help="Empty = the language of your own entries."><input value={f.ai_language} onChange={e => setF({ ...f, ai_language: e.target.value })} placeholder="e.g. Serbian (Latin script)" disabled={!isAdmin} /></Field>
              <Field label="Base URL (optional)"><input value={f.ai_base_url} onChange={e => setF({ ...f, ai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" disabled={!isAdmin} /></Field>
            </div>
            <div className="row"><button className="btn sm" onClick={run} disabled={!f.ai_key}>Test key & model</button><span className={`small ${test.startsWith('✓') ? 'ok' : test.startsWith('✕') ? 'bad' : 'muted'}`}>{test}</span></div>
          </div>
        </Card>
        <Card title="What it does">
          <table className="table compact"><tbody>
            <tr><th>Draft my day</th><td className="muted">A first-person draft from the facts, moments and pins — or only what your text is missing. You edit it.</td></tr>
            <tr><th>Tonight's question</th><td className="muted">One specific question about the day replaces the generic prompt, here and in the evening push.</td></tr>
            <tr><th>Suggest</th><td className="muted">A highlight, tags (from your own vocabulary) and a mood from what you wrote.</td></tr>
            <tr><th>Dictation</th><td className="muted">🎙 Speak the entry or a moment; it is transcribed and added.</td></tr>
            <tr><th>Stories</th><td className="muted">Week, month and year written up as a story — on the Calendar, the Year page and under Stories. A Sunday push with the week.</td></tr>
            <tr><th>Ask</th><td className="muted">“When was I last at the lake?” — answered from your days, with links to them.</td></tr>
          </tbody></table>
        </Card>
      </div>
      <div className="two-col">
        <Card title="Options">
          <div className="grid" style={{ gap: 12 }}>
            <label className="row"><input type="checkbox" checked={f.ai_question} onChange={e => setF({ ...f, ai_question: e.target.checked })} style={{ width: 'auto' }} disabled={!isAdmin} /> Tonight's question on the day page and in the evening push</label>
            <label className="row"><input type="checkbox" checked={f.ai_weekly_story} onChange={e => setF({ ...f, ai_weekly_story: e.target.checked })} style={{ width: 'auto' }} disabled={!isAdmin} /> Write the week's story every Sunday at the evening hour and push it</label>
            <label className="row"><input type="checkbox" checked={f.ai_send_text} onChange={e => setF({ ...f, ai_send_text: e.target.checked })} style={{ width: 'auto' }} disabled={!isAdmin} /> Send what I wrote (highlights, text, moments), not only the app data</label>
            <p className="muted small" style={{ margin: 0 }}>What leaves the server: for the days a feature needs, a plain-text summary — places and stops, photo places and times, artists, shows, purchases, habits, books, dishes, weather, moods and tags, and your words when the last option is on. Never photos, audio is sent only while dictating. OpenAI does not train on API data by default.</p>
            {isAdmin && <div><button className="btn primary" onClick={() => save()} disabled={busy}>Save</button></div>}
          </div>
        </Card>
        <Card title="Usage">
          {usage && (usage.all_time.length > 0 ? <>
            <table className="table compact">
              <thead><tr><th>Feature</th><th className="right">Calls</th><th className="right">Tokens in</th><th className="right">Tokens out</th></tr></thead>
              <tbody>{usage.last_30_days.map(r => <tr key={r.feature + r.model}><td>{FEATURE_LABEL[r.feature] || r.feature} <span className="muted small">{r.model}</span></td><td className="right">{r.calls}</td><td className="right">{r.prompt_tokens.toLocaleString()}</td><td className="right">{r.completion_tokens.toLocaleString()}</td></tr>)}</tbody>
            </table>
            <p className="muted small mt-sm">Last 30 days: {m30.calls} calls, {(m30.p + m30.c).toLocaleString()} tokens. All time: {all.calls} calls, {(all.p + all.c).toLocaleString()} tokens.</p>
          </> : <p className="muted">Nothing used yet.</p>)}
        </Card>
      </div>
    </div>
  )
}

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Card title="Users" actions={isAdmin && <button className="btn primary sm" onClick={() => setEdit({ role: 'member' })}>+ Add user</button>}>
      <p className="muted">Everyone's journal is their own: entries, moments and pins are private to the account. What the connected apps know about a day is shared. Admins manage users, sources and notifications.</p>
      <table className="table compact">
        <thead><tr><th /><th>Name</th><th>Email</th><th>Role</th><th /></tr></thead>
        <tbody>
          {users.map(u => (
            <tr key={u.id}>
              <td style={{ width: 32 }}><Avatar user={u} /></td>
              <td>{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge" style={{ marginLeft: 6 }}>you</span>}</td>
              <td className="muted">{u.email}</td>
              <td><span className={`badge ${u.role === 'admin' ? 'cat' : ''}`}>{u.role}</span></td>
              <td className="right">{isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm title={`Remove ${del.name || del.email}?`} message="Their account and their journal are deleted." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Card>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit user' : 'Add user'} onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add user'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Card title="Profile">
      <form onSubmit={save} className="grid" style={{ gap: 14, maxWidth: 480 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
        <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        {status?.auth_mode === 'local' && <>
          <hr />
          <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
          <Field label="New password" help="At least 8 characters. Leave empty to keep the current one."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
        </>}
        <div><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Card>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="two-col">
      <Card title="API tokens">
        <p className="muted">Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and write your journal, feed a Homepage widget (<code>GET /api/v1/dashboard</code>) or drive a phone shortcut. See <code>docs/api.md</code>.</p>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. iPhone shortcut)" /><button className="btn primary">Create</button></form>
        {fresh && <div className="callout success mb"><b>Copy it now — it is shown once.</b><pre className="mono" style={{ margin: '8px 0 0' }}>{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Card>
      <Card title="Shortcuts & widgets">
        <p className="muted">Handy endpoints at <code>{base}/api/v1</code>:</p>
        <table className="table compact"><tbody>
          <tr><td className="mono">POST /moments</td><td className="muted"><code>{'{"text":"coffee with A.","kind":"note"}'}</code> — a Siri Shortcut or Raycast “add a moment”; time defaults to now</td></tr>
          <tr><td className="mono">POST /entries</td><td className="muted"><code>{'{"highlight":"…","text":"…","mood":4,"tags":"walk"}'}</code> — appends to today; <code>date</code> optional</td></tr>
          <tr><td className="mono">GET /today</td><td className="muted">today's page: your words, moments, pins and every fact</td></tr>
          <tr><td className="mono">GET /dashboard</td><td className="muted">streak, written today, photos / km / listens today — for Homepage</td></tr>
          <tr><td className="mono">GET /memories</td><td className="muted">this date in earlier years</td></tr>
          <tr><td className="mono">GET /export.md</td><td className="muted">the whole journal as Markdown</td></tr>
        </tbody></table>
      </Card>
    </div>
  )
}

function Data() {
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const importJSON = async (file: File) => {
    setBusy(true)
    try { const r = await api.post<Record<string, number>>(`${V1}/import`, JSON.parse(await file.text())); toast('Imported: ' + (Object.entries(r).map(([k, v]) => `${v} ${k}`).join(', ') || 'nothing new')) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="two-col">
      <Card title="Backup">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>): every user's journal plus the frozen facts. Restore by stopping the app and replacing <code>pocket-day.db</code> in the data directory.</p>
        <a className="btn primary" href={`${V1}/backup`}>Download database</a>
      </Card>
      <Card title="Export & import">
        <p className="muted">Your journal (entries, moments, pins) plus the shared facts as one JSON file, or the words alone as Markdown. Importing merges: entries by date (nothing is overwritten), moments and pins added when missing, facts only for days that have none.</p>
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}>Export JSON</a><a className="btn" href={`${V1}/export.json?nofacts=1`}>JSON without facts</a><a className="btn" href={`${V1}/export.md`}>Markdown</a><label className="btn">{busy ? 'Importing…' : 'Import JSON…'}<input type="file" accept="application/json,.json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label></div>
      </Card>
    </div>
  )
}

function About() {
  const { data } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const counts = (data?.counts || {}) as Record<string, number>
  return (
    <Card title="About">
      <p><b>Pocket Day</b> is a journal that writes itself: one page per day with your own words on top and, underneath, what the other apps on the server already know about it — where you were, the photos, what played, what you watched, read, cooked and spent. One binary, one SQLite file, one volume.</p>
      <p className="muted small">Open source (MIT) — <a href="https://github.com/abjelosevic88/pocket-homelab-day" target="_blank" rel="noreferrer">github.com/abjelosevic88/pocket-homelab-day</a>. Sibling projects: Pocket Habits, Pocket Inventory, Pocket Bookshelf, Pocket Split, Pocket Clips, Pocket Travel, Pocket Garage, Pocket Kitchen.</p>
      {data && (
        <table className="table compact mt">
          <tbody>
            <tr><th>Version</th><td className="mono">{String(data.version)}</td></tr>
            <tr><th>Runtime</th><td className="mono">{String(data.go)} · {String(data.os)}/{String(data.arch)}</td></tr>
            <tr><th>Auth mode</th><td className="mono">{String(data.auth_mode)}</td></tr>
            <tr><th>Database</th><td className="mono">{String(data.db_path)} · {fmtBytes(Number(data.db_bytes))}</td></tr>
            <tr><th>Timezone</th><td className="mono">{String(data.timezone)}</td></tr>
            <tr><th>Uptime</th><td className="mono">{String(data.uptime)}</td></tr>
            <tr><th>Rows</th><td className="mono">{counts.entries} entries · {counts.moments} moments · {counts.pins} pins · {counts.facts} facts ({String(data.days_with_facts)} days{data.first_fact_day ? ` since ${String(data.first_fact_day)}` : ''}) · {counts.users} users</td></tr>
          </tbody>
        </table>
      )}
      <p className="muted small mt">Looking for a day? <Link to="/journal">Journal</Link> searches your words; <kbd className="kbd">⌘K</kbd> anywhere.</p>
    </Card>
  )
}
