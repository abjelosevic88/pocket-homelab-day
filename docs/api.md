# API

Base path `/api/v1`. JSON in and out. Authenticate with the session cookie (browser), `Authorization: Bearer <token>` or `X-API-Key: <token>` (tokens under Settings → API & apps). Dates are `YYYY-MM-DD` in the server's `TZ`; `today` and `yesterday` work wherever a `{date}` is expected.

Everything belongs to the caller and nobody else sees it, admins included: entries, moments, pins, stories, questions, AI usage, the facts (pulled with the caller's own sources) and the settings. Another account's record answers 404. The only box-wide setting is the app title (admin-only).

## Auth & account
| Method | Path | Notes |
|---|---|---|
| GET | `/auth/status` | `needs_setup`, `auth_mode`, `authenticated`, `user`, `version` |
| POST | `/auth/setup` · `/auth/login` · `/auth/logout` | `{email, password, name}` |
| GET/PUT | `/auth/me` | profile, password change |
| GET/POST/DELETE | `/tokens`, `/tokens/{id}` | personal access tokens |
| GET/POST/PUT/DELETE | `/users`, `/users/{id}` | admin |
| GET/PUT | `/settings` · GET `/settings/system` | the caller's own settings plus the box-wide `title` (changing it is admin-only, 403 otherwise); `sources.{name}.{url,key,public_url,extra,enabled}`, plus for the Pocket app sources `mode` (`core` \| `direct` \| `off`; send it to choose) and, read-only, `pocket`, `app`, `via` (`core` \| `direct` \| `""` = not connected), `detail` (why not); `core_configured`. Secrets come back masked; a masked value means "keep" |

## Days
| Method | Path | Notes |
|---|---|---|
| GET | `/today` · `/days/{date}` | `{entry, moments, pins, facts: {source: {payload, headline, summary, error}}, sources, memories, prev_entry, next_entry, streak, prompt, links}` — pulls the day first if nothing is cached yet |
| PUT | `/days/{date}` | `{highlight?, text?, mood? (0–5), tags?, pinned?}` — only sent fields change; an entry with nothing left is deleted |
| DELETE | `/days/{date}` | remove the entry (moments and pins stay) |
| POST | `/days/{date}/refresh` | pull every source again for that day → `{errors, facts}` |
| GET | `/days/{date}/facts/{source}` | one source's fact |
| POST | `/days/{date}/pins` · DELETE `/pins/{id}` | `{source, ref, title, subtitle?, image?}` — idempotent per source+ref |
| GET | `/photos/{assetId}/thumb?size=thumbnail\|preview` | Immich thumbnail proxy |

## Journal
| Method | Path | Notes |
|---|---|---|
| GET | `/entries?q=&tag=&mood=&pinned=1&from=&to=&limit=&offset=&order=asc` | `{entries, total, summaries: {date: {source: line}}}` |
| POST | `/entries` | shortcut form: `{date?, highlight?, text?, mood?, tags?, pinned?}` — **appends** `text` to the day instead of replacing it, merges tags |
| GET | `/tags` | `[{tag, count}]` |
| GET | `/calendar?month=YYYY-MM` or `?from=&to=` | per day: `entry, mood, highlight, pinned, moments, headlines: {source: number}, weather` |
| GET | `/memories?date=` | entries on the same month-day in other years |
| GET | `/years` · `/years/{year}` | year list; the recap (months, moods, top artists/shows/places, books, trips, kept days…) |
| GET | `/search?q=` | entries + moments |

## Moments
| Method | Path | Notes |
|---|---|---|
| GET | `/moments?from=&to=` | default the last 30 days |
| POST | `/moments` | `{text, kind? (note\|win\|thanks\|idea\|event), date?, time? (HH:MM)}` — no date = now |
| PUT/DELETE | `/moments/{id}` | |

## Facts & sources
| Method | Path | Notes |
|---|---|---|
| POST | `/facts/backfill` | the caller's own facts from their own sources, `{from?, to?, force?, sources?}` — runs in the background; `sources: ["habits"]` re-pulls only those |
| GET | `/facts/status` | backfill progress, configured sources, days with facts |
| POST | `/integrations/test` | `{which, url?, key?, extra?}` (empty values fall back to the caller's own saved ones); `which: notify` sends a test push. A Pocket app source is tested the way it is pulled: a typed token directly, otherwise its saved mode (Pocket Core: a fresh link token) |
| GET | `/geo/search?q=` | Open-Meteo geocoding for the home location |
| POST | `/notify/test` · `/notify/run` `{which: evening\|morning}` | |

## AI
Needs an OpenAI key (Settings → AI); otherwise these return 400.

| Method | Path | Notes |
|---|---|---|
| POST | `/ai/days/{date}/draft` | `{highlight, text}` — first-person draft; when the day already has text, only what it is missing |
| GET | `/ai/days/{date}/question?refresh=1` | `{question}` — cached 3 h for today, for good on older days |
| POST | `/ai/days/{date}/suggest` | `{highlight, text}` in → `{highlight, tags, mood, why}` |
| POST | `/ai/transcribe` | multipart `file` (webm, m4a, mp3, wav; ≤ 25 MB), optional `language` → `{text}` |
| GET/POST/DELETE | `/ai/stories/{week\|month\|year}/{period}` | period `2026-W42`, `2026-10`, `2026`; GET returns the cached story or `null`, POST (re)writes it |
| GET | `/ai/stories` | every story written, newest first |
| POST | `/ai/asks` | `{question}` → `{answer, dates, scope}`; GET `/ai/asks` history, DELETE `/ai/asks/{id}` (`0` = all) |
| GET | `/ai/usage` | tokens per feature, last 30 days and all time |
| GET | `/patterns?from=&to=` | mood with/without walks, sun, cooking… (no AI needed) |

### iOS Shortcut: ask your journal

*Dictate Text* → *Get Contents of URL* `POST /api/v1/ai/asks` with `{"question": [Dictated Text]}` → *Get Dictionary Value* `answer` → *Speak Text*.

## Data & health
`GET /backup` (SQLite copy of the whole box, admin only) · `GET /export.json` (`?nofacts=1`) · `GET /export.md` · `POST /import` (merge) · `GET /dashboard` · `GET /home` · `GET /meta` · `/healthz` · `/readyz` · `/metrics` (outside `/api/v1`).

### Homepage widget

```yaml
- Pocket Day:
    href: https://day.example.com/
    icon: mdi-calendar-heart
    widget:
      type: customapi
      url: http://pocket-day:8080/api/v1/dashboard
      refreshInterval: 300000
      headers:
        Authorization: Bearer {{HOMEPAGE_VAR_POCKET_DAY_TOKEN}}
      mappings:
        - field: today_text
          label: Today
        - field: streak
          label: Streak
          suffix: " d"
        - field: photos_today
          label: Photos
        - field: entries_month
          label: Month
```

`/dashboard` also has `longest_streak`, `entries_year`, `entries_total`, `mood_month`, `memories`, `km_today`, `listens_today`, `watch_min_today`, `spend_today`, `habits_today_pct`, `today_summary`.

### iOS Shortcut: add a moment

*Ask for Input* (text) → *Get Contents of URL* → `POST https://day.example.com/api/v1/moments`, header `Authorization: Bearer …`, JSON body `{"text": [Provided Input]}`. Add `"kind": "win"` for a separate “Win” shortcut.

### iOS Shortcut: end the day

*Choose from Menu* 😖…😄 → set a number → *Ask for Input* (the one line) → `POST /api/v1/entries` with `{"mood": N, "highlight": "…"}`.
