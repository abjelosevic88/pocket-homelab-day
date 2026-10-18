import React, { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ApiError, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { fmtDate, fmtDateTime } from '../lib/format'
import type { Story } from '../lib/types'
import { Card, useToast } from './ui'

/** True when an OpenAI key is set. */
export function useAI() {
  const { settings } = useApp()
  return !!settings?.ai_configured
}

/** Renders text with [YYYY-MM-DD] (and bare dates) as links to the day; blank lines become paragraphs. */
export function CitedText({ text }: { text: string }) {
  return <>{text.split(/\n{2,}/).map((para, i) => {
    const lines = para.split('\n').map(l => l.trimEnd()).filter(Boolean)
    const isItem = (l: string) => /^\s*([-*•]|\d+[.)])\s+/.test(l)
    if (lines.length > 1 && lines.some(isItem)) {
      const intro = lines.filter(l => !isItem(l))
      const items = lines.filter(isItem)
      return <div key={i}>{intro.length > 0 && <p>{linkDates(intro.join(' '))}</p>}<ul className="cited-list">{items.map((l, j) => <li key={j}>{linkDates(l.replace(/^\s*([-*•]|\d+[.)])\s+/, ''))}</li>)}</ul></div>
    }
    return <p key={i}>{lines.map((l, j) => <span key={j}>{j > 0 && <br />}{linkDates(l)}</span>)}</p>
  })}</>
}
function linkDates(t: string): React.ReactNode[] {
  const out: React.ReactNode[] = []
  const re = /\[?(\d{4}-\d{2}-\d{2})\]?/g
  let last = 0, m: RegExpExecArray | null
  while ((m = re.exec(t))) {
    if (m.index > last) out.push(t.slice(last, m.index))
    out.push(<Link key={m.index} className="date-link" to={`/day/${m[1]}`}>{fmtDate(m[1])}</Link>)
    last = m.index + m[0].length
  }
  if (last < t.length) out.push(t.slice(last))
  return out
}

function pickMime(): { mime: string; ext: string } {
  const opts = [['audio/webm;codecs=opus', 'webm'], ['audio/webm', 'webm'], ['audio/mp4', 'm4a'], ['audio/ogg;codecs=opus', 'ogg']]
  for (const [m, e] of opts) if (typeof MediaRecorder !== 'undefined' && MediaRecorder.isTypeSupported(m)) return { mime: m, ext: e }
  return { mime: '', ext: 'webm' }
}

/** Record from the microphone, send to /ai/transcribe, hand the text back. */
export function DictateButton({ onText, label = 'Dictate', small = false }: { onText: (t: string) => void; label?: string; small?: boolean }) {
  const { toast } = useToast()
  const [state, setState] = useState<'idle' | 'rec' | 'busy'>('idle')
  const [secs, setSecs] = useState(0)
  const rec = useRef<MediaRecorder | null>(null)
  const chunks = useRef<Blob[]>([])
  const timer = useRef<number | null>(null)
  useEffect(() => () => { if (timer.current) clearInterval(timer.current); rec.current?.stream.getTracks().forEach(t => t.stop()) }, [])
  const start = async () => {
    if (!navigator.mediaDevices?.getUserMedia) { toast('Dictation needs HTTPS and a browser with microphone access', 'err'); return }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      const { mime, ext } = pickMime()
      const r = new MediaRecorder(stream, mime ? { mimeType: mime } : undefined)
      chunks.current = []
      r.ondataavailable = e => { if (e.data.size) chunks.current.push(e.data) }
      r.onstop = async () => {
        stream.getTracks().forEach(t => t.stop())
        if (timer.current) clearInterval(timer.current)
        setState('busy')
        try {
          const fd = new FormData()
          fd.append('file', new Blob(chunks.current, { type: r.mimeType || mime }), `dictation.${ext}`)
          const res = await fetch(`${V1}/ai/transcribe`, { method: 'POST', body: fd, credentials: 'same-origin' })
          const j = await res.json().catch(() => ({}))
          if (!res.ok) throw new ApiError(res.status, j.error || res.statusText)
          if (j.text) onText(j.text); else toast('Nothing heard', 'err')
        } catch (e) { toast((e as Error).message, 'err') } finally { setState('idle') }
      }
      rec.current = r
      r.start()
      setSecs(0); setState('rec')
      timer.current = window.setInterval(() => setSecs(s => { if (s >= 600) { r.stop(); return s } return s + 1 }), 1000)
    } catch (e) { toast('Microphone: ' + (e as Error).message, 'err') }
  }
  const stop = () => rec.current?.state === 'recording' && rec.current.stop()
  const cls = `btn ${small ? 'sm' : 'sm'} ${state === 'rec' ? 'rec' : 'ghost'}`
  if (state === 'rec') return <button type="button" className={cls} onClick={stop} title="Stop and transcribe">■ {Math.floor(secs / 60)}:{String(secs % 60).padStart(2, '0')}</button>
  if (state === 'busy') return <button type="button" className={cls} disabled>Transcribing…</button>
  return <button type="button" className={cls} onClick={start} title="Speak instead of typing">🎙{small ? '' : ` ${label}`}</button>
}

/** A stored story for a period, with write / rewrite. */
export function StoryCard({ kind, period, label, className = 'mb' }: { kind: 'week' | 'month' | 'year'; period: string; label: string; className?: string }) {
  const { toast } = useToast()
  const [story, setStory] = useState<Story | null | undefined>(undefined)
  const [busy, setBusy] = useState(false)
  useEffect(() => { setStory(undefined); api.get<{ story: Story | null }>(`${V1}/ai/stories/${kind}/${period}`).then(r => setStory(r.story)).catch(() => setStory(null)) }, [kind, period])
  const write = async () => {
    setBusy(true)
    try { const r = await api.post<{ story: Story }>(`${V1}/ai/stories/${kind}/${period}`); setStory(r.story) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  if (story === undefined) return null
  return (
    <Card className={`${className} story-card`} title={<span>✨ {story?.title || `The story of ${label}`}</span>} actions={<button className="btn ghost sm" onClick={write} disabled={busy}>{busy ? 'Writing…' : story ? '↻ Rewrite' : 'Write it'}</button>}>
      {story ? <><div className="story-text"><CitedText text={story.text} /></div><p className="muted small" style={{ margin: 0 }}>Written {fmtDateTime(story.created_at)} · {story.model}</p></>
        : <p className="muted" style={{ margin: 0 }}>{busy ? 'Reading the days…' : `Let the AI read ${label} — your words and what the apps recorded — and write it up as a story.`}</p>}
    </Card>
  )
}
