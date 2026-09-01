package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Listen           string        `json:"listen"`
	DatabasePath     string        `json:"database_path"`
	MusicPath        string        `json:"music_path"`
	ImportPath       string        `json:"import_path"`
	ArtworkPath      string        `json:"artwork_path"`
	MPVBinary        string        `json:"mpv_binary"`
	MPVSocket        string        `json:"mpv_socket"`
	AudioDevice      string        `json:"audio_device"`
	AudioFilter      string        `json:"audio_filter"`
	DefaultVolume    int           `json:"default_volume"`
	MaximumVolume    int           `json:"maximum_volume"`
	StartMPV         bool          `json:"start_mpv"`
	SeedDemo         bool          `json:"seed_demo"`
	BackupPath       string        `json:"backup_path"`
	FFmpegBinary     string        `json:"ffmpeg_binary"`
	FFprobeBinary    string        `json:"ffprobe_binary"`
	MaxUploadBytes   int64         `json:"max_upload_bytes"`
	ImportWorkers    int           `json:"import_workers"`
	AcoustIDKey      string        `json:"acoustid_key"`
	FPCalcBinary     string        `json:"fpcalc_binary"`
	TorrentBinary    string        `json:"torrent_binary"`
	TorrentPath      string        `json:"torrent_path"`
	TorrentRPCURL    string        `json:"torrent_rpc_url"`
	JWTSecretPath    string        `json:"jwt_secret_path"`
	CookieSecure     bool          `json:"cookie_secure"`
	RoomName         string        `json:"room_name"`
	TorrentPeerPort  int           `json:"torrent_peer_port"`
	TorrentSeedDays  int           `json:"torrent_seed_days"`
	AudioOutputs     []AudioOutput `json:"audio_outputs,omitempty"`
	UpdateRepository string        `json:"update_repository"`
}

type AudioOutput struct {
	Name              string `json:"name"`
	Device            string `json:"device"`
	Enabled           bool   `json:"enabled"`
	Primary           bool   `json:"primary"`
	VolumeTrim        int    `json:"volume_trim"`
	Muted             bool   `json:"muted"`
	DelayMS           int    `json:"delay_ms"`
	BufferMS          int    `json:"buffer_ms"`
	Channels          string `json:"channels,omitempty"`
	SampleRate        int    `json:"sample_rate,omitempty"`
	Format            string `json:"format,omitempty"`
	Exclusive         bool   `json:"exclusive,omitempty"`
	Filter            string `json:"filter,omitempty"`
	DriftCorrectionMS int    `json:"drift_correction_ms,omitempty"`
}

func Defaults() Config {
	base := filepath.Join(os.TempDir(), "ekkoplayer-dev")
	return Config{
		Listen:           "127.0.0.1:9091",
		DatabasePath:     filepath.Join(base, "player.db"),
		MusicPath:        filepath.Join(base, "music"),
		ImportPath:       filepath.Join(base, "imports"),
		ArtworkPath:      filepath.Join(base, "artwork"),
		MPVBinary:        "/usr/bin/mpv",
		MPVSocket:        filepath.Join(base, "mpv.sock"),
		AudioDevice:      "alsa/default",
		AudioFilter:      "",
		DefaultVolume:    55,
		MaximumVolume:    85,
		StartMPV:         false,
		SeedDemo:         true,
		BackupPath:       filepath.Join(base, "backup"),
		FFmpegBinary:     "/usr/bin/ffmpeg",
		FFprobeBinary:    "/usr/bin/ffprobe",
		MaxUploadBytes:   16 << 30,
		ImportWorkers:    2,
		FPCalcBinary:     "/usr/bin/fpcalc",
		TorrentBinary:    "/usr/bin/transmission-daemon",
		TorrentPath:      filepath.Join(base, "torrents"),
		TorrentRPCURL:    "http://127.0.0.1:9092/transmission/rpc",
		JWTSecretPath:    filepath.Join(base, "jwt.key"),
		RoomName:         "Music Room",
		TorrentPeerPort:  51413,
		TorrentSeedDays:  14,
		UpdateRepository: "rickcollette/ekkoplay",
	}
}

func Load() (Config, error) {
	cfg := Defaults()
	path := os.Getenv("EKKOPLAYER_CONFIG")
	if path == "" {
		const applianceConfig = "/etc/ekkoplayer/player.json"
		if _, err := os.Stat(applianceConfig); err == nil {
			path = applianceConfig
		}
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return cfg, err
		}
		// Existing appliance configs predate torrent settings. Keep their torrent
		// payload and Transmission state on durable appliance storage.
		if path == "/etc/ekkoplayer/player.json" && cfg.TorrentPath == filepath.Join(basePath(), "torrents") {
			cfg.TorrentPath = "/srv/ekkoplayer/torrents"
		}
		if path == "/etc/ekkoplayer/player.json" && cfg.JWTSecretPath == filepath.Join(basePath(), "jwt.key") {
			cfg.JWTSecretPath = "/etc/ekkoplayer/jwt.key"
		}
	}

	setString(&cfg.Listen, "EKKOPLAYER_LISTEN")
	setString(&cfg.DatabasePath, "EKKOPLAYER_DATABASE")
	setString(&cfg.MusicPath, "EKKOPLAYER_MUSIC")
	setString(&cfg.ImportPath, "EKKOPLAYER_IMPORTS")
	setString(&cfg.ArtworkPath, "EKKOPLAYER_ARTWORK")
	setString(&cfg.BackupPath, "EKKOPLAYER_BACKUP")
	setString(&cfg.AudioDevice, "EKKOPLAYER_AUDIO_DEVICE")
	setString(&cfg.AudioFilter, "EKKOPLAYER_AUDIO_FILTER")
	setString(&cfg.AcoustIDKey, "EKKOPLAYER_ACOUSTID_KEY")
	setString(&cfg.FPCalcBinary, "EKKOPLAYER_FPCALC")
	setString(&cfg.TorrentBinary, "EKKOPLAYER_TORRENT_BINARY")
	setString(&cfg.TorrentPath, "EKKOPLAYER_TORRENTS")
	setString(&cfg.TorrentRPCURL, "EKKOPLAYER_TORRENT_RPC")
	setString(&cfg.JWTSecretPath, "EKKOPLAYER_JWT_SECRET")
	setString(&cfg.RoomName, "EKKOPLAYER_ROOM_NAME")
	if v := os.Getenv("EKKOPLAYER_START_MPV"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, err
		}
		cfg.StartMPV = parsed
	}
	if cfg.MaximumVolume < 1 || cfg.MaximumVolume > 100 {
		return cfg, errors.New("maximum_volume must be 1..100")
	}
	if cfg.DefaultVolume > cfg.MaximumVolume {
		cfg.DefaultVolume = cfg.MaximumVolume
	}
	if cfg.MaxUploadBytes < 1 {
		return cfg, errors.New("max_upload_bytes must be positive")
	}
	if cfg.ImportWorkers < 1 || cfg.ImportWorkers > 4 {
		return cfg, errors.New("import_workers must be 1..4")
	}
	if cfg.TorrentPeerPort < 1 || cfg.TorrentPeerPort > 65535 {
		return cfg, errors.New("torrent_peer_port must be 1..65535")
	}
	if cfg.TorrentSeedDays < 1 || cfg.TorrentSeedDays > 365 {
		return cfg, errors.New("torrent_seed_days must be 1..365")
	}
	if len(cfg.AudioOutputs) == 0 {
		cfg.AudioOutputs = []AudioOutput{{Name: "Main", Device: cfg.AudioDevice, Enabled: true, Primary: true, BufferMS: 100, Filter: cfg.AudioFilter, DriftCorrectionMS: 40}}
	}
	primary := 0
	names := map[string]bool{}
	for i := range cfg.AudioOutputs {
		o := &cfg.AudioOutputs[i]
		o.Name = strings.TrimSpace(o.Name)
		if o.Name == "" || names[strings.ToLower(o.Name)] {
			return cfg, errors.New("audio output names must be non-empty and unique")
		}
		names[strings.ToLower(o.Name)] = true
		if o.Device == "" {
			return cfg, errors.New("audio output device is required")
		}
		if o.VolumeTrim < -100 || o.VolumeTrim > 100 {
			return cfg, errors.New("audio output volume_trim must be -100..100")
		}
		if o.DelayMS < -5000 || o.DelayMS > 5000 {
			return cfg, errors.New("audio output delay_ms must be -5000..5000")
		}
		if o.BufferMS < 20 || o.BufferMS > 5000 {
			return cfg, errors.New("audio output buffer_ms must be 20..5000")
		}
		if o.Primary && o.Enabled {
			primary++
		}
	}
	if primary != 1 {
		return cfg, errors.New("exactly one enabled audio output must be primary")
	}

	for _, p := range []string{filepath.Dir(cfg.DatabasePath), cfg.MusicPath, cfg.ImportPath, cfg.ArtworkPath, cfg.BackupPath, cfg.TorrentPath, filepath.Dir(cfg.MPVSocket)} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func basePath() string { return filepath.Join(os.TempDir(), "ekkoplayer-dev") }

func setString(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}
