# rex-xstream

![rex-xstream](assets/logo.svg)

rex-xstream is an XC IPTV add-on for [REX](https://rex.debridmediamanager.com). It connects one
or more Xtream Codes (XC) playlists and exposes their live channels, movies and series as
libraries in REX. REX continues to serve Plex clients; rex-xstream never stores or copies media
bytes.

This project is not an IPTV player, a replacement for REX, or a replacement for Debrid Media
Manager. It is a source add-on built on REX's public add-on API.

## Features

- Multiple independent XC playlists
- Automatic REX libraries with stable identities:
  - `Live TV`
  - `<Playlist> Movies`
  - `<Playlist> Series`
- Live channels with numbers, categories, logos and EPG
- Movies with provider metadata and optional TMDB ids
- Series with seasons and episodes, including TMDB episode ids
- Encrypted provider credentials at rest
- Background synchronisation with per playlist status
- Incremental probes and batched REX pushes
- Channel logo fallbacks when a provider has no image
- API authentication, rate limiting and SSRF checks
- SQLite persistence with migrations
- Docker image with ffprobe included

## How it works

rex-xstream is a REX add-on, which means three things:

1. It registers a source with REX over `PUT /rex/sources/<name>`.
2. It pushes items over `/rex/feed/<name>/items`.
3. It serves the bytes when REX asks for them.

Every playlist gets a stable directory prefix:

| Directory | REX library | Content |
| --- | --- | --- |
| `live` | Live TV | channels from every enabled playlist |
| `p1-movies` | Playlist 1 Movies | movies from playlist 1 |
| `p1-shows` | Playlist 1 Series | series and episodes from playlist 1 |

Directory names never change when a playlist is renamed. Only the library title changes, so REX
does not create duplicate libraries.

REX requires `media_info` (ffprobe JSON) before an item becomes visible. rex-xstream probes each
stream once, caches the result in SQLite, and pushes the item after the probe. The probe reads a
small part of the file over HTTP; it never downloads the file.

Live channels use `.m3u8` paths. REX follows a redirect to the provider for live streams so HLS
playlists and segments stay at the provider. Movies and episodes are proxied with Range support,
which keeps provider credentials out of client URLs.

## Requirements

- REX nightly 2026.11.02 or newer for Live TV
- REX reachable from the rex-xstream container or host
- Docker with Compose, or Go 1.27 or newer for a manual build
- ffprobe on the manual installation path (the Docker image includes FFmpeg)
- An XC provider that returns standard `player_api.php` responses

## Quick start with Docker

```bash
git clone https://github.com/leleasley/rex-xstream.git
cd rex-xstream
./scripts/create-env.sh
```

Edit `.env` and set at least:

```ini
PUBLIC_URL=http://192.168.1.20:9981
REX_URL=http://192.168.1.20:32400
```

`PUBLIC_URL` is the address REX uses to reach rex-xstream. `REX_URL` is the address rex-xstream
uses to reach REX. Do not use `127.0.0.1` for either value when the services run on different
machines.

Start the service:

```bash
docker compose up -d
docker compose logs -f
```

Open `http://192.168.1.20:9981/api/v1/health`. A healthy response contains `"status":"ok"`.

### Image versions and updates

The Compose file uses `ghcr.io/leleasley/rex-xstream:latest`. The release workflow publishes
`edge` from main, a commit SHA tag, and version tags such as `1.2.3`, `1.2`, and `1`
from a stable `v1.2.3` Git tag. `latest` tracks stable releases only. Set
`REX_XSTREAM_VERSION` in `.env` to pin a version. After a release is published, update with
`docker compose pull && docker compose up -d`. The first release requires a Git tag and GitHub
Actions package permissions; GHCR package visibility may need to be made public in GitHub.

For local development, build with `docker build -t rex-xstream:local .` and set the image
in your own Compose override. The running version is reported by `/api/v1/health`.

`PUBLIC_URL` is the address REX uses to reach this add-on, not the Xtream provider URL.
Enter the provider address and login when creating a playlist below. The connection test checks
credentials; the status endpoint shows whether catalog, EPG, probes and delivery finished.
A missing guide may mean the provider supplies no matching EPG IDs or listings. Check logs
and the playlist status before sharing diagnostic output; never share provider credentials.

## Add a playlist

```bash
curl -X POST http://192.168.1.20:9981/api/v1/playlists \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Provider",
    "base_url": "http://provider.example:8080",
    "username": "account",
    "password": "secret"
  }'
```

The playlist is tested, stored and synced in the background. The three libraries appear in REX
as soon as the first items are probed and pushed.

Check progress:

```bash
curl -H "Authorization: Bearer $API_TOKEN" \
  http://192.168.1.20:9981/api/v1/playlists/1/status
```

## Configuration

All settings are environment variables. See `.env.example`.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_HOST` | `0.0.0.0` | Listen address |
| `APP_PORT` | `9981` | Listen port |
| `PUBLIC_URL` | required | Address REX uses to fetch streams |
| `DATABASE_PATH` | `/data/rex-xstream.db` | SQLite file |
| `API_TOKEN` | required | Bearer token for the admin API |
| `ENCRYPTION_KEY` | required | Base64 of 32 bytes, encrypts credentials |
| `REX_SOURCE_USER` | `rex` | Basic auth user REX sends |
| `REX_SOURCE_PASSWORD` | required with REX | Basic auth password REX sends |
| `REX_TOKEN` | empty | Optional Plex token, enables library renames on playlist rename |

A playlist rename changes the source directory titles immediately. REX does not rename an
existing library from a source registration alone, so set `REX_TOKEN` when the library titles
must change in Plex too. The token is the server token from REX, not a client token.
| `REX_ENABLED` | `true` | Register with REX |
| `REX_URL` | required with REX | REX base URL |
| `REX_SOURCE_NAME` | `rex-xstream` | Source name in REX, keep it stable |
| `REX_PUSH_BATCH` | `100` | Items per feed request |
| `REX_SEND_TMDB_IDS` | `true` | Send TMDB ids so Plex names items. Set `false` to keep overlapping titles in their own libraries |
| `SYNC_ON_START` | `true` | Sync enabled playlists at startup |
| `SYNC_ON_CREATE` | `true` | Sync a playlist after it is added |
| `SYNC_INTERVAL` | `6h` | Full refresh interval |
| `SYNC_CONCURRENCY` | `4` | Parallel series detail requests |
| `PROBE_WORKERS` | `4` | Parallel ffprobe processes |
| `PROBE_TIMEOUT` | `30s` | ffprobe timeout per item |
| `HTTP_TIMEOUT` | `45s` | XC API request timeout |
| `EPG_DAYS` | `7` | Guide window |
| `SYNC_MAX_MOVIES` | `0` | Optional movie limit, 0 means all |
| `SYNC_MAX_SERIES` | `0` | Optional series limit, 0 means all |
| `SYNC_MAX_LIVE` | `0` | Optional channel limit, 0 means all |
| `FFPROBE_PATH` | `ffprobe` | ffprobe binary |
| `XC_ALLOW_PRIVATE_HOSTS` | `false` | Allow providers on private addresses |
| `RATE_LIMIT_PER_MINUTE` | `120` | API requests per IP per minute |
| `CORS_ORIGINS` | empty | Comma separated allowed origins |

`SYNC_MAX_*` limits are useful for a first run against a large provider. Set them to small values,
verify the libraries in REX, then raise or remove them.

## Playlist management

```text
GET    /api/v1/playlists
POST   /api/v1/playlists
GET    /api/v1/playlists/{id}
PATCH  /api/v1/playlists/{id}
DELETE /api/v1/playlists/{id}
POST   /api/v1/playlists/{id}/test
POST   /api/v1/playlists/{id}/sync
GET    /api/v1/playlists/{id}/status
```

Renaming a playlist changes its REX library titles and keeps the directory names. Deleting a
playlist removes its items from REX and removes its libraries from future source registrations.
REX does not delete libraries itself; if an empty library remains in Plex, remove it there.

## Content API

```text
GET /api/v1/playlists/{id}/live/categories
GET /api/v1/playlists/{id}/live/channels?category=&limit=&offset=
GET /api/v1/playlists/{id}/movies/categories
GET /api/v1/playlists/{id}/movies?category=&limit=&offset=
GET /api/v1/playlists/{id}/series/categories
GET /api/v1/playlists/{id}/series?category=&limit=&offset=
GET /api/v1/playlists/{id}/series/{sid}
GET /api/v1/playlists/{id}/series/{sid}/seasons
GET /api/v1/playlists/{id}/series/{sid}/episodes?season=
GET /api/v1/playlists/{id}/epg?channel=<epg id>
GET /api/v1/libraries
GET /api/v1/libraries/{directory}
POST /api/v1/stream/resolve
```

The OpenAPI document is served at `/api/v1/openapi.yaml` and lives in `docs/openapi.yaml`.

## REX compatibility

rex-xstream uses the documented REX add-on API. It does not modify REX and does not rely on
private endpoints.

- Source registration: `PUT /rex/sources/rex-xstream`
- Item pushes: `POST /rex/feed/rex-xstream/items`
- Removals: `DELETE /rex/feed/rex-xstream/items`
- Stream bytes: `<PUBLIC_URL>/<directory>/<id>.<ext>`

Live TV needs REX's `.m3u8` plus `channel` support. Plex Web and Plex for Android are the clients
tested by the REX project for Live TV.

REX requires ffmpeg on its own host to convert live channels. If Live TV does not play, check
ffmpeg on the REX machine first.

## Channel logos and fallbacks

Provider logo URLs are sent as `display.thumb`. They are never downloaded or copied. A channel
without a logo uses `assets/logo-icon.svg` served by rex-xstream, so Plex never shows a blank
tile.

## EPG

The provider XMLTV guide is parsed once per synchronisation and stored by channel EPG id. If a
channel is missing from XMLTV, its provider `get_simple_data_table` response is used as a
fallback. Only channels present in the playlist receive a guide. The guide window is three hours in the
past and `EPG_DAYS` ahead. Programme icons, categories and `xmltv_ns` episode numbers are kept.

## Security

- Credentials are encrypted with AES-256-GCM before they are written.
- API responses never include passwords. Provider stream URLs are stored encrypted.
- Administrative endpoints require `Authorization: Bearer <API_TOKEN>`.
- Stream and fallback logo requests require REX's Basic credentials.
- Provider URLs are validated and private or loopback destinations are rejected unless
  `XC_ALLOW_PRIVATE_HOSTS=true`.
- Stream paths are parsed as numeric ids. Arbitrary URLs and path traversal are rejected.
- API rate limiting is per client IP.
- The container runs as a non-root user.
- Movies and episodes are proxied with Range support. Live HLS is a redirect to the provider,
  so the provider credentials are present in that redirect between rex-xstream and the provider,
  never in the add-on API or a client response.

Never expose the admin API to the internet. Put it behind a VPN or a reverse proxy that blocks
`/api/v1/playlists` from untrusted clients. REX's feed itself is designed for the local network.

## Development

```bash
make test
make vet
make build
```

Run locally:

```bash
export API_TOKEN=local-development-token
export ENCRYPTION_KEY=$(openssl rand -base64 32 | tr -d '\n')
export PUBLIC_URL=http://127.0.0.1:9981
export REX_ENABLED=false
export DATABASE_PATH=/tmp/rex-xstream.db
go run ./cmd/rex-xstream
```

The test suite uses httptest providers and does not need a real IPTV subscription. It covers XC
authentication, provider failures, category and stream parsing, XMLTV parsing, playlist
lifecycle, renaming, multiple playlists, duplicate prevention, encryption, API authentication,
stream proxying and REX item mapping.

## Troubleshooting

**A playlist says error.** Call `POST /api/v1/playlists/{id}/test` and read the response. The
provider may reject the account, the URL, or the request user agent.

**A library is empty.** Check `GET /api/v1/playlists/{id}/status`. Items appear only after they
are probed. If probes fail, `probe_error` is stored per item and the item is held.

**Live TV is missing.** Confirm REX is nightly 2026.11.02 or newer, ffmpeg exists on the REX
host, and at least one channel has a successful HLS probe.

**A channel has no guide.** Its `epg_channel_id` must match a channel id in the provider's
`xmltv.php` document.

**REX does not show a new library.** Check the REX log for `SOURCES rex-xstream registered` and
the source response with `GET /rex/sources/rex-xstream`.

**Probes take a long time.** A large provider has tens of thousands of items. Use `SYNC_MAX_*`
for the first run, or raise `PROBE_WORKERS` carefully. Providers may rate limit aggressive
probing.

## Limitations

- REX needs real `media_info`. A catalogue cannot be shown without probing each item once.
- Large providers have tens of thousands of entries. The first full probe pass takes time.
- Live channels are HLS only. MPEG-TS only channels are skipped.
- Catch-up and archive playback are not supported by REX Live TV.
- Plex does not offer custom badge fields; quality words in provider names are preserved in
  titles, and 4K or HDR indicators come from the real probe.
- REX does not delete libraries. Deleting a playlist removes its items, but an empty Plex
  library may need to be removed in Plex.
- REX keeps one row per external title. If the same movie or show already exists in another
  REX source, setting `REX_SEND_TMDB_IDS=false` keeps the playlist copy in its own library at
  the cost of Plex naming and artwork. With TMDB ids on, REX merges the title and its library
  follows the existing row.
- REX library refresh buttons do not scan add-ons. Use the scheduled sync or the sync API.

## Contributing

Open an issue with the provider response shape, the failing endpoint and a redacted log. Never
post credentials, tokens or full stream URLs. Pull requests should include tests and `make test`
should pass.

## Licence

MIT. See `LICENSE`.
