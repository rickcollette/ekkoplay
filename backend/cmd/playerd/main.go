package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ekkoplayer/internal/api"
	"ekkoplayer/internal/auth"
	"ekkoplayer/internal/backup"
	"ekkoplayer/internal/config"
	"ekkoplayer/internal/db"
	"ekkoplayer/internal/installconfig"
	"ekkoplayer/internal/media"
	"ekkoplayer/internal/player"
)

func main() {
	if len(os.Args) == 4 && os.Args[1] == "install" && os.Args[2] == "validate" {
		if _, err := installconfig.Load(os.Args[3]); err != nil {
			slog.Error("install config", "error", err)
			os.Exit(1)
		}
		fmt.Println("install config valid")
		return
	}
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "restore" {
		if len(os.Args) != 4 || os.Args[3] != "--confirm" {
			slog.Error("usage: ekkoplayer restore BACKUP --confirm")
			os.Exit(2)
		}
		if err := backup.Restore(os.Args[2], cfg.DatabasePath); err != nil {
			slog.Error("restore", "error", err)
			os.Exit(1)
		}
		slog.Info("restore complete")
		return
	}
	store, err := db.OpenWithDemo(cfg.DatabasePath, cfg.SeedDemo)
	if err != nil {
		slog.Error("database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	if len(os.Args) > 2 && os.Args[1] == "admin" {
		if err := adminCommand(store, cfg, os.Args[2], os.Args[3:]); err != nil {
			slog.Error("admin command", "error", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "backup" {
		if _, err := backup.Create(context.Background(), cfg, store); err != nil {
			slog.Error("backup", "error", err)
			os.Exit(1)
		}
		return
	}
	hub := api.NewHub()
	authService, err := loadAuth(store, cfg)
	if err != nil {
		slog.Error("authentication", "error", err)
		os.Exit(1)
	}
	if n, e := authService.AdminCount(context.Background()); e != nil || n != 1 {
		if e == nil {
			e = fmt.Errorf("expected exactly one active administrator, found %d", n)
		}
		slog.Error("authentication", "error", e)
		os.Exit(1)
	}
	var engine player.Engine
	var mirrored *player.MirroredEngine
	if cfg.StartMPV {
		type override struct {
			trim  int
			muted bool
			delay int
		}
		overrides := map[string]override{}
		if rows, qerr := store.DB.Query(`SELECT name,volume_trim,muted,delay_ms FROM audio_output_overrides`); qerr == nil {
			for rows.Next() {
				var n string
				var trim, mu, delay int
				if rows.Scan(&n, &trim, &mu, &delay) == nil {
					overrides[strings.ToLower(n)] = override{trim, mu != 0, delay}
				}
			}
			rows.Close()
		}
		zones := make([]player.ZoneConfig, 0, len(cfg.AudioOutputs))
		for _, o := range cfg.AudioOutputs {
			if x, ok := overrides[strings.ToLower(o.Name)]; ok {
				o.VolumeTrim, o.Muted, o.DelayMS = x.trim, x.muted, x.delay
			}
			zones = append(zones, player.ZoneConfig{Name: o.Name, Device: o.Device, Enabled: o.Enabled, Primary: o.Primary, VolumeTrim: o.VolumeTrim, Muted: o.Muted, DelayMS: o.DelayMS, BufferMS: o.BufferMS, Channels: o.Channels, SampleRate: o.SampleRate, Format: o.Format, Exclusive: o.Exclusive, Filter: o.Filter, DriftCorrectionMS: o.DriftCorrectionMS})
		}
		m, e := player.StartMirroredMPV(cfg.MPVBinary, cfg.MPVSocket, zones, cfg.DefaultVolume)
		if e != nil {
			slog.Error("mpv", "error", e)
			os.Exit(1)
		}
		engine, mirrored = m, m
		slog.Info("mirrored audio started", "outputs", len(zones))
	} else {
		engine = player.NewNoopEngine()
		slog.Warn("mpv disabled; API is running in development mode")
	}
	controller := player.NewController(store, engine, cfg.MaximumVolume, hub.Broadcast)
	defer controller.Close()
	imports := media.NewManager(cfg, store, hub.Broadcast)
	defer imports.Close()
	torrents := media.NewTorrentManager(cfg, store, imports, hub.Broadcast)
	defer torrents.Close()
	enricher := media.NewEnricher(cfg, store, hub.Broadcast)
	defer enricher.Close()
	server := api.NewServer(cfg, store, controller, hub, imports)
	server.SetEnricher(enricher)
	server.SetTorrents(torrents)
	server.SetAuth(authService)
	server.SetAudio(mirrored)
	httpServer := &http.Server{Addr: cfg.Listen, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("ekkoplayer listening", "address", cfg.Listen)
		if e := httpServer.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			slog.Error("http", "error", e)
			os.Exit(1)
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func loadAuth(store *db.Store, cfg config.Config) (*auth.Service, error) {
	a, err := auth.NewFromFile(store.DB, cfg.JWTSecretPath, cfg.CookieSecure)
	if err == nil {
		return a, nil
	}
	// Development defaults remain zero-setup. Appliance paths must be provisioned
	// explicitly by initialize.sh and never receive an ephemeral signing key.
	if strings.HasPrefix(cfg.JWTSecretPath, os.TempDir()+string(os.PathSeparator)) {
		key := make([]byte, 64)
		if _, e := rand.Read(key); e != nil {
			return nil, e
		}
		if e := os.WriteFile(cfg.JWTSecretPath, key, 0o600); e != nil {
			return nil, e
		}
		return auth.New(store.DB, key, cfg.CookieSecure)
	}
	return nil, err
}

func adminCommand(store *db.Store, cfg config.Config, command string, args []string) error {
	if command == "exists" {
		a, err := loadAuth(store, cfg)
		if err != nil {
			return err
		}
		n, err := a.AdminCount(context.Background())
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("expected exactly one active administrator, found %d", n)
		}
		fmt.Println("administrator exists")
		return nil
	}
	fs := flag.NewFlagSet("admin "+command, flag.ContinueOnError)
	username := fs.String("username", "", "administrator username")
	passwordFile := fs.String("password-file", "", "file containing the new password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *passwordFile == "" {
		return errors.New("--password-file is required")
	}
	b, err := os.ReadFile(*passwordFile)
	if err != nil {
		return err
	}
	password := strings.TrimRight(string(b), "\r\n")
	a, err := loadAuth(store, cfg)
	if err != nil {
		return err
	}
	switch command {
	case "create":
		err = a.CreateInitialAdmin(context.Background(), *username, password)
	case "reset-password":
		err = a.ResetPassword(context.Background(), *username, password)
	default:
		return fmt.Errorf("unknown admin command %q", command)
	}
	return err
}
