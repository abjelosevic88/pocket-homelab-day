import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { addDays, fmtDateTime, todayISO } from '../lib/format'
import type { Story } from '../lib/types'
import { Card, Empty, PageHeader, useAsync } from '../components/ui'
import { StoryCard, useAI } from '../components/ai'
import { Icon } from '../components/icons'

function isoWeek(iso: string): string {
  const [y, m, d] = iso.split('-').map(Number)
  const t = new Date(Date.UTC(y, m - 1, d))
  const day = t.getUTCDay() || 7
  t.setUTCDate(t.getUTCDate() + 4 - day)
  const y0 = new Date(Date.UTC(t.getUTCFullYear(), 0, 1))
  const w = Math.ceil(((t.getTime() - y0.getTime()) / 86400000 + 1) / 7)
  return `${t.getUTCFullYear()}-W${String(w).padStart(2, '0')}`
}
const KIND_LABEL: Record<string, string> = { week: 'Week', month: 'Month', year: 'Year' }

export default function Stories() {
  const aiOn = useAI()
  const [sp, setSp] = useSearchParams()
  const today = todayISO()
  const choices = useMemo(() => {
    const lastMonth = new Date(Number(today.slice(0, 4)), Number(today.slice(5, 7)) - 2, 1)
    const lm = `${lastMonth.getFullYear()}-${String(lastMonth.getMonth() + 1).padStart(2, '0')}`
    return [
      { kind: 'week' as const, period: isoWeek(today), label: 'this week' },
      { kind: 'week' as const, period: isoWeek(addDays(today, -7)), label: 'last week' },
      { kind: 'month' as const, period: today.slice(0, 7), label: 'this month' },
      { kind: 'month' as const, period: lm, label: 'last month' },
      { kind: 'year' as const, period: today.slice(0, 4), label: 'this year' },
    ]
  }, [today])
  const kind = (sp.get('kind') as 'week' | 'month' | 'year') || 'week'
  const period = sp.get('period') || choices[0].period
  const label = choices.find(c => c.kind === kind && c.period === period)?.label || `${KIND_LABEL[kind].toLowerCase()} ${period}`
  const { data: list } = useAsync(() => api.get<Story[]>(`${V1}/ai/stories`), [kind, period])
  const [, force] = useState(0)
  if (!aiOn) return <><PageHeader title="Stories" /><Card><Empty icon="sparkles" title="Connect an AI provider">Choose Pocket AI or OpenAI under <Link to="/settings/ai">Settings → AI</Link> and every week, month and year can be written up as a story.</Empty></Card></>
  return (
    <>
      <PageHeader title="Stories" sub="Your weeks, months and years, written up from your words and what the apps recorded." />
      <div className="chips mb">{choices.map(c => <button type="button" key={c.kind + c.period} className={`chip ${c.kind === kind && c.period === period ? 'active' : ''}`} onClick={() => setSp({ kind: c.kind, period: c.period })}>{c.label[0].toUpperCase() + c.label.slice(1)} <span className="muted">{c.period}</span></button>)}</div>
      <StoryCard key={kind + period} kind={kind} period={period} label={label} />
      {list && list.length > 0 && <Card title={<><Icon name="book" size={16} />Written so far</>}>
        <div className="inline-list">{list.map(s => <a key={s.kind + s.period} className="li" href="#" style={{ color: 'inherit' }} onClick={e => { e.preventDefault(); setSp({ kind: s.kind, period: s.period }); force(x => x + 1); document.querySelector('.content')?.scrollTo({ top: 0 }) }}>
          <span className="badge">{KIND_LABEL[s.kind]}</span><span className="grow"><b>{s.title}</b> <span className="muted small">{s.period}</span></span><span className="muted small">{fmtDateTime(s.created_at, false)}</span>
        </a>)}</div>
      </Card>}
    </>
  )
}
