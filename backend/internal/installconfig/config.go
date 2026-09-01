package installconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type TLS struct {
	Enabled     bool   `json:"enabled"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key"`
}
type Paths struct {
	Data     string `json:"data"`
	Music    string `json:"music"`
	Imports  string `json:"imports"`
	Artwork  string `json:"artwork"`
	Backups  string `json:"backups"`
	Torrents string `json:"torrents"`
}
type Config struct {
	RoomName     string `json:"room_name"`
	HTTPPort     int    `json:"http_port"`
	TLS          TLS    `json:"tls"`
	Paths        Paths  `json:"paths"`
	AudioDevice  string `json:"audio_device"`
	AudioFilter  string `json:"audio_filter"`
	AudioOutputs []struct {
		Name              string `json:"name"`
		Device            string `json:"device"`
		Enabled           bool   `json:"enabled"`
		Primary           bool   `json:"primary"`
		VolumeTrim        int    `json:"volume_trim"`
		Muted             bool   `json:"muted"`
		DelayMS           int    `json:"delay_ms"`
		BufferMS          int    `json:"buffer_ms"`
		Channels          string `json:"channels"`
		SampleRate        int    `json:"sample_rate"`
		Format            string `json:"format"`
		Exclusive         bool   `json:"exclusive"`
		Filter            string `json:"filter"`
		DriftCorrectionMS int    `json:"drift_correction_ms"`
	} `json:"audio_outputs,omitempty"`
	DefaultVolume   int    `json:"default_volume"`
	MaximumVolume   int    `json:"maximum_volume"`
	ImportWorkers   int    `json:"import_workers"`
	AcoustIDKey     string `json:"acoustid_key"`
	TorrentPeerPort int    `json:"torrent_peer_port"`
	TorrentSeedDays int    `json:"torrent_seed_days"`
	AdminUsername   string `json:"admin_username"`
	AdminPassword   string `json:"admin_password"`
}

func Load(path string) (Config, error) {
	var c Config
	f, e := os.Open(path)
	if e != nil {
		return c, e
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return c, e
	}
	if e = d.Decode(&struct{}{}); e != io.EOF {
		return c, errors.New("install config must contain one JSON object")
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	if strings.TrimSpace(c.RoomName) == "" {
		return errors.New("room_name is required")
	}
	if c.HTTPPort < 1 || c.HTTPPort > 65535 {
		return errors.New("http_port must be 1..65535")
	}
	if c.AudioDevice == "" {
		return errors.New("audio_device is required")
	}
	if len(c.AudioOutputs) > 0 {
		primary := 0
		names := map[string]bool{}
		for _, o := range c.AudioOutputs {
			key := strings.ToLower(strings.TrimSpace(o.Name))
			if key == "" || names[key] {
				return errors.New("audio output names must be non-empty and unique")
			}
			names[key] = true
			if o.Device == "" {
				return errors.New("audio output device is required")
			}
			if o.VolumeTrim < -100 || o.VolumeTrim > 100 || o.DelayMS < -5000 || o.DelayMS > 5000 || o.BufferMS < 20 || o.BufferMS > 5000 {
				return errors.New("invalid audio output trim, delay, or buffer")
			}
			if o.Enabled && o.Primary {
				primary++
			}
		}
		if primary != 1 {
			return errors.New("exactly one enabled audio output must be primary")
		}
	}
	if c.DefaultVolume < 0 || c.DefaultVolume > 100 || c.MaximumVolume < 1 || c.MaximumVolume > 100 || c.DefaultVolume > c.MaximumVolume {
		return errors.New("invalid volume limits")
	}
	if c.ImportWorkers < 1 || c.ImportWorkers > 4 {
		return errors.New("import_workers must be 1..4")
	}
	if c.TorrentPeerPort < 1 || c.TorrentPeerPort > 65535 {
		return errors.New("torrent_peer_port must be 1..65535")
	}
	if c.TorrentSeedDays < 1 || c.TorrentSeedDays > 365 {
		return errors.New("torrent_seed_days must be 1..365")
	}
	for n, p := range map[string]string{"data": c.Paths.Data, "music": c.Paths.Music, "imports": c.Paths.Imports, "artwork": c.Paths.Artwork, "backups": c.Paths.Backups, "torrents": c.Paths.Torrents} {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("paths.%s must be absolute", n)
		}
		if strings.ContainsAny(p, " \t\r\n") {
			return fmt.Errorf("paths.%s cannot contain whitespace", n)
		}
	}
	if c.TLS.Enabled {
		if !filepath.IsAbs(c.TLS.Certificate) || !filepath.IsAbs(c.TLS.PrivateKey) {
			return errors.New("TLS certificate and private_key must be absolute paths")
		}
		if _, e := os.Stat(c.TLS.Certificate); e != nil {
			return fmt.Errorf("TLS certificate: %w", e)
		}
		if _, e := os.Stat(c.TLS.PrivateKey); e != nil {
			return fmt.Errorf("TLS private key: %w", e)
		}
	}
	if c.AdminPassword != "" {
		if e := validateAdmin(c.AdminUsername, c.AdminPassword); e != nil {
			return e
		}
	} else if c.AdminUsername != "" && len(c.AdminUsername) < 3 {
		return errors.New("admin_username must be at least 3 characters")
	}
	return nil
}
func validateAdmin(u, p string) error {
	u = strings.TrimSpace(u)
	if len(u) < 3 || len(u) > 64 {
		return errors.New("admin_username must be 3..64 characters")
	}
	if len(p) < 12 {
		return errors.New("admin_password must contain at least 12 characters")
	}
	return nil
}
