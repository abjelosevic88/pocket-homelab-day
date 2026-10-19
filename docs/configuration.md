# Configuration

Everything is an environment variable with a sensible default. Sources, the home location, the prompt and notifications are seeded from the environment on first start and then edited under **Settings**; the saved values win.

## Server
| Variable | Default | |
|---|---|---|
| `TZ` | container default (UTC) | **Set it.** A day is midnight to midnight in this zone. On Alpine/distroless also bind-mount `/usr/share/zoneinfo`. |
| `PORT` / `HOST` | `8080` / `0.0.0.0` | |
| `BASE_URL` | | public URL, used for links in pushes |
| `DATA_DIR` | `./data` (`/data` in the image) | database and backups |
| `DB_PATH` | `$DATA_DIR/pocket-day.db` | |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `text` | `debug` logs every failed source fetch |
| `TRUST_PROXY` | `true` | honour `X-Forwarded-For` |
| `METRICS_ENABLED` | `true` | Prometheus `/metrics` |
| `CORE_URL` / `CORE_REGISTRATION_KEY` | – | [Pocket Core](https://git.bjelke.org/abjelosevic88/pocket-homelab-core): register on start (the key is needed once; the minted app token lives in `$DATA_DIR/core.token`), publish `day.entry_written` / `day.moment_created` / `day.story_created`, and accept the tokens Core hands to paired phones |
| `CORE_APP_URL` | `BASE_URL` | How Core reaches this app for health checks (e.g. `http://host.docker.internal:8177`) |
| `DEMO_DATA` | `false` | six weeks of journal and synthetic facts |

## Auth
| Variable | Default | |
|---|---|---|
| `AUTH_MODE` | `local` | `local` (built-in login), `proxy` (trust a header from an SSO proxy), `none` (single user, trusted LAN) |
| `AUTH_PROXY_HEADER` / `AUTH_PROXY_EMAIL_HEADER` | `Remote-User` / `Remote-Email` | |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_NAME` | | create the first admin on an empty database |
| `SESSION_TTL` | `720h` | |
| `SECURE_COOKIES` | `false` | `true` behind HTTPS |

## The day
| Variable | Default | |
|---|---|---|
| `HOME_LAT` / `HOME_LON` / `HOME_PLACE` | | weather location for days spent within ~40 km of home |
| `BACKFILL_DAYS` | `90` | how far back facts are pulled on first start and by “Backfill missing days” |
| `DEFAULT_CURRENCY` | `EUR` | |

## Sources

Each source takes `<NAME>_URL`, a secret as `<NAME>_TOKEN` / `<NAME>_API_KEY` / `<NAME>_KEY`, an optional `<NAME>_PUBLIC_URL` (what the browser opens for ↗ links) and `<NAME>_EXTRA`. Names: `DAWARICH`, `IMMICH`, `MALOJA`, `JELLYSTAT`, `FIREFLY`, `HABITS`, `BOOKSHELF`, `TRAVEL`, `KITCHEN`, `GARAGE`, `PINEPODS`, `AUDIOBOOKSHELF`, `JELLYSEERR`, `FORGEJO`, `NEXTCLOUD`, `PAPERLESS`, `INVOICING`, `SPLIT`, `INVENTORY`. `_EXTRA` is: the Jellyfin user for Jellystat, the users to keep for Audiobookshelf (comma separated), the user name for Forgejo and Nextcloud. `JELLYSTAT_USER` (= `JELLYSTAT_EXTRA`) keeps only one Jellyfin user's plays.

Inside Docker, reach sibling containers by service name or `http://host.docker.internal:PORT` with `extra_hosts: ["host.docker.internal:host-gateway"]`.

## How facts are pulled

- Every 30 minutes the server pulls **today and yesterday** from every enabled, configured source, in parallel, with a 90-second budget per source.
- Days before yesterday are **frozen**: once a source has answered for a day it is not asked again. “↻ Pull again” on a day, or a forced backfill, re-fetches.
- A failed fetch keeps the earlier payload and marks the card *stale*.
- The first start (and each restart) runs a background **backfill** of `BACKFILL_DAYS`, one day every 1.5 s, skipping days that already have every source.
- **Habits journal as the entry:** on a day with no words written in Pocket Day (a mood or tags alone do not count), the Pocket Habits journal becomes the entry of the first admin — the “good thing” line as the highlight, the important note and the “bad thing” line as the text. It follows Habits until you edit it here (then it is yours) and stays gone if you remove it. It counts as written: streaks, the calendar, memories, the AI and the evening push see it.
- Weather: the home location, unless the day's Dawarich bounds are more than ~40 km away — then the centre of that day's movement.

## AI
| Variable | Default | |
|---|---|---|
| `OPENAI_API_KEY` | | seeds Settings → AI; empty = AI off |
| `OPENAI_MODEL` | `gpt-5.4-mini` | any chat-completions model; reasoning models get `reasoning_effort: low` |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | any OpenAI-compatible endpoint |

Dictation model, the writing language, tonight's question, the Sunday week story and whether your own words are sent are set in the UI.

## Instant money updates

| Variable | Default | Notes |
|---|---|---|
| `MONEY_WEBHOOK_SECRET` | | enables `POST /api/v1/hooks/money` |

In Pocket Money → Settings → Webhooks, add a webhook to `https://<day>/api/v1/hooks/money` for the three transaction events (either format) and put its signing secret here. The request is checked against `X-Pocket-Signature`. A transaction added, changed or deleted then re-pulls the money facts for its day within a few seconds; a burst of changes, like a bulk edit, becomes one pull per day. Without the secret the endpoint answers 404, and the 30-minute refresh carries on as before.

## Notifications

`NOTIFY_URL` + `NOTIFY_FORMAT` (`apprise` | `ntfy` | `json`). Evening push at the evening hour (default 21:00) unless every user already wrote — with tonight's AI question when AI is on; on Sundays with AI on, the week's story at the same hour; morning push (default 09:00) with yesterday's line and “on this day”.

## CLI

```
pocket-day                          serve
pocket-day backup [-o FILE]         consistent SQLite copy
pocket-day backfill [-from D] [-to D] [-force]
pocket-day reset-password [-email U] [-password P]
pocket-day version
```
