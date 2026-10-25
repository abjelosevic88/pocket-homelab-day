import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { weekday } from '../lib/format'
import { MOODS, MOOD_ICON, tagList, type Entry } from '../lib/types'
import { Card, Empty, Loading, PageHeader, Segmented, useAsync, useDebounce } from '../components/ui'
import { Icon, SourceIcon } from '../components/icons'

interface Resp { entries: Entry[]; total: number; summaries: Record<string, Record<string, string>>; limit: number; offset: number }

export default function Journal() {
  const [sp, setSp] = useSearchParams()
  const [q, setQ] = useState(sp.get('q') || '')
  const dq = useDebounce(q, 300)
  const tag = sp.get('tag') || ''
  const mood = Number(sp.get('mood') || 0)
  const pinned = sp.get('pinned') === '1'
  const [page, setPage] = useState(0)
  const limit = 20
  useEffect(() => { setPage(0) }, [dq, tag, mood, pinned])
  const { data, loading, error } = useAsync(() => api.get<Resp>(`${V1}/entries?q=${encodeURIComponent(dq)}&tag=${encodeURIComponent(tag)}&mood=${mood}&pinned=${pinned ? 1 : 0}&limit=${limit}&offset=${page * limit}`), [dq, tag, mood, pinned, page])
  const { data: tags } = useAsync(() => api.get<{ tag: string; count: number }[]>(`${V1}/tags`), [])
  const set = (k: string, v: string) => { const n = new URLSearchParams(sp); if (v) n.set(k, v); else n.delete(k); setSp(n) }
  const pages = data ? Math.ceil(data.total / limit) : 0
  return (
    <>
      <PageHeader title="Journal" sub={data ? `${data.total} day${data.total === 1 ? '' : 's'} written` : ''} actions={<Link className="btn primary" to="/" state={{ focus: 'entry' }}><Icon name="pen" size={15} />Write today</Link>} />
      <div className="toolbar">
        <div className="search"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Search words, tags, moments…" aria-label="Search the journal" /></div>
        <Segmented size="sm" value={mood} onChange={v => set('mood', v ? String(v) : '')} options={[{ value: 0, label: 'Any mood' }, ...[1, 2, 3, 4, 5].map(m => ({ value: m, label: MOOD_ICON[m], title: MOODS[m] }))]} />
        <button type="button" className={`btn sm ${pinned ? 'primary' : ''}`} onClick={() => set('pinned', pinned ? '' : '1')} aria-pressed={pinned}><Icon name="star" size={14} />Kept</button>
        {tag && <button type="button" className="chip on" onClick={() => set('tag', '')} title="Clear the tag">#{tag}<Icon name="x" size={13} /></button>}
      </div>
      {tags && tags.length > 0 && !tag && <div className="chips mb">{tags.slice(0, 20).map(t => <button type="button" key={t.tag} className="chip" onClick={() => set('tag', t.tag)}>#{t.tag} <span className="muted">{t.count}</span></button>)}</div>}
      {loading && !data && <Loading />}
      {error && <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>}
      {data && data.entries.length === 0 && <Card><Empty icon={dq || tag || mood || pinned ? 'search' : 'book-open'} title={dq || tag || mood || pinned ? 'Nothing matches' : 'Nothing written yet'}>{dq || tag || mood || pinned ? 'Try fewer filters.' : 'Open today and write one line. The rest of the day fills itself in.'}</Empty></Card>}
      <div className="journal">
        {data?.entries.map(e => <EntryRow key={e.id} e={e} sums={data.summaries[e.date]} />)}
      </div>
      {pages > 1 && <div className="row mt" style={{ justifyContent: 'center' }}><button type="button" className="btn sm" disabled={page === 0} onClick={() => setPage(p => p - 1)}><Icon name="chevron-left" size={14} />Newer</button><span className="muted small">{page + 1} / {pages}</span><button type="button" className="btn sm" disabled={page + 1 >= pages} onClick={() => setPage(p => p + 1)}>Older<Icon name="chevron-right" size={14} /></button></div>}
    </>
  )
}

function EntryRow({ e, sums }: { e: Entry; sums?: Record<string, string> }) {
  const order = ['travel', 'dawarich', 'immich', 'maloja', 'jellystat', 'bookshelf', 'kitchen', 'firefly', 'habits', 'health', 'still', 'weather']
  return (
    <Link to={`/day/${e.date}`} className="jrow" style={{ color: 'inherit' }}>
      <div className="jdate"><div className="d">{e.date.slice(8)}</div><div className="m">{monthShort(e.date)} {e.date.slice(0, 4) !== String(new Date().getFullYear()) ? e.date.slice(0, 4) : ''}</div><div className="muted small">{weekday(e.date)}</div></div>
      <div className="jbody">
        <div className="row" style={{ gap: 8 }}>{e.mood > 0 && <span title={MOODS[e.mood]}>{MOOD_ICON[e.mood]}</span>}<b className="grow">{e.highlight || (e.text ? firstLine(e.text) : <span className="muted">(no words)</span>)}</b>{e.origin === 'habits' && <span className="chip sm" title="Taken from the Pocket Habits journal"><Icon name="check-circle" size={12} />Habits</span>}{e.pinned && <span title="Kept" style={{ color: 'var(--star)', display: 'inline-grid' }}><Icon name="star" size={14} strokeWidth={2.2} /></span>}</div>
        {e.highlight && e.text && <div className="muted clamp2">{e.text}</div>}
        <div className="row wrap small muted" style={{ gap: 6, marginTop: 6 }}>
          {tagList(e.tags).map(t => <span key={t} className="chip sm">#{t}</span>)}
          {(e.moments || 0) > 0 && <span className="row" style={{ gap: 3 }}><Icon name="clock" size={12} />{e.moments} moment{e.moments === 1 ? '' : 's'}</span>}
          {e.words ? <span>{e.words} words</span> : null}
        </div>
        {sums && <div className="jsums">{order.filter(s => sums[s]).map(s => <span key={s} title={s}><SourceIcon source={s} size={13} bare />{sums[s]}</span>)}</div>}
      </div>
    </Link>
  )
}

function firstLine(t: string) { return t.split('\n')[0] }
function monthShort(d: string) { const [y, m] = d.split('-').map(Number); return new Date(y, m - 1, 1).toLocaleDateString(undefined, { month: 'short' }) }
