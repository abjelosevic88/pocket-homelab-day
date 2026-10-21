import { Icon } from './icons'

const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']
const WD = ['S', 'M', 'T', 'W', 'T', 'F', 'S']
const iso = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

/** The sidebar's month: days with an entry carry a dot; future days are off. month is "YYYY-MM". */
export function MiniMonth({ month, onMonth, onTitle, selected, today, weekStart, onPick, dots }: {
  month: string; onMonth: (m: string) => void; onTitle?: () => void; selected?: string; today: string; weekStart: number; onPick: (d: string) => void; dots?: Set<string>
}) {
  const [y, m] = month.split('-').map(Number)
  const first = new Date(y, m - 1, 1)
  const lead = (first.getDay() - weekStart + 7) % 7
  const start = new Date(y, m - 1, 1 - lead)
  const days = Array.from({ length: 42 }, (_, i) => new Date(start.getFullYear(), start.getMonth(), start.getDate() + i))
  const shift = (n: number) => { const d = new Date(y, m - 1 + n, 1); onMonth(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`) }
  const rows = days.length / 7
  return (
    <div className="mini">
      <div className="mini-head">
        <button type="button" className="mini-title" onClick={onTitle} title="Open the month">{MONTHS[m - 1]} {y}</button>
        <button type="button" className="btn ghost icon-only xs" aria-label="Previous month" onClick={() => shift(-1)}><Icon name="chevron-left" size={15} /></button>
        <button type="button" className="btn ghost icon-only xs" aria-label="Next month" onClick={() => shift(1)} disabled={month >= today.slice(0, 7)}><Icon name="chevron-right" size={15} /></button>
      </div>
      <div className="mini-grid" role="grid">
        <div className="mini-row" role="row">{Array.from({ length: 7 }, (_, i) => <span key={i} className="mini-wd">{WD[(weekStart + i) % 7]}</span>)}</div>
        {Array.from({ length: rows }, (_, r) => (
          <div className="mini-row" key={r} role="row">
            {days.slice(r * 7, r * 7 + 7).map(d => {
              const k = iso(d)
              const cls = ['mini-day', d.getMonth() !== m - 1 && 'out', k === today && 'today', k === selected && 'sel'].filter(Boolean).join(' ')
              return (
                <button type="button" role="gridcell" key={k} className={cls} onClick={() => onPick(k)} disabled={k > today} aria-label={k} aria-current={k === today ? 'date' : undefined}>
                  <span>{d.getDate()}</span>
                  {dots?.has(k) && <i className="mini-dot" />}
                </button>
              )
            })}
          </div>
        ))}
      </div>
    </div>
  )
}
