export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }
export interface Currency { code: string; name: string; decimals: number }

export interface Source { url: string; key: string; public_url: string; extra: string; enabled: boolean; configured: boolean; needs_key: boolean }
export type SourceName = 'dawarich' | 'immich' | 'maloja' | 'jellystat' | 'firefly' | 'habits' | 'bookshelf' | 'travel' | 'kitchen' | 'garage' | 'weather'
export interface Settings {
  title: string; date_format: string; time_format: string; week_start: 'mon' | 'sun'; home_currency: string; distance_unit: 'km' | 'mi'
  home_lat: number; home_lon: number; home_place: string; prompt: string; backfill_days: number
  notify_url: string; notify_format: string; notify_configured: boolean; notify_evening: boolean; evening_hour: number; notify_morning: boolean; morning_hour: number
  sources: Record<SourceName, Source>; source_names: SourceName[]; setup_complete: boolean; base_url: string; timezone: string
  ai_key: string; ai_model: string; ai_base_url: string; ai_transcribe_model: string; ai_language: string
  ai_question: boolean; ai_weekly_story: boolean; ai_send_text: boolean; ai_configured: boolean; ai_default_model: string; ai_default_transcribe: string
}
export interface Story { user_id: number; kind: 'week' | 'month' | 'year'; period: string; title: string; text: string; model: string; created_at: string }
export interface Ask { id: number; question: string; answer: string; dates: string[]; scope: string; model: string; created_at: string }
export interface Pattern { label: string; with: number; without: number; n_with: number; n_without: number }
export interface UsageRow { feature: string; model: string; calls: number; prompt_tokens: number; completion_tokens: number }

export interface Entry { id: number; user_id: number; date: string; highlight: string; text: string; mood: number; tags: string; pinned: boolean; created_at: string; updated_at: string; moments?: number; words?: number }
export interface Moment { id: number; user_id: number; date: string; time: string; text: string; kind: string; created_at: string }
export interface Pin { id: number; user_id: number; date: string; source: string; ref: string; title: string; subtitle: string; image: string; created_at: string }
export interface Fact<T = unknown> { date: string; source: string; payload: T; headline: number; summary: string; error?: string; fetched_at: string }

export interface DayPage {
  date: string; weekday: number; today: boolean; future: boolean; entry: Entry | null; moments: Moment[]; pins: Pin[]
  facts: Record<string, Fact>; sources: SourceName[]; memories: Entry[]; prev_entry: string; next_entry: string; streak: number; prompt: string; links: Record<string, string>
}
export interface CalendarDay { date: string; entry: boolean; mood: number; highlight: string; pinned: boolean; moments: number; headlines: Record<string, number>; weather?: string }
export interface HomeData { today: string; stats: { total: number; this_month: number; this_year: number; mood_month: number; words: number; first_date: string; pinned: number; moments: number }; streak: number; longest_streak: number; recent: Entry[]; memories: Entry[] }
export interface BackfillStatus { running: boolean; from: string; to: string; done: number; total: number; current: string; last_run: string; error?: string }

// ---- fact payloads (mirror internal/sources) ----
export interface Visit { kind: 'visit' | 'journey'; name?: string; start: string; end: string; minutes: number; lat?: number; lng?: number; mode?: string; distance_km?: number; status?: string }
export interface Where { distance_km: number; moving_minutes: number; still_minutes: number; modes: Record<string, number>; places: number; journeys: number; entries: Visit[]; bounds?: number[]; first_move: string; last_move: string }
export interface Photo { id: string; type: 'IMAGE' | 'VIDEO'; taken_at: string; time: string; lat?: number; lng?: number; city?: string; country?: string; favorite: boolean; duration?: string; file_name?: string }
export interface Photos { count: number; videos: number; favorites: number; places: string[]; first: string; last: string; items: Photo[] }
export interface Count { name: string; count: number }
export interface Listen { time: string; artist: string; title: string; album?: string }
export interface Music { listens: number; minutes: number; artists: Count[]; tracks: Count[]; albums: Count[]; first: string; last: string; items: Listen[] }
export interface Play { title: string; series?: string; kind: 'episode' | 'movie'; minutes: number; time: string; client?: string; user?: string; item_id: string; plays: number }
export interface Watched { count: number; episodes: number; movies: number; minutes: number; items: Play[] }
export interface Spend { id: string; time: string; description: string; amount: number; currency: string; category?: string; budget?: string; destination?: string; source?: string; tags?: string[] }
export interface Money { count: number; totals: Record<string, number>; categories: Count[]; biggest?: Spend; items: Spend[]; main_currency: string }
export interface HabitRow { id: number; name: string; color: string; status: number; scheduled: boolean; streak: number }
export interface TrackerRow { id: number; name: string; unit: string; kind: string; value: number | null; min?: number; max?: number; higher_is_better?: boolean; previous?: number; defaulted?: boolean }
export interface Checkin { done: number; total: number; habits: HabitRow[]; trackers: TrackerRow[]; journal: string; journal_bad?: string; important?: string }
export interface Book { id: number; title: string; authors: string; pages?: number; page?: number; percent?: number; rating?: number; status: string; cover?: string; finished_on?: string; started_on?: string }
export interface Reading { reading: Book[]; started: Book[]; finished: Book[] }
export interface Trip { id: number; name: string; destination: string; start_date: string; end_date: string; day_no: number; days: number; color: string; status: string }
export interface Trips { trips: Trip[] }
export interface Cook { id: number; recipe_id: number | null; title: string; servings: number; rating: number; notes: string; by?: string }
export interface Cooked { cooks: Cook[] }
export interface Fill { vehicle: string; kind: string; title: string; volume?: number; cost: number; currency: string; odometer?: number; station?: string }
export interface Car { items: Fill[] }
export interface WeatherDay { code: number; condition: string; t_max: number | null; t_min: number | null; precip_mm: number | null; sunshine_h: number | null; daylight_h: number | null; wind_max: number | null; place: string }

export interface MonthRecap { month: string; entries: number; moments: number; mood: number; photos: number; km: number; listens: number; watch_min: number; spend: Record<string, number>; books: number; cooked: number; habits_pct: number; days: number; sunny_pct: number }
export interface YearRecap {
  year: number; months: MonthRecap[]; entries: number; moments: number; words: number; mood: number; moods: number[]; photos: number; videos: number; km: number; modes: Record<string, number>
  listens: number; artists: Count[]; tracks: Count[]; watch_min: number; shows: Count[]; movies: number; spend: Record<string, number>; categories: Count[]; books: Book[]; cooked: number; dishes: Count[]
  places: Count[]; visits: Count[]; trips: Trip[]; conditions: Count[]; tags: { tag: string; count: number }[]; pinned: Entry[]; best: Entry[]; longest_streak: number; days_with_facts: number; busiest_day: string; farthest_day: string; weekday_mood: number[]
}

export const MOODS = ['', 'rough', 'meh', 'okay', 'good', 'great']
export const MOOD_ICON = ['○', '😖', '😕', '😐', '🙂', '😄']
export const MOOD_COLOR = ['var(--border)', '#ef4444', '#f59e0b', '#a3a3a3', '#22c55e', '#4f46e5']
export const KIND_ICON: Record<string, string> = { note: '•', win: '🏆', thanks: '🙏', idea: '💡', event: '📌' }
export const SOURCE_LABEL: Record<string, string> = { dawarich: 'Where', immich: 'Photos', maloja: 'Music', jellystat: 'Watched', firefly: 'Money', habits: 'Habits', bookshelf: 'Reading', travel: 'Trip', kitchen: 'Cooked', garage: 'Garage', weather: 'Weather' }
export const SOURCE_APP: Record<string, string> = { dawarich: 'Dawarich', immich: 'Immich', maloja: 'Maloja', jellystat: 'Jellystat', firefly: 'Firefly III', habits: 'Pocket Habits', bookshelf: 'Pocket Bookshelf', travel: 'Pocket Travel', kitchen: 'Pocket Kitchen', garage: 'Pocket Garage', weather: 'Open-Meteo' }
export const SOURCE_ICON: Record<string, string> = { dawarich: '🗺', immich: '📷', maloja: '🎧', jellystat: '📺', firefly: '💸', habits: '✅', bookshelf: '📚', travel: '✈️', kitchen: '🍳', garage: '🚗', weather: '⛅' }
export const WEATHER_ICON: Record<string, string> = { clear: '☀️', partly: '⛅', cloudy: '☁️', fog: '🌫', rain: '🌧', snow: '❄️', storm: '⛈' }
export const cap = (s: string) => s ? s[0].toUpperCase() + s.slice(1).replace(/_/g, ' ') : ''
export function fmtMinutes(m: number): string {
  if (!m) return ''
  const h = Math.floor(m / 60), mm = m % 60
  return h ? (mm ? `${h} h ${mm} min` : `${h} h`) : `${mm} min`
}
export function fmtKm(km: number, unit: 'km' | 'mi' = 'km'): string {
  if (unit === 'mi') return `${(km / 1.609344).toFixed(km >= 100 ? 0 : 1)} mi`
  return `${km.toFixed(km >= 100 ? 0 : 1)} km`
}
export function fmtAmount(v: number, cur: string): string {
  try { return new Intl.NumberFormat(undefined, { style: 'currency', currency: cur, maximumFractionDigits: 2 }).format(v) } catch { return `${v.toFixed(2)} ${cur}` }
}
export const tagList = (t: string) => t ? t.split(',').filter(Boolean) : []
