import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { fmtDate, fmtDateTime } from '../lib/format'
import type { Ask as AskT } from '../lib/types'
import { Card, Confirm, Empty, Loading, PageHeader, useAsync, useToast } from '../components/ui'
import { CitedText, DictateButton, useAI } from '../components/ai'
import { Icon } from '../components/icons'

const EXAMPLES = [
  'When was I last at the lake?',
  'What did I cook in the last two weeks?',
  'Which days this month did I feel great, and what did they have in common?',
  'How many days did I walk more than 5 km in September?',
  'What was I listening to on the trip?',
  'What did I spend the most on last week?',
]

export default function Ask() {
  const aiOn = useAI()
  const { toast } = useToast()
  const { data, setData, loading } = useAsync(() => api.get<AskT[]>(`${V1}/ai/asks`), [])
  const [q, setQ] = useState('')
  const [busy, setBusy] = useState(false)
  const [clear, setClear] = useState(false)
  const ask = async (question: string) => {
    if (!question.trim()) return
    setBusy(true)
    try { const a = await api.post<AskT>(`${V1}/ai/asks`, { question }); setData(d => [a, ...(d || [])]); setQ('') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const del = async (id: number) => { await api.del(`${V1}/ai/asks/${id}`); setData(d => (d || []).filter(a => a.id !== id)) }
  if (!aiOn) return <><PageHeader title="Ask your journal" /><Card><Empty icon="sparkles" title="Connect an AI provider">Choose Pocket AI or OpenAI under <Link to="/settings/ai">Settings → AI</Link> and ask things like “when was I last at the lake?”.</Empty></Card></>
  return (
    <>
      <PageHeader title="Ask your journal" sub="Questions about your own days, answered from what you wrote and what the apps recorded." actions={data && data.length > 0 ? <button type="button" className="btn ghost sm" onClick={() => setClear(true)}><Icon name="trash" size={14} />Clear history</button> : undefined} />
      <Card className="mb">
        <form className="ask-form" onSubmit={e => { e.preventDefault(); ask(q) }}>
          <input value={q} onChange={e => setQ(e.target.value)} placeholder="When did I last…?" autoFocus disabled={busy} aria-label="Your question" />
          <DictateButton small onText={t => setQ(t)} />
          <button type="submit" className="btn primary lg" disabled={busy || !q.trim()}><Icon name="sparkles" size={15} />{busy ? 'Reading…' : 'Ask'}</button>
        </form>
        {(!data || data.length === 0) && <div className="chips mt">{EXAMPLES.map(x => <button type="button" key={x} className="chip" onClick={() => ask(x)} disabled={busy}>{x}</button>)}</div>}
        {busy && <p className="muted small mt-sm row"><span className="spinner" />Working out which days to read, then reading them…</p>}
      </Card>
      {loading && !data && <Loading />}
      {data && data.length > 0 && <Card>
        {data.map(a => <div key={a.id} className="ask-item">
          <div className="row"><div className="q grow">{a.question}</div><button type="button" className="btn ghost icon-only sm" onClick={() => ask(a.question)} disabled={busy} title="Ask again" aria-label="Ask again"><Icon name="refresh" size={14} /></button><button type="button" className="btn ghost icon-only sm" onClick={() => del(a.id)} title="Forget" aria-label="Forget"><Icon name="x" size={14} /></button></div>
          <div className="a"><CitedText text={a.answer} /></div>
          {a.dates.length > 0 && <div className="chips">{a.dates.map(d => <Link key={d} className="chip sm" to={`/day/${d}`}>{fmtDate(d)}</Link>)}</div>}
          <div className="muted small mt-sm">Read {a.scope} · {fmtDateTime(a.created_at)}</div>
        </div>)}
      </Card>}
      {clear && <Confirm title="Clear every question?" message="Only the history goes; your journal is untouched." onCancel={() => setClear(false)} onConfirm={async () => { setClear(false); await api.del(`${V1}/ai/asks/0`); setData([]) }} />}
    </>
  )
}
