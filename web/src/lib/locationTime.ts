import type { Visit } from './types'

/** Dates arrive in the source's timezone; never reinterpret them in the browser's timezone. */
export function locationTime(visit: Visit, date: string, isToday = false) {
  const startDate = visit.start_date || date
  const endDate = visit.end_date || date
  const continued = startDate < date
  const crossesDay = startDate !== endDate
  const dayName = (day: string) => {
    if (isToday) {
      const offset = (Date.parse(`${day}T00:00:00Z`) - Date.parse(`${date}T00:00:00Z`)) / 86400000
      if (offset === -1) return 'yesterday'
      if (offset === 1) return 'tomorrow'
    }
    return new Intl.DateTimeFormat('en-GB', {
      day: 'numeric', month: 'short', timeZone: 'UTC',
      ...(day.slice(0, 4) !== date.slice(0, 4) ? { year: 'numeric' } : {}),
    }).format(new Date(`${day}T00:00:00Z`))
  }
  const start = startDate === date ? visit.start : `${dayName(startDate)} at ${visit.start}`
  const end = endDate === date ? visit.end : `${dayName(endDate)} at ${visit.end}`
  const range = [
    continued ? `Continued from ${start}` : '',
    visit.end && (crossesDay || visit.end !== visit.start) ? `until ${end}` : '',
  ].filter(Boolean).join(' · ')
  return { continued, crossesDay, time: continued ? 'Earlier' : visit.start, sortKey: `${startDate}T${visit.start}`, range, start, end }
}

export function timelineOrder(a: { time: string; sortKey?: string; kind: string }, b: { time: string; sortKey?: string; kind: string }, date: string) {
  return (a.sortKey || `${date}T${a.time}`).localeCompare(b.sortKey || `${date}T${b.time}`)
    || Number(b.kind === 'moment') - Number(a.kind === 'moment')
}
