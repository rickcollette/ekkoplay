# Validation

Validated on 2026-08-31:

- `go test -race ./...` passes, including schema/migration, production-empty-library, transactional queue, and media path tests.
- Both React/Vite applications pass TypeScript and production builds.
- Chromium browser tests cover live dashboard rendering, paginated metadata editing, playlist/radio forms, and import workflows.
- Persisted shuffle order and paginated/searchable song queries have backend regression coverage.
- The backend cross-compiles as static Linux ARM64 and AMD64 executables without QEMU.
- Local health and statistics API smoke checks pass.
- Release archives contain matching architecture markers and native binaries for ARM64 and AMD64.
- The initializer accepts Debian 12/13 and Ubuntu 22.04/24.04 on either supported architecture.
- Audio configuration defaults to generic `alsa/default` and validates configured ALSA devices.
- Player and admin assets respond through nginx.
- A tagged FLAC upload was imported into the managed library with extracted technical metadata.
- Uploading identical content produced a duplicate job referencing the existing song.
- Managed song deletion removed the track and orphaned artist/album records.
- Backup creation, integrity-checked restore, service restart, and post-restore health checks pass.
