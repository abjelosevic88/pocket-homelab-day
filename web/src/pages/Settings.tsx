import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, BackfillStatus, ConnMode, Settings as SettingsT, Source, SourceName, UsageRow, User } from '../lib/types'
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

// withoutModes drops the per-source modes so saving the form never pins a
// default (a mode is only stored when chosen in a Connection).
const withoutModes = (srcs: Record<SourceName, Source>) => Object.fromEntries(Object.entries(srcs).map(([k, v]) => [k, { ...v, mode: undefined }])) as Record<SourceName, Source>

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, sources: withoutModes(f.sources), ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
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
          <Field label="App title"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} disabled={!isAdmin} title={isAdmin ? undefined : "Only an admin can rename the app"} /></Field>
          <Field label="The question above the text box" help="Shown on every day page and in the evening push."><input value={f.prompt} onChange={e => setF({ ...f, prompt: e.target.value })} maxLength={120} /></Field>
          <div className="two-col tight">
            <Field label="Date format"><Select value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></Field>
            <Field label="Time format"><Select value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></Field>
          </div>
          <div className="two-col tight">
            <Field group label="Week starts on"><Segmented value={f.week_start} onChange={v => setF({ ...f, week_start: v as 'mon' | 'sun' })} options={[{ value: 'mon', label: 'Monday' }, { value: 'sun', label: 'Sunday' }]} size="sm" /></Field>
            <Field group label="Distances"><Segmented value={f.distance_unit} onChange={v => setF({ ...f, distance_unit: v as 'km' | 'mi' })} options={[{ value: 'km', label: 'km' }, { value: 'mi', label: 'miles' }]} size="sm" /></Field>
          </div>
          <Field label="Home location" help="Weather for days spent near home; days Dawarich places far away get the weather there.">
            <div className="grid" style={{ gap: 6 }}>
              <input value={q} onChange={e => setQ(e.target.value)} placeholder={f.home_place ? `${f.home_place} (${f.home_lat.toFixed(3)}, ${f.home_lon.toFixed(3)}) — type to change` : 'Search a town…'} />
              {hits.length > 0 && <div className="result-list" style={{ maxHeight: 200 }}>{hits.map((h, i) => <button key={i} className="sl-item" onClick={() => { setF({ ...f, home_place: h.name, home_lat: h.lat, home_lon: h.lon }); setQ(''); setHits([]) }}><span className="grow">{h.name}</span><span className="muted small">{h.region}</span></button>)}</div>}
            </div>
          </Field>
          <Field label="Backfill" help="How many days back to pull facts for on first start and when a new source is connected."><input type="number" min={0} max={3650} value={f.backfill_days} onChange={e => setF({ ...f, backfill_days: Number(e.target.value) })} style={{ maxWidth: 140 }} /></Field>
          <div><button className="btn primary" onClick={() => save()} disabled={busy}>Save</button></div>
          <p className="muted small">These are your own settings{isAdmin ? '; the app title is the same for everyone' : ''}.</p>
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
  firefly: { what: 'Withdrawals of the day with categories and totals per currency, read from Pocket Money’s Firefly-compatible API.', key: 'Direct: a token from Pocket Money → Settings → API tokens with the URL ending in /firefly (a Firefly III URL with its personal access token works too)', url: 'http://host.docker.internal:8178/firefly' },
  habits: { what: 'The check-in: habits done, tracker values, the journal line.', key: 'Token from Pocket Habits → Settings → API tokens', url: 'http://host.docker.internal:8169' },
  bookshelf: { what: 'Books being read, started or finished.', key: 'Token from Pocket Bookshelf → Settings → API', url: 'http://host.docker.internal:8171' },
  travel: { what: 'Which trip the day fell in, and which day of it.', key: 'Token from Pocket Travel → Settings → API', url: 'http://host.docker.internal:8174' },
  kitchen: { what: 'What was cooked.', key: 'Token from Pocket Kitchen → Settings → API & apps', url: 'http://host.docker.internal:8176' },
  garage: { what: 'Fill-ups and service records.', key: 'Token from Pocket Garage → Settings → API', url: 'http://host.docker.internal:8175' },
  weather: { what: 'Daily weather from Open-Meteo for the home location, or where Dawarich says you were.', key: 'no key needed', url: '' },
  pinepods: { what: 'Podcast episodes played, with minutes per day. Episodes spread over several days count on each of them (from the day Pocket Day starts watching).', key: 'API key from PinePods → Settings → API keys', url: 'http://host.docker.internal:8136' },
  audiobookshelf: { what: 'Listening sessions — audiobooks and ABS podcasts — per title, with minutes. Sessions under a minute are ignored.', key: 'An admin API key (Settings → API Keys) so every user’s sessions are visible', url: 'http://host.docker.internal:13378', extra: 'Only these users (comma separated; empty = everyone)' },
  jellyseerr: { what: 'Films and series requested, and requested titles that became available.', key: 'API key from Jellyseerr → Settings → General', url: 'http://host.docker.internal:5055' },
  forgejo: { what: 'Your pushes per repository: commits, the latest message, new repositories, tags and pull requests.', key: 'A read-only access token (Settings → Applications, scopes read:user + read:repository)', url: 'http://host.docker.internal:3000', extra: 'User name (empty = the token’s owner)' },
  nextcloud: { what: 'Tasks completed (every task list), events and contact birthdays, over CalDAV.', key: 'An app password (Personal settings → Security)', url: 'http://host.docker.internal:8080', extra: 'Nextcloud user name' },
  paperless: { what: 'Documents added that day, with correspondent and type.', key: 'API token from Paperless → My profile', url: 'http://host.docker.internal:8000' },
  invoicing: { what: 'Invoices issued and payments received, with amounts.', key: 'Token from Pocket Invoicing → Settings → API', url: 'http://host.docker.internal:8168' },
  split: { what: 'Shared expenses dated that day, across every group.', key: 'Token from Pocket Split → Settings → API', url: 'http://host.docker.internal:8172' },
  inventory: { what: 'Things added, used up or restocked.', key: 'Token from Pocket Inventory → Settings → API tokens', url: 'http://host.docker.internal:8170' },
}

function Sources() {
  const { f, setF, save, busy } = useSettingsForm()
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
  const saveSource = (name: SourceName, patch: Partial<Source>) => save({ sources: { ...withoutModes(f.sources), [name]: { ...f.sources[name], mode: undefined, ...patch } } })
  const backfill = async (force: boolean) => {
    try { const r = await api.post<{ ok: boolean; error?: string }>(`${V1}/facts/backfill`, { force }); toast(r.ok ? 'Backfill started' : r.error || 'failed', r.ok ? 'ok' : 'err'); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const bf = status?.backfill
  const title = (name: SourceName) => <span className="row" style={{ gap: 8 }}><span>{SOURCE_ICON[name]}</span>{SOURCE_APP[name]} <span className="muted" style={{ fontWeight: 400 }}>— {SOURCE_LABEL[name].toLowerCase()}</span></span>
  return (
    <div className="grid" style={{ gap: 16 }}>
      <Card title="Pulling the days" actions={bf?.running ? <span className="badge cat">running</span> : undefined}>
        <p className="muted small">Today and yesterday are pulled every 30 minutes. Older days are frozen once fetched. {status ? <>{status.days_with_facts} days have facts{status.first ? ` (${status.first} → ${status.last})` : ''}.</> : null}</p>
        {bf?.running && <div className="progress"><div style={{ width: `${bf.total ? bf.done / bf.total * 100 : 0}%` }} /><span className="small muted">{bf.done}/{bf.total} · {bf.current}</span></div>}
        <p className="muted small">The sources are yours: what you connect here is pulled with your own accounts and only ever shows up in your journal. The Pocket apps can come through Pocket Core, as you, with nothing to paste; so can the other services (Immich, Maloja, Dawarich…), with your own account there from Pocket Core.</p>
        <div className="row mt-sm"><button className="btn sm" onClick={() => backfill(false)} disabled={bf?.running}>Backfill missing days ({f.backfill_days} back)</button><button className="btn ghost sm" onClick={() => backfill(true)} disabled={bf?.running} title="Re-fetch even days that already have facts">Re-pull everything</button></div>
      </Card>
      {f.source_names.map(name => {
        const src = f.sources[name]
        const help = SOURCE_HELP[name]
        if (src.pocket) return <Card key={name} title={title(name)} actions={connBadge(src.mode || 'off', src.via)}>
          <p className="muted small">{help.what}</p>
          <Connection app={src.app || SOURCE_APP[name]} mode={src.mode || 'off'} via={src.via} detail={src.detail || ''} core={f.core_configured} hasToken={!!src.key || !!src.url}
            onMode={m => saveSource(name, { mode: m })} onDisconnect={() => saveSource(name, { url: '', key: '', public_url: '' })}
            test={<><button className="btn sm" onClick={() => run(name, src)}>Test connection</button>{stat(name)}</>}
            direct={<>
              <div className="two-col tight">
                <Field label="URL" help={name === 'firefly' ? 'Pocket Money’s URL followed by /firefly. Inside Docker use http://host.docker.internal:PORT.' : 'Inside Docker use http://host.docker.internal:PORT or the service name.'}><input value={src.url} onChange={e => upd(name, { url: e.target.value })} placeholder={help.url} /></Field>
                <Field label="API token" help={help.key}><input value={src.key} onChange={e => upd(name, { key: e.target.value })} autoComplete="off" /></Field>
              </div>
              <Field label="Public URL (optional)" help="What the browser opens for the ↗ links when the URL above is internal."><input value={src.public_url} onChange={e => upd(name, { public_url: e.target.value })} placeholder="https://…" /></Field>
            </>} />
        </Card>
        if (src.service) {
          // Non-credential settings (user filters, the Forgejo user) apply in every mode; the Nextcloud user name is part of the credential.
          const credExtra = name === 'nextcloud'
          const extraField = help.extra && !credExtra ? <Field label={help.extra}><input value={src.extra} onChange={e => upd(name, { extra: e.target.value })} /></Field> : null
          return <Card key={name} title={title(name)} actions={connBadge(src.mode || 'off', src.via)}>
            <p className="muted small">{help.what}</p>
            <Connection app={src.app || SOURCE_APP[name]} mode={src.mode || 'off'} via={src.via} detail={src.detail || ''} core={f.core_configured} hasToken={!!src.key || !!src.url}
              service keySource={src.key_source || ''} keyless={!src.needs_key}
              onMode={m => saveSource(name, { mode: m })} onDisconnect={() => saveSource(name, { url: '', key: '', public_url: '', ...(credExtra ? { extra: '' } : {}) })}
              extra={extraField ? <><div className="two-col tight">{extraField}</div><div><button className="btn sm" onClick={() => saveSource(name, { extra: src.extra })} disabled={busy}>Save</button></div></> : undefined}
              test={<><button className="btn sm" onClick={() => run(name, src)}>Test connection</button>{stat(name)}</>}
              direct={<>
                <div className={credExtra ? 'three-col tight' : 'two-col tight'}>
                  <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={src.url} onChange={e => upd(name, { url: e.target.value })} placeholder={help.url} /></Field>
                  {src.needs_key && <Field label={name === 'nextcloud' ? 'App password' : 'API key / token'} help={help.key}><input value={src.key} onChange={e => upd(name, { key: e.target.value })} autoComplete="off" /></Field>}
                  {credExtra && <Field label={help.extra || 'User name'}><input value={src.extra} onChange={e => upd(name, { extra: e.target.value })} /></Field>}
                </div>
                <Field label="Public URL (optional)" help="What the browser opens for the ↗ links when the URL above is internal."><input value={src.public_url} onChange={e => upd(name, { public_url: e.target.value })} placeholder="https://…" /></Field>
              </>} />
          </Card>
        }
        return <Card key={name} title={title(name)}
          actions={<><span className={`badge ${src.configured ? 'cat' : ''}`}>{src.configured ? 'connected' : 'set a home location'}</span><label className="row small"><input type="checkbox" checked={src.enabled} onChange={e => upd(name, { enabled: e.target.checked })} style={{ width: 'auto' }} /> on</label></>}>
          <p className="muted small">{help.what}</p>
          <div className="row"><button className="btn sm" onClick={() => run(name, src)} disabled={!f.home_lat && !f.home_lon}>Test</button>{stat(name)}{!f.home_lat && !f.home_lon && <span className="muted small">Set the home location under General first.</span>}</div>
        </Card>
      })}
      <div className="row"><button className="btn primary" onClick={() => save()} disabled={busy}>Save sources</button><span className="muted small">A newly connected source is pulled for today on the next tick; use Backfill for the past.</span></div>
    </div>
  )
}

// Connection is how Day reaches one Pocket app or service for you: through
// Pocket Core (as you, nothing to paste), with your own URL and token, or not
// at all. A service through Pocket Core says whose key it comes with; extra
// holds its settings that are not credentials (shown in every mode but off).
// On a box with Pocket Core, Direct is advanced: the switch is Pocket Core |
// Off and Direct sits behind a small link, unless you are on Direct already
// (your current mode is never hidden).
function Connection({ app, mode, via, detail, core, onMode, onDisconnect, hasToken, test, direct, service, keySource, keyless, extra }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode
  service?: boolean; keySource?: string; keyless?: boolean; extra?: React.ReactNode
}) {
  const ok = via !== ''
  const [advanced, setAdvanced] = useState(mode === 'direct')
  const showDirect = !core || mode === 'direct' || advanced
  const cred = keyless ? 'URL' : service ? 'URL and key' : 'URL and token'
  const whose = keySource === 'personal' ? (keyless ? ' (your own entry)' : ' (your own key)') : keySource === 'box' ? (keyless ? " (the box's address)" : " (the box's key)") : ''
  const coreLine = service
    ? `Connected through Pocket Core${whose}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.`
    : 'Connected through Pocket Core, as you. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.'
  const line = mode === 'off' ? `Off: Day does not read ${app}.`
    : mode === 'core' ? (ok ? coreLine : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} ${cred}; use Test connection to check it.` : `Not connected: ${detail || `add the ${cred}`}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'There is no Pocket Core on this box' },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: `Your own ${cred}` }] : []),
          { value: 'off', label: 'Off' },
        ]} />
      </div>
      {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL{keyless ? '' : ' and key'}</button>}
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
      {mode !== 'off' && extra}
    </div>
  )
}

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge cat">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function Notify() {
  const { f, setF, save, busy } = useSettingsForm()
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
          <Field label="URL" help="Apprise gateway (…/notify/<key>), an ntfy topic URL, or any webhook that takes JSON."><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/day" /></Field>
          <Field group label="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></Field>
          <div className="row"><button className="btn sm" onClick={run} disabled={!f.notify_url}>Send a test</button><span className={`small ${test.startsWith('✓') ? 'ok' : test.startsWith('✕') ? 'bad' : 'muted'}`}>{test}</span></div>
        </div>
      </Card>
      <Card title="When">
        <div className="grid" style={{ gap: 12 }}>
          <label className="row"><input type="checkbox" checked={f.notify_evening} onChange={e => setF({ ...f, notify_evening: e.target.checked })} style={{ width: 'auto' }} /> Evening: “{f.prompt}” with the day's numbers, unless the day is already written</label>
          <Field label="Evening hour"><Select value={String(f.evening_hour)} onChange={v => setF({ ...f, evening_hour: Number(v) })} options={hours} /></Field>
          <label className="row"><input type="checkbox" checked={f.notify_morning} onChange={e => setF({ ...f, notify_morning: e.target.checked })} style={{ width: 'auto' }} /> Morning: yesterday in one line and “on this day” from earlier years</label>
          <Field label="Morning hour"><Select value={String(f.morning_hour)} onChange={v => setF({ ...f, morning_hour: Number(v) })} options={hours} /></Field>
          <div className="row wrap"><button className="btn primary" onClick={() => save()} disabled={busy}>Save</button><button className="btn sm" onClick={() => send('evening')} disabled={!f.notify_configured}>Send evening now</button><button className="btn sm" onClick={() => send('morning')} disabled={!f.notify_configured}>Send morning now</button>{f.ai_configured && <button className="btn sm" onClick={() => send('weekly')} disabled={!f.notify_configured}>Send week story now</button>}</div>
        </div>
      </Card>
    </div>
  )
}

const AI_MODELS = ['gpt-5.4-mini', 'gpt-5.4', 'gpt-5.4-nano', 'gpt-5.5', 'gpt-4.1-mini', 'gpt-4o-mini']
const FEATURE_LABEL: Record<string, string> = { draft: 'Draft my day', question: "Tonight's question", suggest: 'Suggest highlight & tags', dictation: 'Dictation', 'story-week': 'Week stories', 'story-month': 'Month stories', 'story-year': 'Year stories', ask: 'Ask (answers)', 'ask-plan': 'Ask (planning)' }

function AISettings() {
  const { f, setF, save, busy } = useSettingsForm()
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
            <Field label="API key" help="platform.openai.com → API keys. Any OpenAI-compatible endpoint works with the base URL below."><input value={f.ai_key} onChange={e => setF({ ...f, ai_key: e.target.value })} placeholder="sk-…" autoComplete="off" /></Field>
            <div className="two-col tight">
              <Field label="Model" help={`Default ${f.ai_default_model}: fast and cheap, good enough for every feature here.`}><input list="ai-models" value={f.ai_model} onChange={e => setF({ ...f, ai_model: e.target.value })} placeholder={f.ai_default_model} /><datalist id="ai-models">{AI_MODELS.map(m => <option key={m} value={m} />)}</datalist></Field>
              <Field label="Dictation model"><input value={f.ai_transcribe_model} onChange={e => setF({ ...f, ai_transcribe_model: e.target.value })} placeholder={f.ai_default_transcribe} /></Field>
            </div>
            <div className="two-col tight">
              <Field label="Write in" help="Empty = the language of your own entries."><input value={f.ai_language} onChange={e => setF({ ...f, ai_language: e.target.value })} placeholder="e.g. Serbian (Latin script)" /></Field>
              <Field label="Base URL (optional)"><input value={f.ai_base_url} onChange={e => setF({ ...f, ai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
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
            <label className="row"><input type="checkbox" checked={f.ai_question} onChange={e => setF({ ...f, ai_question: e.target.checked })} style={{ width: 'auto' }} /> Tonight's question on the day page and in the evening push</label>
            <label className="row"><input type="checkbox" checked={f.ai_weekly_story} onChange={e => setF({ ...f, ai_weekly_story: e.target.checked })} style={{ width: 'auto' }} /> Write the week's story every Sunday at the evening hour and push it</label>
            <label className="row"><input type="checkbox" checked={f.ai_send_text} onChange={e => setF({ ...f, ai_send_text: e.target.checked })} style={{ width: 'auto' }} /> Send what I wrote (highlights, text, moments), not only the app data</label>
            <p className="muted small" style={{ margin: 0 }}>What leaves the server: for the days a feature needs, a plain-text summary — places and stops, photo places and times, artists, shows, purchases, habits, books, dishes, weather, moods and tags, and your words when the last option is on. Never photos, audio is sent only while dictating. OpenAI does not train on API data by default.</p>
            <div><button className="btn primary" onClick={() => save()} disabled={busy}>Save</button></div>
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
      <p className="muted">Everything is per person: entries, moments, pins, stories, questions, the facts the connected apps know and the sources, AI key and notifications. Nobody, admins included, sees another account's journal. Admins manage users and the app title.</p>
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
  const { user } = useApp()
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const importJSON = async (file: File) => {
    setBusy(true)
    try { const r = await api.post<Record<string, number>>(`${V1}/import`, JSON.parse(await file.text())); toast('Imported: ' + (Object.entries(r).map(([k, v]) => `${v} ${k}`).join(', ') || 'nothing new')) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="two-col">
      {user?.role === 'admin' && <Card title="Backup">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>): the whole box, every user's journal and facts. Restore by stopping the app and replacing <code>pocket-day.db</code> in the data directory. For your own journal only, use the export.</p>
        <a className="btn primary" href={`${V1}/backup`}>Download database</a>
      </Card>}
      <Card title="Export & import">
        <p className="muted">Your journal (entries, moments, pins) plus your facts as one JSON file, or the words alone as Markdown. Importing merges: entries by date (nothing is overwritten), moments and pins added when missing, facts only for days that have none.</p>
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
