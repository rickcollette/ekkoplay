package installconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRejectsUnknownAndInvalidCriticalValues(t *testing.T) {
	dir := t.TempDir()
	unknown := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{"unexpected":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(unknown); err == nil {
		t.Fatal("unknown field accepted")
	}
	c := Config{RoomName: "Room", HTTPPort: 9090, Paths: Paths{Data: "/srv/ekko", Music: "/srv/ekko/music", Imports: "/srv/ekko/imports", Artwork: "/srv/ekko/art", Backups: "/srv/ekko/backups", Torrents: "/srv/ekko/torrents"}, AudioDevice: "alsa/default", DefaultVolume: 90, MaximumVolume: 80, ImportWorkers: 2, TorrentPeerPort: 51413, TorrentSeedDays: 14, AdminUsername: "admin", AdminPassword: "a long password value"}
	if err := c.Validate(); err == nil {
		t.Fatal("invalid volume limits accepted")
	}
}
