# Validation

Validated on 2026-08-31:

- `go test -race ./...` passes, including schema/migration, production-empty-library, transactional queue, and media path tests.
- Both React/Vite applications pass TypeScript and production builds.
- Chromium browser tests cover live dashboard rendering, paginated metadata editing, playlist/radio forms, and import workflows.
- Persisted shuffle order and paginated/searchable song queries have backend regression coverage.
- The backend cross-compiles as a static Linux/aarch64 executable.
- Local health and statistics API smoke checks pass.
- Deployed to the configured Raspberry Pi 4 build host, running Debian 13/aarch64.
- nginx, playerd, and the daily backup timer are active.
- mpv starts against Raspberry Pi analog ALSA output `hw:0,0`.
- Player and admin assets respond through nginx.
- A tagged FLAC upload was imported into the managed library with extracted technical metadata.
- Uploading identical content produced a duplicate job referencing the existing song.
- Managed song deletion removed the track and orphaned artist/album records.
- Backup creation, integrity-checked restore, service restart, and post-restore health checks pass.
