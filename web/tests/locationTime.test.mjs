import { test } from 'node:test'
import assert from 'node:assert/strict'
import { locationTime, timelineOrder } from '../src/lib/locationTime.ts'

const visit = { kind: 'visit', start: '23:20', end: '17:21', start_date: '2026-10-23', end_date: '2026-10-24', minutes: 1081 }

test('overnight visit explains yesterday and sorts before today’s midnight and evening entries', () => {
  const timing = locationTime(visit, '2026-10-24', true)
  assert.equal(timing.time, 'Earlier')
  assert.equal(timing.range, 'Continued from yesterday at 23:20 · until 17:21')
  const items = [{ time: '19:17', kind: 'act' }, { ...timing, kind: 'place' }, { time: '00:00', kind: 'moment' }]
  items.sort((a, b) => timelineOrder(a, b, '2026-10-24'))
  assert.deepEqual(items.map(x => x.time), ['Earlier', '00:00', '19:17'])
})

test('historical dates and multi-day stays use explicit dates, including the year at New Year', () => {
  assert.equal(locationTime(visit, '2026-10-24').range, 'Continued from 23 Oct at 23:20 · until 17:21')
  assert.equal(locationTime({ ...visit, start_date: '2026-10-21' }, '2026-10-24', true).range, 'Continued from 21 Oct at 23:20 · until 17:21')
  assert.equal(locationTime({ ...visit, start_date: '2025-12-30', end_date: '2026-01-01' }, '2026-01-01', true).range, 'Continued from 30 Dec 2025 at 23:20 · until 17:21')
})

test('a visit extending past midnight labels its end even when both clocks match', () => {
  const timing = locationTime({ ...visit, end: '23:20' }, '2026-10-23', true)
  assert.equal(timing.time, '23:20')
  assert.equal(timing.range, 'until tomorrow at 23:20')
})

test('same-day and legacy records preserve their clocks without inventing a date', () => {
  for (const v of [{ ...visit, start: '09:00', start_date: '2026-10-24' }, { ...visit, start_date: undefined, end_date: undefined }]) {
    const timing = locationTime(v, '2026-10-24', true)
    assert.equal(timing.time, v.start)
    assert.equal(timing.continued, false)
    assert.equal(timing.range, 'until 17:21')
  }
})

test('calendar-relative labels work across daylight saving boundaries', () => {
  assert.match(locationTime({ ...visit, start_date: '2026-10-25', end_date: '2026-10-26' }, '2026-10-26', true).range, /^Continued from yesterday/)
})
