# Configuration

Everything is an environment variable with a sensible default. Sources, the AI key, the home location and notifications are seeded from the environment into the **owner's** settings (the first admin) and then edited under **Settings**; the saved values win. Those tokens are the owner's own accounts, so every other account starts with no sources and connects their own under Settings → Sources: settings, facts and the journal are all per person.

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
| `CORE_URL` / `CORE_REGISTRATION_KEY` | – | [Pocket Core](https://git.bjelke.org/abjelosevic88/pocket-homelab-core): register on start (the key is needed once; the minted app token lives in `$DATA_DIR/core.token`), publish `day.entry_written` / `day.moment_created` / `day.story_created`, accept the tokens Core hands to paired phones, and report everybody's connection modes (core / direct / off) for Core's Links page |
| `CORE_APP_URL` | `BASE_URL` | How Core reaches this app for health checks (e.g. `http://host.docker.internal:8177`) |
| `DEMO_DATA` | `false` | six weeks of journal and synthetic facts |

## Auth
| Variable | Default | |
|---|---|---|
| `AUTH_MODE` | `local` | `local` (built-in login, plus “Sign in with …” when an OpenID Connect client is configured or issued by Pocket Core), `proxy` (trust a header from an SSO proxy), `none` (single user, trusted LAN) |
| `AUTH_PROXY_HEADER` / `AUTH_PROXY_EMAIL_HEADER` | `Remote-User` / `Remote-Email` | |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_NAME` | | create the first admin on an empty database |
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | | sign in with an OpenID Connect provider (Pocket Core, PocketID, Authelia, Authentik, Keycloak…). Needs `BASE_URL`; the callback is `BASE_URL/api/v1/auth/oidc/callback`. On a box with Pocket Core none of these are needed — the app receives its client when it registers. |
| `OIDC_NAME` | `single sign-on` | button label: “Sign in with …” (“Pocket Core” for the client Core issued) |
| `OIDC_AUTO_CREATE` | `true` | give an unknown (verified) email a member account instead of refusing it; always on for the client Core issued, since Core already vouches for the person |
| `SESSION_TTL` | `720h` | |
| `SECURE_COOKIES` | `false` | `true` behind HTTPS |

## The day
| Variable | Default | |
|---|---|---|
| `HOME_LAT` / `HOME_LON` / `HOME_PLACE` | | weather location for days spent within ~40 km of home |
| `BACKFILL_DAYS` | `90` | how far back facts are pulled on first start and by “Backfill missing days” |
| `DEFAULT_CURRENCY` | `EUR` | |

## Sources

Each source takes `<NAME>_URL`, a secret as `<NAME>_TOKEN` / `<NAME>_API_KEY` / `<NAME>_KEY`, an optional `<NAME>_PUBLIC_URL` (what the browser opens for ↗ links) and `<NAME>_EXTRA`. Names: `DAWARICH`, `IMMICH`, `MALOJA`, `JELLYSTAT`, `FIREFLY`, `HABITS`, `BOOKSHELF`, `TRAVEL`, `KITCHEN`, `GARAGE`, `PINEPODS`, `AUDIOBOOKSHELF`, `JELLYSEERR`, `FORGEJO`, `NEXTCLOUD`, `DOCUMENTS`, `INVOICING`, `SPLIT`, `INVENTORY`. `_EXTRA` is: the Jellyfin user for Jellystat, the users to keep for Audiobookshelf (comma separated), the user name for Forgejo and Nextcloud. `JELLYSTAT_USER` (= `JELLYSTAT_EXTRA`) keeps only one Jellyfin user's plays.

`DOCUMENTS_*` is **Pocket Documents** (a URL like `http://…:8183` and a `pd_…` API token from Pocket Documents → Settings → API tokens; on a box with Pocket Core neither is needed). It replaced Paperless-ngx: `PAPERLESS_*` is ignored (a warning is logged at start), and migration 0005 renamed the facts and pins frozen while Day read Paperless to the `documents` source, so those days show as before and are not pulled again. Documents that came over from Paperless or from an export (`source` `paperless` / `import`) are not counted as added on the day of the import. The day's documents link to `<Pocket Documents public URL>/documents/{id}`.

`FIREFLY_*` is the money source, shown as **Pocket Money**: any Firefly III-compatible API — Pocket Money's `http://…:8178/firefly` with a Money API token, or a real Firefly III with a personal access token. The internal name stays `firefly`, so days already frozen from Firefly III keep their facts and are not pulled again.

### Pocket apps: Pocket Core, Direct or Off

The Pocket app sources (`HABITS`, `BOOKSHELF`, `TRAVEL`, `KITCHEN`, `GARAGE`, `INVOICING`, `SPLIT`, `INVENTORY`, `DOCUMENTS` = Pocket Documents, `FIREFLY` = Pocket Money) are reached per person in one of three ways, chosen in Settings → Sources:

| Mode | How |
|---|---|
| **Pocket Core** | Day acts for the person through a Pocket Core app link: every fetch asks Core for a fresh five-minute `pcx_` token for that person and that app, so a link switched off in Core stops the next pull. Nothing to paste; every person on the box gets their own data. |
| **Direct** | the URL and API token in Settings (or from the environment, for the owner). Without a URL the app's address comes from Core. |
| **Off** | not pulled. |

Not chosen yet: Direct when a token is saved (so pasted tokens keep working until the person switches), Pocket Core on a box with Core, Direct otherwise. A source that was switched off before modes existed counts as Off.

Day declares its uses in the manifest, one read-only scope per app (the targets only open these routes to it):

| App | Scope | Day reads |
|---|---|---|
| habits | `habits:read` | `GET /api/v1/days/{date}` |
| bookshelf | `books:read` | `GET /api/v1/books` |
| travel | `trips:read` | `GET /api/v1/trips` |
| kitchen | `cooklog:read` | `GET /api/v1/cooklog` |
| garage | `vehicles:read` | `GET /api/v1/vehicles`, `/vehicles/{id}/fuel`, `/vehicles/{id}/records` |
| invoicing | `invoices:read` | `GET /api/v1/invoices`, `/payments` |
| split | `groups:read` | `GET /api/v1/groups`, `/groups/{id}`, `/groups/{id}/expenses` |
| inventory | `activity:read` | `GET /api/v1/activity` |
| documents | `documents:read` | `GET /api/v1/documents?added_from=&added_to=&sort=added&limit=500&no_facets=1` |
| money | `transactions:read` | `GET /firefly/api/v1/about`, `/firefly/api/v1/transactions` |

plus `GET /api/v1/auth/me` (and the open `/healthz`) for “Test connection”. A link that is off — or a person Core does not let use the app — means the source is skipped for the day, not recorded as failed; Settings → Sources says why (“you switched the Day → Pocket Garage link off …”). If Core does not answer, sources in Pocket Core mode are skipped for that pull. **Disconnect** (Direct) clears the URL and token; values from the environment are copied into the owner's settings once, so a disconnected token stays gone after a restart.

### Third-party services: Pocket Core, Direct or Off

The other sources (`IMMICH`, `MALOJA`, `DAWARICH`, `JELLYSTAT`, `PINEPODS`, `AUDIOBOOKSHELF`, `JELLYSEERR`, `FORGEJO`, `NEXTCLOUD`) have the same three modes, through Pocket Core **service connections** (Core v0.15+). Day lists them in its manifest under `services`, so Core shows a “Use your Immich” link per service with the admin's switch and each person's:

| Mode | How |
|---|---|
| **Pocket Core** | Day asks Core (`POST /api/v1/services/credential`) for the service's address and the person's credential there; Day itself keeps neither, the Core SDK reuses the answer for a minute (a pull or a page of Immich thumbnails asks once; a switch-off in Core applies within that minute) and, while Core cannot be reached, the last answer for up to 15 minutes, so a Core restart does not stop the pulls. **Test connection** always asks Core afresh. The person's own key (set in Core by hand or by a connector) is used when there is one; the box's key only when the person is an admin. A member with no account of their own gets `no_account`: “not connected”, the source is skipped, nothing fails. |
| **Direct** | the URL and key in Settings (or from the environment, for the owner), as before. |
| **Off** | not pulled. |

Not chosen yet: Direct when a key is saved (Maloja, which needs no key: when a URL is saved), Pocket Core on a box with Core, Direct otherwise; switched off before modes existed counts as Off. Settings → Sources says “Connected through Pocket Core (your own key)” or “(the box's key)”, or why not (“you have no account of your own in Immich yet; ask the admin or let a connector create one”). Whether a service is on comes from `GET /api/v1/links/status` (`service:<type>`, scope `account`); whose key it is, from the last credential Day was handed (the Settings page asks once when it does not know yet).

### Direct connections on Core's map

Core cannot see a URL and key pasted into Day, so Day reports every person's mode in effect per source (what Settings → Sources shows, defaults included) with `PUT /api/v1/links/modes`: targets `habits`, `bookshelf`, `travel`, `kitchen`, `garage`, `invoicing`, `split`, `inventory`, `documents`, `money` (the money source) and `service:<type>` for the services, mode `core` / `direct` / `off`. One call for everybody after each registration (on start and every 30 minutes) and right after a person's modes change (a mode picked, Disconnect, a key pasted or cleared). Report only; failures are logged.

In Settings → Sources on a box with Core the switch shows **Pocket Core | Off**; Direct is behind “Advanced: connect directly with a URL and key” (shown at once to anyone already on Direct). Without Core, Direct is shown as before.

| Source | Core type | Credential from Core | How Day sends it |
|---|---|---|---|
| Immich | `immich` | `API_KEY` | `x-api-key` header (also for thumbnails) |
| Maloja | `maloja` | — (address only) | none, the read API is open |
| Dawarich | `dawarich` | `API_KEY` | `api_key` query parameter |
| Jellystat | `jellystat` | `API_KEY` | `x-api-token` header |
| PinePods | `pinepods` | `API_KEY` | `Api-Key` header |
| Audiobookshelf | `audiobookshelf` | `TOKEN` | `Authorization: Bearer`; with the person's own key Day reads `/api/me/listening-sessions` (their sessions), with the box's admin key `/api/sessions` and the users filter |
| Jellyseerr | `jellyseerr` | `API_KEY` | `X-Api-Key` header |
| Forgejo | `forgejo` | `TOKEN` | `Authorization: token …` |
| Nextcloud | `nextcloud` | `USERNAME` + `APP_PASSWORD` | basic auth (CalDAV); the user name typed in Settings is the fallback |

`_EXTRA` values that are not credentials (the Jellyfin user for Jellystat, the Audiobookshelf users, the Forgejo user name) and `_PUBLIC_URL` apply in every mode. **Disconnect** clears the URL and key (and the Nextcloud user name); environment values are copied once, like the Pocket apps'.

Inside Docker, reach sibling containers by service name or `http://host.docker.internal:PORT` with `extra_hosts: ["host.docker.internal:host-gateway"]`.

## How facts are pulled

- Every 30 minutes the server pulls **today and yesterday** from every connected source (for a source in Pocket Core mode: while its link is on, and for a service while the person has an account there), in parallel, with a 90-second budget per source.
- Days before yesterday are **frozen**: once a source has answered for a day it is not asked again. “↻ Pull again” on a day, or a forced backfill, re-fetches.
- A failed fetch keeps the earlier payload and marks the card *stale*.
- The first start (and each restart) runs a background **backfill** of `BACKFILL_DAYS` for every account with their own sources, one day every 1.5 s, skipping days that already have every source.
- **Habits journal as the entry:** on a day with no words written in Pocket Day (a mood or tags alone do not count), the person's own Pocket Habits journal (from their own Habits source) becomes their entry — the “good thing” line as the highlight, the important note and the “bad thing” line as the text. It follows Habits until you edit it here (then it is yours) and stays gone if you remove it. It counts as written: streaks, the calendar, memories, the AI and the evening push see it.
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

In Pocket Money → Settings → Webhooks, add a webhook to `https://<day>/api/v1/hooks/money` for the three transaction events (either format) and put its signing secret here. The request is checked against `X-Pocket-Signature`. A transaction added, changed or deleted then re-pulls the money facts for its day within a few seconds; a burst of changes, like a bulk edit, becomes one pull per day. Without the secret the endpoint answers 404, and the 30-minute refresh carries on as before. The refresh is one person's: the account named by `user_email` (or `data.user.email`) in the signed body or `?user=<email>` on the webhook URL; a webhook that names nobody (today's Pocket Money and Firefly payloads) counts as the owner's, since it was set up with the owner's Pocket Money account.

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
