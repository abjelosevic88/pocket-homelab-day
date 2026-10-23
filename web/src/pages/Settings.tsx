import { PocketModelSelect } from '../components/PocketModelSelect'
import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, BackfillStatus, ConnMode, LocationState, Settings as SettingsT, Source, SourceName, UsageRow, User } from '../lib/types'
import { SOURCE_APP, SOURCE_LABEL } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon, SourceIcon } from '../components/icons'
import { Select } from '../components/Select'
import { LAST_PAGE_KEY } from '../components/Layout'

type TabId = 'general' | 'sources' | 'notifications' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'settings', sub: 'The question on every day, date and time formats, units, home and how far back to pull.' },
  { id: 'sources', label: 'Sources', icon: 'layers', sub: 'The apps a day is written from — through Pocket Core, with your own URL and key, or off.' },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: 'The evening “How was today?” and the morning “On this day”.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Pocket AI or OpenAI for drafts, stories and Ask; an OpenAI key for dictation.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box.' },
  { id: 'tokens', label: 'API & apps', icon: 'key', sub: 'Personal tokens for phone shortcuts, Raycast and a Homepage widget.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Download the database, export or import your journal.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]
// the old ?tab= names
const ALIAS: Record<string, TabId> = { notify: 'notifications', users: 'people', api: 'tokens' }

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const legacy = new URLSearchParams(window.location.search).get('tab') || ''
  const want = raw || legacy
  const tab: TabId = (TABS.find(t => t.id === (ALIAS[want] || want))?.id) || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  const back = () => { const last = sessionStorage.getItem(LAST_PAGE_KEY); navigate(last && !last.startsWith('/settings') ? last : '/') }

  // old links (/settings?tab=sources, ?tab=notify …) land on /settings/<tab>
  useEffect(() => { if (!raw && legacy) navigate(`/settings/${tab}`, { replace: true }) }, [raw, legacy, tab, navigate])
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop, .palette-backdrop')) back() }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back to the journal" title="Back to the journal (esc)" onClick={back}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title">
              <h2>{current.label}</h2>
              <p>{current.sub}</p>
            </div>
            {tab === 'general' && <General />}
            {tab === 'sources' && <Sources />}
            {tab === 'notifications' && <Notify />}
            {tab === 'ai' && <AISettings />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

/** A settings card: title, explanation, actions on the right — Pocket Calendar's section. */
function Section({ title, sub, icon, source, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; source?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {source ? <SourceIcon source={source} size={32} /> : icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

/** "✓ ok" / "✕ failed" test results. */
const TestResult = ({ text }: { text?: string }) => text ? <span className={`small ${text.startsWith('✓') ? 'ok' : text.startsWith('✕') ? 'bad' : 'muted'}`}>{text}</span> : null

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
    <div className="stack loose">
      <Section title="Writing" sub="Yours alone — everyone on the box keeps their own.">
        <SettingRow title="The question above the text box" help="Shown on every day page and in the evening push." stack><input value={f.prompt} onChange={e => setF({ ...f, prompt: e.target.value })} maxLength={120} /></SettingRow>
      </Section>
      <Section title="Date, time & units">
        <SettingRow title="Date format"><Select value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Week starts on"><Segmented value={f.week_start} onChange={v => setF({ ...f, week_start: v as 'mon' | 'sun' })} options={[{ value: 'mon', label: 'Monday' }, { value: 'sun', label: 'Sunday' }]} size="sm" /></SettingRow>
        <SettingRow title="Distances"><Segmented value={f.distance_unit} onChange={v => setF({ ...f, distance_unit: v as 'km' | 'mi' })} options={[{ value: 'km', label: 'km' }, { value: 'mi', label: 'miles' }]} size="sm" /></SettingRow>
      </Section>
      <Section title="Home & weather">
        <SettingRow title="Home location" help="Weather for days spent near home; days Dawarich places far away get the weather there." stack>
          <div>
            <input value={q} onChange={e => setQ(e.target.value)} placeholder={f.home_place ? `${f.home_place} (${f.home_lat.toFixed(3)}, ${f.home_lon.toFixed(3)}) — type to change` : 'Search a town…'} aria-label="Search a town" />
            {hits.length > 0 && <div className="geo-hits">{hits.map((h, i) => <button type="button" key={i} className="geo-hit" onClick={() => { setF({ ...f, home_place: h.name, home_lat: h.lat, home_lon: h.lon }); setQ(''); setHits([]) }}><Icon name="pin" size={15} className="muted" /><span className="grow">{h.name}</span><span className="muted small">{h.region}</span></button>)}</div>}
          </div>
        </SettingRow>
        <SettingRow title="Backfill" help="How many days back to pull facts for on first start and when a new source is connected."><input type="number" min={0} max={3650} value={f.backfill_days} onChange={e => setF({ ...f, backfill_days: Number(e.target.value) })} style={{ width: 110 }} aria-label="Days to backfill" /></SettingRow>
      </Section>
      <Section title="This box" sub={isAdmin ? 'The app title is the same for everyone on this box.' : 'Set by an admin for everyone.'}>
        <SettingRow title="App title" help="Shown in the sidebar and the browser tab."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} disabled={!isAdmin} title={isAdmin ? undefined : 'Only an admin can rename the app'} style={{ width: 220 }} /></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
      <Section title="How it fits together">
        <dl className="set-dl">
          <div><dt>Your words</dt><dd className="muted">One line, a longer note, a mood and tags per day. Moments are short timestamped lines through the day; a phone shortcut can post them.</dd></div>
          <div><dt>Facts</dt><dd className="muted">Everything the other apps know about the day, pulled in and frozen once the day is over. The journal still reads when a source is gone.</dd></div>
          <div><dt>Pins</dt><dd className="muted">Star a photo, a song, a place or a transaction to keep it at the top of the day.</dd></div>
          <div><dt>Memories</dt><dd className="muted">The same date in earlier years shows on the day page and in the morning push.</dd></div>
          <div><dt>Year</dt><dd className="muted">Month by month, moods, most played, most watched, books, places, trips and the days you starred.</dd></div>
          <div><dt>Keys</dt><dd className="muted"><kbd>←</kbd> <kbd>→</kbd> move a day, <kbd>t</kbd> today, <kbd>n</kbd> write, <kbd>m</kbd> a moment, <kbd>⌘K</kbd> search, <kbd>?</kbd> all of them.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

const SOURCE_HELP: Record<SourceName, { what: string; key: string; url: string; extra?: string }> = {
  dawarich: { what: 'Distance, moving time, how you moved and the stops of the day, from Pocket Movement or Dawarich.', key: 'API key from Dawarich → Account', url: 'https://dawarich.example.com' },
  immich: { what: 'Photos and videos taken that day, with a thumbnail grid proxied through this app.', key: 'API key from Immich → Account settings → API keys', url: 'http://host.docker.internal:2283' },
  maloja: { what: 'What played: listens, top artists, the full track list.', key: 'no key needed (read API is open)', url: 'http://host.docker.internal:42010' },
  jellystat: { what: 'What you watched in Jellyfin: episodes, films, minutes.', key: 'API key from Jellystat → Settings → API keys', url: 'http://host.docker.internal:3000', extra: 'Jellyfin user name to keep (empty = everyone)' },
  firefly: { what: 'Withdrawals of the day with categories and totals per currency, read from Pocket Money’s Firefly-compatible API.', key: 'Direct: a token from Pocket Money → Settings → API tokens with the URL ending in /firefly (a Firefly III URL with its personal access token works too)', url: 'http://host.docker.internal:8178/firefly' },
  habits: { what: 'The check-in: habits done, tracker values, the journal line.', key: 'Token from Pocket Habits → Settings → API tokens', url: 'http://host.docker.internal:8169' },
  bookshelf: { what: 'Books being read, started or finished.', key: 'Token from Pocket Bookshelf → Settings → API', url: 'http://host.docker.internal:8171' },
  travel: { what: 'Which trip the day fell in, and which day of it.', key: 'Token from Pocket Travel → Settings → API', url: 'http://host.docker.internal:8174' },
  kitchen: { what: 'What was cooked.', key: 'Token from Pocket Kitchen → Settings → API & apps', url: 'http://host.docker.internal:8176' },
  garage: { what: 'Fill-ups and service records.', key: 'Token from Pocket Garage → Settings → API', url: 'http://host.docker.internal:8175' },
  weather: { what: 'Daily weather from Open-Meteo for the home location, or where your location source says you were.', key: 'no key needed', url: '' },
  pinepods: { what: 'Podcast episodes played, with minutes per day. Episodes spread over several days count on each of them (from the day Pocket Day starts watching).', key: 'API key from PinePods → Settings → API keys', url: 'http://host.docker.internal:8136' },
  audiobookshelf: { what: 'Listening sessions — audiobooks and ABS podcasts — per title, with minutes. Sessions under a minute are ignored.', key: 'An admin API key (Settings → API Keys) so every user’s sessions are visible', url: 'http://host.docker.internal:13378', extra: 'Only these users (comma separated; empty = everyone)' },
  jellyseerr: { what: 'Films and series requested, and requested titles that became available.', key: 'API key from Jellyseerr → Settings → General', url: 'http://host.docker.internal:5055' },
  forgejo: { what: 'Your pushes per repository: commits, the latest message, new repositories, tags and pull requests.', key: 'A read-only access token (Settings → Applications, scopes read:user + read:repository)', url: 'http://host.docker.internal:3000', extra: 'User name (empty = the token’s owner)' },
  nextcloud: { what: 'Tasks completed (every task list), events and contact birthdays, over CalDAV.', key: 'An app password (Personal settings → Security)', url: 'http://host.docker.internal:8080', extra: 'Nextcloud user name' },
  documents: { what: 'Documents added that day, with correspondent and type, each linking to the document in Pocket Documents.', key: 'Token from Pocket Documents → Settings → API tokens', url: 'http://host.docker.internal:8183' },
  invoicing: { what: 'Invoices issued and payments received, with amounts.', key: 'Token from Pocket Invoicing → Settings → API', url: 'http://host.docker.internal:8168' },
  split: { what: 'Shared expenses dated that day, across every group.', key: 'Token from Pocket Split → Settings → API', url: 'http://host.docker.internal:8172' },
  tasks: { what: 'Tasks completed on each day.', key: 'Token from Pocket Tasks → Settings → API tokens', url: 'http://pocket-tasks:8080' },
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
  const stat = (which: string) => <TestResult text={test[which]} />
  const upd = (name: SourceName, patch: Partial<Source>) => setF({ ...f, sources: { ...f.sources, [name]: { ...f.sources[name], ...patch } } })
  const saveSource = (name: SourceName, patch: Partial<Source>) => save({ sources: { ...withoutModes(f.sources), [name]: { ...f.sources[name], mode: undefined, ...patch } } })
  const backfill = async (force: boolean) => {
    try { const r = await api.post<{ ok: boolean; error?: string }>(`${V1}/facts/backfill`, { force }); toast(r.ok ? 'Backfill started' : r.error || 'failed', r.ok ? 'ok' : 'err'); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const bf = status?.backfill
  const core = !!f.core_connected
  const title = (name: SourceName) => name === 'dawarich' ? <>Location <span className="muted">— where you were</span></> : <>{SOURCE_APP[name]} <span className="muted">— {SOURCE_LABEL[name].toLowerCase()}</span></>
  const testBtn = (name: SourceName, src: Source) => <><button type="button" className="btn sm" onClick={() => run(name, src)}>Test connection</button>{stat(name)}</>
  return (
    <div className="stack loose">
      <Section title="Pulling the days" icon="refresh" actions={bf?.running ? <span className="badge cat">running</span> : undefined}
        sub={<>Today and yesterday are pulled every 30 minutes. Older days are frozen once fetched. {status ? <>{status.days_with_facts} days have facts{status.first ? ` (${status.first} → ${status.last})` : ''}.</> : null}</>}>
        <div className="set-backfill">
          {bf?.running && <div className="progress"><div style={{ width: `${bf.total ? bf.done / bf.total * 100 : 0}%` }} /><span className="small muted">{bf.done}/{bf.total} · {bf.current}</span></div>}
          <p className="muted small" style={{ margin: 0 }}>The sources are yours: what you connect here is pulled with your own accounts and only ever shows up in your journal. The Pocket apps can come through Pocket Core, as you, with nothing to paste; so can the other services (Immich, Maloja, Dawarich…), with your own account there from Pocket Core.</p>
          {!core && <CoreHint />}
          <div className="row wrap"><button type="button" className="btn sm" onClick={() => backfill(false)} disabled={bf?.running}><Icon name="download" size={14} />Backfill missing days ({f.backfill_days} back)</button><button type="button" className="btn ghost sm" onClick={() => backfill(true)} disabled={bf?.running} title="Re-fetch even days that already have facts"><Icon name="refresh" size={14} />Re-pull everything</button></div>
        </div>
      </Section>
      {f.source_names.map(name => {
        const src = f.sources[name]
        const help = SOURCE_HELP[name]
        if (src.pocket) return <Section key={name} source={name} title={title(name)} sub={help.what} actions={connBadge(src.mode || 'off', src.via)}>
          <Connection app={src.app || SOURCE_APP[name]} mode={src.mode || 'off'} via={src.via} detail={src.detail || ''} core={core} hasToken={!!src.key || !!src.url}
            onMode={m => saveSource(name, { mode: m })} onDisconnect={() => saveSource(name, { url: '', key: '', public_url: '' })}
            test={testBtn(name, src)}
            direct={<>
              <div className="two-col tight">
                <Field label="URL" help={name === 'firefly' ? 'Pocket Money’s URL followed by /firefly. Inside Docker use http://host.docker.internal:PORT.' : 'Inside Docker use http://host.docker.internal:PORT or the service name.'}><input value={src.url} onChange={e => upd(name, { url: e.target.value })} placeholder={help.url} /></Field>
                <Field label="API token" help={help.key}><input value={src.key} onChange={e => upd(name, { key: e.target.value })} autoComplete="off" /></Field>
              </div>
              <Field label="Public URL (optional)" help="What the browser opens for the open-in-app links when the URL above is internal."><input value={src.public_url} onChange={e => upd(name, { public_url: e.target.value })} placeholder="https://…" /></Field>
            </>} />
        </Section>
        if (src.service) {
          // Non-credential settings (user filters, the Forgejo user) apply in every mode; the Nextcloud user name is part of the credential.
          const credExtra = name === 'nextcloud'
          const extraField = help.extra && !credExtra ? <Field label={help.extra}><input value={src.extra} onChange={e => upd(name, { extra: e.target.value })} /></Field> : null
          return <Section key={name} source={name} title={title(name)} sub={help.what} actions={src.location ? locationBadge(src.location, src.mode || 'off', src.via) : connBadge(src.mode || 'off', src.via)}>
            {src.location && <LocationChoice loc={src.location} core={core} onPick={p => saveSource(name, { provider: p })} test={testBtn('movement' as SourceName, src)} />}
            {/* One source, one card: while Pocket Movement answers, Dawarich's connection stays out of the way. */}
            {!(src.location?.provider === 'movement' && src.location.movement_via) && <Connection app={src.app || SOURCE_APP[name]} mode={src.mode || 'off'} via={src.via} detail={src.detail || ''} core={core} hasToken={!!src.key || !!src.url}
              service keySource={src.key_source || ''} keyless={!src.needs_key}
              onMode={m => saveSource(name, { mode: m })} onDisconnect={() => saveSource(name, { url: '', key: '', public_url: '', ...(credExtra ? { extra: '' } : {}) })}
              extra={extraField ? <><div className="two-col tight">{extraField}</div><div><button type="button" className="btn sm" onClick={() => saveSource(name, { extra: src.extra })} disabled={busy}>Save</button></div></> : undefined}
              test={testBtn(name, src)}
              direct={<>
                <div className={credExtra ? 'three-col tight' : 'two-col tight'}>
                  <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={src.url} onChange={e => upd(name, { url: e.target.value })} placeholder={help.url} /></Field>
                  {src.needs_key && <Field label={name === 'nextcloud' ? 'App password' : 'API key / token'} help={help.key}><input value={src.key} onChange={e => upd(name, { key: e.target.value })} autoComplete="off" /></Field>}
                  {credExtra && <Field label={help.extra || 'User name'}><input value={src.extra} onChange={e => upd(name, { extra: e.target.value })} /></Field>}
                </div>
                <Field label="Public URL (optional)" help="What the browser opens for the open-in-app links when the URL above is internal."><input value={src.public_url} onChange={e => upd(name, { public_url: e.target.value })} placeholder="https://…" /></Field>
              </>} />}
          </Section>
        }
        return <Section key={name} source={name} title={title(name)} sub={help.what}
          actions={<span className={`badge ${src.configured ? 'ok' : ''}`}>{src.configured ? 'connected' : 'set a home location'}</span>}>
          <SettingRow title="Pull the weather" help={!f.home_lat && !f.home_lon ? 'Set the home location under General first.' : 'Saved with “Save sources” below.'}>
            <Switch checked={src.enabled} onChange={v => upd(name, { enabled: v })} label="Pull the weather" />
          </SettingRow>
          <div className="row"><button type="button" className="btn sm" onClick={() => run(name, src)} disabled={!f.home_lat && !f.home_lon}>Test</button>{stat(name)}</div>
        </Section>
      })}
      <div className="row wrap"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save sources</button><span className="muted small">A newly connected source is pulled for today on the next tick; use Backfill for the past.</span></div>
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
  // Pocket Core saved earlier on an app that is not connected to one: nothing is read, say so.
  const stranded = !core && mode === 'core'
  if (stranded) mode = 'off'
  const ok = via !== ''
  const [advanced, setAdvanced] = useState(mode === 'direct')
  const showDirect = !core || mode === 'direct' || advanced
  const cred = keyless ? 'URL' : service ? 'URL and key' : 'URL and token'
  const whose = keySource === 'personal' ? (keyless ? ' (your own entry)' : ' (your own key)') : keySource === 'box' ? (keyless ? " (the box's address)" : " (the box's key)") : ''
  const coreLine = service
    ? `Connected through Pocket Core${whose}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.`
    : 'Connected through Pocket Core, as you. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.'
  const line = stranded ? `Pocket Core was chosen, but this app is not connected to one, so Day does not read ${app}. Use Direct with your own ${cred}.`
    : mode === 'off' ? `Off: Day does not read ${app}.`
    : mode === 'core' ? (ok ? coreLine : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} ${cred}; use Test connection to check it.` : `Not connected: ${detail || `add the ${cred}`}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', disabled: !core, title: core ? 'Through Pocket Core: per person, nothing to paste' : 'Needs Pocket Core' },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: `Your own ${cred}` }] : []),
          { value: 'off', label: 'Off' },
        ]} />
      </div>
      {!core && <p className="core-hint">Pocket Core is greyed out: it needs Pocket Core (see the note at the top).</p>}
      {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL{keyless ? '' : ' and key'}</button>}
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button type="button" className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
      {mode !== 'off' && extra}
    </div>
  )
}

// The words every Core-only choice shows while this app is not connected to a Pocket Core.
function CoreHint({ what = 'This choice' }: { what?: string }) {
  return <p className="core-hint">{what} needs Pocket Core. Pocket Core connects your Pocket apps to each other and to your services: one sign-in for everyone, each person's own accounts, nothing to copy or paste. Set CORE_URL to connect this app. <a href="https://github.com/abjelosevic88/pocket-homelab-core" target="_blank" rel="noreferrer">Learn more</a></p>
}

// LocationChoice is where the location source's days come from: Pocket
// Movement (the person's own location history, through Pocket Core only) or
// Dawarich (its connection below). While Movement is chosen, a connected
// Dawarich fills in when Movement does not answer.
function LocationChoice({ loc, core, onPick, test }: { loc: LocationState; core: boolean; onPick: (p: 'movement' | 'dawarich') => void; test: React.ReactNode }) {
  const movement = loc.provider === 'movement'
  const fill = loc.dawarich_via ? ' If it ever does not answer, your Dawarich fills in.' : ''
  const line = movement
    ? (loc.movement_via ? `Your days come from Pocket Movement, as you, through Pocket Core.${fill}` : `Pocket Movement is not connected: ${loc.movement_detail || 'unknown'}.${loc.dawarich_via ? ' Dawarich below fills in meanwhile.' : ''}`)
    : !core && loc.choice === 'movement' ? 'Pocket Movement was chosen, but this app is not connected to a Pocket Core, so your days come from Dawarich below.'
    : `Your days come from Dawarich${loc.dawarich_via ? '' : ' once its connection below works'}.`
  const ok = movement ? !!loc.movement_via : !!loc.dawarich_via
  return (
    <div className="loc-block">
      <span className="lbl">Where you were, from</span>
      <div className="conn-mode">
        <Segmented size="sm" label="Location source" value={loc.provider} onChange={p => onPick(p as 'movement' | 'dawarich')} options={[
          { value: 'movement', label: 'Pocket Movement', disabled: !core, title: core ? 'Your own location history, through Pocket Core' : 'Needs Pocket Core' },
          { value: 'dawarich', label: 'Dawarich', title: 'Dawarich, connected below' },
        ]} />
      </div>
      {!core && <CoreHint what="Pocket Movement" />}
      <div className={`conn-status ${ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {movement && <div className="row wrap">{test}</div>}
      {!(movement && loc.movement_via) && <span className="lbl">Dawarich connection{movement ? ' (filling in meanwhile)' : ''}</span>}
    </div>
  )
}

const locationBadge = (loc: LocationState, mode: ConnMode, via: string) => loc.provider === 'movement' && loc.movement_via
  ? <span className="badge ok">Pocket Movement</span>
  : loc.provider === 'movement' && via ? <span className="badge warn">Dawarich (fallback)</span>
  : connBadge(mode, via)

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
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
    <div className="stack loose">
      <Section title="Where to send" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'connected' : 'not set'}</span>}
        sub={<>Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/day" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
        <SettingRow title="Check it" help="Sends a test message to the URL above (before saving)."><button type="button" className="btn sm" onClick={run} disabled={!f.notify_url}><Icon name="mail" size={14} />Send a test</button><TestResult text={test} /></SettingRow>
      </Section>
      <Section title="When" icon="clock">
        <SettingRow title="Evening" help={<>“{f.prompt}” with the day's numbers, unless the day is already written.</>}><Switch checked={f.notify_evening} onChange={v => setF({ ...f, notify_evening: v })} label="Evening push" /></SettingRow>
        <SettingRow title="Evening hour"><Select value={String(f.evening_hour)} onChange={v => setF({ ...f, evening_hour: Number(v) })} options={hours} /></SettingRow>
        <SettingRow title="Morning" help="Yesterday in one line and “on this day” from earlier years."><Switch checked={f.notify_morning} onChange={v => setF({ ...f, notify_morning: v })} label="Morning push" /></SettingRow>
        <SettingRow title="Morning hour"><Select value={String(f.morning_hour)} onChange={v => setF({ ...f, morning_hour: Number(v) })} options={hours} /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => send('evening')} disabled={!f.notify_configured}><Icon name="moon" size={15} />Send evening now</button>
        <button type="button" className="btn" onClick={() => send('morning')} disabled={!f.notify_configured}><Icon name="sun" size={15} />Send morning now</button>
        {f.ai_configured && <button type="button" className="btn" onClick={() => send('weekly')} disabled={!f.notify_configured}><Icon name="book" size={15} />Send week story now</button>}
      </div>
    </div>
  )
}

const AI_MODELS = ['gpt-5.4-mini', 'gpt-5.4', 'gpt-5.4-nano', 'gpt-5.5', 'gpt-4.1-mini', 'gpt-4o-mini']
const FEATURE_LABEL: Record<string, string> = { draft: 'Draft my day', question: "Tonight's question", suggest: 'Suggest highlight & tags', dictation: 'Dictation', 'story-week': 'Week stories', 'story-month': 'Month stories', 'story-year': 'Year stories', ask: 'Ask (answers)', 'ask-plan': 'Ask (planning)' }

function AISettings() {
  const [showDictation, setShowDictation] = useState(false)
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
    <div className="stack loose" style={{ gridTemplateColumns: 'minmax(0, 1fr)' }}>
      <Section title="AI provider" icon="sparkles" actions={<span className={`badge ${f.ai_configured ? 'ok' : ''}`}>{f.ai_configured ? 'connected' : 'not set'}</span>}
        sub="Automatic uses Pocket AI when its Core link is enabled. You can also choose your own OpenAI-compatible endpoint.">
        <div className="stack">
          <Field group label="Text generation" help="Automatic prefers your enabled Pocket AI connection. Without one, it uses your saved OpenAI settings."><Select ariaLabel="Text generation provider" value={f.ai_provider || 'auto'} onChange={v => { setF({ ...f, ai_provider: v }); setShowDictation(false) }} options={[{ value: 'auto', label: 'Automatic — prefer Pocket AI' }, { value: 'pocket', label: 'Pocket AI', disabled: !f.core_connected, hint: f.core_connected ? undefined : 'needs Pocket Core' }, { value: 'openai', label: 'OpenAI / compatible API' }]} /></Field>
          {!f.core_connected && <CoreHint what="Pocket AI" />}
          {!f.core_connected && f.ai_provider === 'pocket' && <p className="muted small">Pocket AI was chosen, but this app is not connected to a Pocket Core, so text generation is off. Choose OpenAI / compatible API, or Automatic.</p>}
          <p className="muted small" role="status">Saved selection: {f.ai_status?.detail}. Pocket AI uses your connected Claude account.</p>
          {f.ai_provider !== 'openai' && !!f.core_connected && <PocketModelSelect value={f.ai_pocket_model || ''} onChange={v => setF({ ...f, ai_pocket_model: v })} />}
          {f.ai_provider === 'openai' && <div className="stack">
            <Field label="OpenAI API key" help="platform.openai.com → API keys."><input value={f.ai_key} onChange={e => setF({ ...f, ai_key: e.target.value })} placeholder="sk-…" autoComplete="off" /></Field>
            <Field label="Model" help={`Default ${f.ai_default_model}: fast and cheap, good enough for every feature here.`}><input list="ai-models" value={f.ai_model} onChange={e => setF({ ...f, ai_model: e.target.value })} placeholder={f.ai_default_model} /><datalist id="ai-models">{AI_MODELS.map(m => <option key={m} value={m} />)}</datalist></Field>
            <Field label="Base URL (optional)"><input value={f.ai_base_url} onChange={e => setF({ ...f, ai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            <div className="row"><button type="button" className="btn sm" onClick={run} disabled={!f.ai_key}>Test key & model</button><TestResult text={test} /></div>
          </div>}
          <Field label="Write in" help="Empty = the language of your own entries."><input value={f.ai_language} onChange={e => setF({ ...f, ai_language: e.target.value })} placeholder="e.g. Serbian (Latin script)" /></Field>
        </div>
      </Section>
      <Section title="Dictation" icon="mic" sub="Voice transcription uses OpenAI separately from text generation. Pocket AI does not transcribe audio.">
        <button type="button" className="btn sm" aria-expanded={showDictation} aria-controls="dictation-settings" onClick={() => setShowDictation(v => !v)}>{showDictation ? 'Hide dictation settings' : 'Configure dictation'}</button>
        {showDictation && <div id="dictation-settings" className="stack mt-sm">
          {f.ai_provider !== 'openai' && <>
            <Field label="OpenAI API key" help="platform.openai.com → API keys."><input value={f.ai_key} onChange={e => setF({ ...f, ai_key: e.target.value })} placeholder="sk-…" autoComplete="off" /></Field>
            <Field label="Base URL (optional)"><input value={f.ai_base_url} onChange={e => setF({ ...f, ai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
          </>}
          <Field label="Dictation model"><input value={f.ai_transcribe_model} onChange={e => setF({ ...f, ai_transcribe_model: e.target.value })} placeholder={f.ai_default_transcribe} /></Field>
        </div>}
      </Section>
      <Section title="Options" sub={<>What leaves the server: for the days a feature needs, a plain-text summary — places and stops, photo places and times, artists, shows, purchases, habits, books, dishes, weather, moods and tags, and your words when the last option is on. Never photos; audio is sent to your OpenAI endpoint only while dictating. Text goes to the selected AI provider.</>}>
        <SettingRow title="Tonight's question" help="On the day page and in the evening push."><Switch checked={f.ai_question} onChange={v => setF({ ...f, ai_question: v })} label="Tonight's question" /></SettingRow>
        <SettingRow title="Week story on Sunday" help="Written at the evening hour every Sunday and pushed."><Switch checked={f.ai_weekly_story} onChange={v => setF({ ...f, ai_weekly_story: v })} label="Week story on Sunday" /></SettingRow>
        <SettingRow title="Send what I wrote" help="Highlights, text and moments, not only the app data."><Switch checked={f.ai_send_text} onChange={v => setF({ ...f, ai_send_text: v })} label="Send what I wrote" /></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
      <Section title="What it does">
        <dl className="set-dl">
          <div><dt>Draft my day</dt><dd className="muted">A first-person draft from the facts, moments and pins — or only what your text is missing. You edit it.</dd></div>
          <div><dt>Tonight's question</dt><dd className="muted">One specific question about the day replaces the generic prompt, here and in the evening push.</dd></div>
          <div><dt>Suggest</dt><dd className="muted">A highlight, tags (from your own vocabulary) and a mood from what you wrote.</dd></div>
          <div><dt>Dictation</dt><dd className="muted">Speak the entry or a moment; it is transcribed and added.</dd></div>
          <div><dt>Stories</dt><dd className="muted">Week, month and year written up as a story — on the Calendar, the Year page and under Stories. A Sunday push with the week.</dd></div>
          <div><dt>Ask</dt><dd className="muted">“When was I last at the lake?” — answered from your days, with links to them.</dd></div>
        </dl>
      </Section>
      <Section title="Usage" icon="activity">
        {usage && (usage.all_time.length > 0 ? <>
          <div style={{ overflowX: 'auto' }}><table className="table compact">
            <thead><tr><th>Feature</th><th className="right">Calls</th><th className="right">Tokens in</th><th className="right">Tokens out</th></tr></thead>
            <tbody>{usage.last_30_days.map(r => <tr key={r.feature + r.model}><td>{FEATURE_LABEL[r.feature] || r.feature} <span className="muted small">{r.model}</span></td><td className="right">{r.calls}</td><td className="right">{r.prompt_tokens.toLocaleString()}</td><td className="right">{r.completion_tokens.toLocaleString()}</td></tr>)}</tbody>
          </table></div>
          <p className="muted small mt-sm" style={{ margin: '8px 0 0' }}>Last 30 days: {m30.calls} calls, {(m30.p + m30.c).toLocaleString()} tokens. All time: {all.calls} calls, {(all.p + all.c).toLocaleString()} tokens.</p>
        </> : <p className="muted" style={{ margin: 0 }}>Nothing used yet.</p>)}
      </Section>
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
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everything is per person: entries, moments, pins, stories, questions, the facts the connected apps know and the sources, AI key and notifications. Nobody, admins included, sees another account's journal. Admins manage accounts and the app title.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account and their journal are deleted." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
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
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="stack">
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="submit" className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
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
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
        <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button type="submit" className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
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
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and write your journal, feed a Homepage widget (<code>GET /api/v1/dashboard</code>) or drive a phone shortcut. See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. iPhone shortcut)" /><button type="submit" className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted" style={{ margin: 0 }}>No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Shortcuts & widgets" icon="smartphone" sub={<>Handy endpoints at <code>{base}/api/v1</code>:</>}>
        <dl className="set-dl">
          <div><dt><code>POST /moments</code></dt><dd className="muted"><code>{'{"text":"coffee with A.","kind":"note"}'}</code> — a Siri Shortcut or Raycast “add a moment”; time defaults to now</dd></div>
          <div><dt><code>POST /entries</code></dt><dd className="muted"><code>{'{"highlight":"…","text":"…","mood":4,"tags":"walk"}'}</code> — appends to today; <code>date</code> optional</dd></div>
          <div><dt><code>GET /today</code></dt><dd className="muted">today's page: your words, moments, pins and every fact</dd></div>
          <div><dt><code>GET /dashboard</code></dt><dd className="muted">streak, written today, photos / km / listens today — for Homepage</dd></div>
          <div><dt><code>GET /memories</code></dt><dd className="muted">this date in earlier years</dd></div>
          <div><dt><code>GET /export.md</code></dt><dd className="muted">the whole journal as Markdown</dd></div>
        </dl>
      </Section>
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
    <div className="stack loose">
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="Admins only.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>): the whole box, every user's journal and facts. Restore by stopping the app and replacing <code>pocket-day.db</code> in the data directory. For your own journal only, use the export.</p>
        <a className="btn primary" href={`${V1}/backup`}><Icon name="download" size={15} />Download database</a>
      </Section>}
      <Section title="Export & import" icon="archive" sub="Your journal (entries, moments, pins) plus your facts as one JSON file, or the words alone as Markdown. Importing merges: entries by date (nothing is overwritten), moments and pins added when missing, facts only for days that have none.">
        <div className="row wrap">
          <a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a>
          <a className="btn" href={`${V1}/export.json?nofacts=1`}>JSON without facts</a>
          <a className="btn" href={`${V1}/export.md`}><Icon name="text" size={15} />Markdown</a>
          <label className="btn"><Icon name="upload" size={15} />{busy ? 'Importing…' : 'Import JSON…'}<input type="file" accept="application/json,.json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label>
        </div>
      </Section>
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const tiles: [string, number | undefined, string][] = [['Entries', counts.entries, 'book-open'], ['Moments', counts.moments, 'clock'], ['Pins', counts.pins, 'star'], ['Days with facts', Number(data.days_with_facts) || 0, 'layers']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    ['Rows', `${counts.facts ?? 0} facts${data.first_fact_day ? ` since ${String(data.first_fact_day)}` : ''} · ${counts.users ?? 0} users`],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Day</h3>
          <p className="muted small">A journal that writes itself: one page per day with your own words on top and, underneath, what the other apps on the server already know about it — where you were, the photos, what played, what you watched, read, cooked and spent. One binary, one SQLite file, one volume.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{n ?? 0}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <p className="muted small" style={{ margin: 0 }}>Open source (MIT). Looking for a day? <Link to="/journal">Journal</Link> searches your words; <kbd>⌘K</kbd> anywhere. Sibling projects: Pocket Habits, Pocket Documents, Pocket Inventory, Pocket Bookshelf, Pocket Split, Pocket Clips, Pocket Travel, Pocket Garage, Pocket Kitchen.</p>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => window.dispatchEvent(new CustomEvent('pd:shortcuts'))}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-day" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
    </div>
  )
}
