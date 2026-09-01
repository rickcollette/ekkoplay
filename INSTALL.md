# ekkoPlayer first installation

Supported targets are Debian 12/13 and Ubuntu 22.04/24.04 on amd64 or arm64. Extract the matching archive, copy `install.example.json`, edit every site-specific value, then run:

```sh
sudo ./initialize.sh my-install.json
```

The initializer validates the configuration and host, installs apt dependencies, creates the appliance account and durable storage, migrates SQLite, seeds the seven-station radio directory, creates the first administrator, enables services, and verifies the result. It does not include or create audio tracks. After success, `admin_password` is atomically removed from the supplied file.

Re-running the initializer upgrades in place and preserves the database, JWT key, media, artwork, playlists, radio edits, torrents, and backups. Bootstrap credentials are ignored once the sole administrator exists. For root password recovery, put the new password in a mode-0600 file and run:

```sh
sudo EKKOPLAYER_CONFIG=/etc/ekkoplayer/player.json /opt/ekkoplayer/current/ekkoplayer admin reset-password --password-file /root/new-password
```

## Updates

After the initial bundle is installed, administrators can use **Admin → Settings → System updates** to check GitHub Releases and install a newer native build. The updater accepts only the configured project release, verifies the published SHA-256 checksum and bundle architecture, creates a backup, atomically activates the release, and waits for the health endpoint. A failed health check restores the previous release and database snapshot.

Release artifacts are produced for both architectures whenever a `v*` tag is pushed. The application version must use `major.minor.patch` form.

Open the configured TCP HTTP/HTTPS port for LAN clients and the configured TCP/UDP Transmission peer port for peers. Default HTTP sends credentials without transport encryption and is appropriate only on a trusted LAN. Configure TLS before exposing the appliance to an untrusted network.

Root-owned configuration and the persistent signing key are under `/etc/ekkoplayer`; versioned releases are under `/opt/ekkoplayer/releases`, with `/opt/ekkoplayer/current` switched atomically.
