# ekkoPlayer

A production-oriented Raspberry Pi 4 music appliance built around a Go control plane, SQLite, mpv/ALSA audio playback, nginx, a phone-first React/Vite PWA, and a separate administration application.

## Core rule

The Raspberry Pi owns playback. The phone is only a remote control. Music continues if every browser closes, the phone sleeps, Wi-Fi drops, or a WebSocket reconnects.

## Repository

- `backend/` — Go API, SQLite schema, playback controller, mpv IPC, WebSocket event hub.
- `player-ui/` — complete mobile-first player UI / PWA.
- `admin-ui/` — standalone administration UI theme/template.
- `deploy/` — nginx, systemd, and install helper.
- `.github/workflows/release.yml` — tagged amd64/arm64 release publishing.
- `config/` — example player configuration.

## Development

Requirements:

- Go 1.24+
- Node.js 20+
- npm
- mpv (optional for UI/API development)

Start backend:

```bash
cd backend
go mod tidy
go run ./cmd/playerd
```

Start the player UI:

```bash
cd player-ui
npm install
npm run dev
```

Start the admin UI:

```bash
cd admin-ui
npm install
npm run dev
```

The Vite development servers proxy `/api` and `/ws` to `127.0.0.1:9091`.

## Raspberry Pi paths

Production defaults use:

```text
/srv/ekkoplayer/
├── music/
├── artwork/
├── database/
├── imports/
├── backup/
└── web/
    ├── player/
    └── admin/
```

## Build

```bash
make build
```

Copy the resulting assets:

```bash
sudo cp -r player-ui/dist/* /srv/ekkoplayer/web/player/
sudo cp -r admin-ui/dist/* /srv/ekkoplayer/web/admin/
sudo install -m 0755 backend/ekkoplayer /usr/local/bin/ekkoplayer
```

## URLs

- Player: `http://PI:9090/`
- Admin: `http://PI:9090/admin/`
- API: `http://PI:9090/api/v1/`
- WebSocket: `ws://PI:9090/ws`

## Raspberry Pi analog output

On Raspberry Pi OS, force analog output if necessary with `raspi-config` or the appropriate ALSA/PipeWire configuration for the image you use. The default example config asks mpv for `alsa/default`; change `audio_device` after checking:

```bash
mpv --audio-device=help
```

## Production capabilities

The backend includes versioned SQLite migrations, asynchronous upload and staged-folder imports, ffprobe metadata, content-hash duplicate detection, managed library paths, artwork extraction, tag writeback with pre-edit backups, playlists, radio, transactional queue state, persistent player checkpoints, REST control endpoints, and WebSocket broadcasts. Production configuration starts with an empty library; demo seeding is opt-in.

The mobile UI includes Home, Search, Library, Albums, Artists, Playlists, Radio, Queue, full Now Playing, a persistent mini-player, light/dark/night/automatic themes, reconnection behavior, touch-sized controls, responsive portrait/landscape layouts, and PWA metadata.

The admin application uses live dashboard, library, playback, import, playlist, radio, storage, health, and backup data. It includes paginated library search, atomic tag/artwork editing, recoverable deletion, playlist membership management, and radio CRUD/testing. Imports support uploads, folder scans, progress, duplicate reporting, retry, and cancellation.

mpv is supervised in-process with bounded restart backoff. After an mpv crash, the service restores the persisted track and position paused without restarting `playerd`. Shuffle order is generated once, persisted in SQLite, and reconciled transactionally as the queue changes.

Appliance upgrades are managed from Admin → Settings. Update checks use the configured GitHub Releases repository; installation runs in a separate root-owned systemd unit with checksum and architecture verification, an atomic release symlink, health validation, and rollback.

## Release and appliance installation

Build an aarch64 release with `make release-arm64`. On the Pi, run `deploy/install.sh` once, then deploy the generated `release/` directory with `deploy/release.sh`. Releases are stored under `/opt/ekkoplayer/releases` and switched atomically through `/opt/ekkoplayer/current`.

Daily database/configuration backups retain seven archives. Create one immediately with `ekkoplayer backup`; restore only while `playerd` is stopped using `ekkoplayer restore BACKUP --confirm`. On an installed appliance the CLI automatically loads `/etc/ekkoplayer/player.json`; development can override it with `EKKOPLAYER_CONFIG`.

The default production configuration targets Raspberry Pi analog output at `alsa/hw:0,0`. The application assumes a trusted LAN and does not implement authentication or public-internet exposure.
