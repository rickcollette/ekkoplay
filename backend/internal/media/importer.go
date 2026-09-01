package media

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ekkoplayer/internal/config"
	"ekkoplayer/internal/db"
)

type BroadcastFunc func(string, any)
type Manager struct {
	cfg       config.Config
	store     *db.Store
	wake      chan struct{}
	broadcast BroadcastFunc
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func NewManager(cfg config.Config, store *db.Store, broadcast BroadcastFunc) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{cfg: cfg, store: store, wake: make(chan struct{}, 1), broadcast: broadcast, cancel: cancel}
	if err := store.RequeueInterruptedImports(ctx); err != nil {
		slog.Error("requeue interrupted imports", "error", err)
	}
	for i := 0; i < cfg.ImportWorkers; i++ {
		m.wg.Add(1)
		go m.worker(ctx)
	}
	m.notify()
	return m
}
func (m *Manager) Close() { m.cancel(); m.wg.Wait() }
func (m *Manager) Enqueue(id int64) error {
	m.notify()
	return nil
}
func (m *Manager) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Manager) worker(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		id, err := m.store.ClaimImportJob(ctx)
		if err == nil {
			m.process(ctx, id)
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
			slog.Warn("claim import job", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		case <-ticker.C:
		}
	}
}
func (m *Manager) emit(id int64) {
	if m.broadcast != nil {
		if j, e := m.store.ImportJob(context.Background(), id); e == nil {
			m.broadcast("import.changed", j)
		}
	}
}
func (m *Manager) fail(ctx context.Context, id int64, err error) {
	_ = m.store.UpdateImportJob(ctx, id, "failed", err.Error(), 0, 0)
	m.emit(id)
}

type probe struct {
	Format struct {
		Duration string            `json:"duration"`
		BitRate  string            `json:"bit_rate"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		CodecType   string `json:"codec_type"`
		CodecName   string `json:"codec_name"`
		SampleRate  string `json:"sample_rate"`
		Channels    int    `json:"channels"`
		Disposition struct {
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
	} `json:"streams"`
}

func (m *Manager) process(ctx context.Context, id int64) {
	j, err := m.store.ImportJob(ctx, id)
	if err != nil {
		return
	}
	if j.Status == "cancelled" {
		return
	}
	_ = m.store.UpdateImportJob(ctx, id, "processing", "Reading metadata", 0, 0)
	m.emit(id)
	f, err := os.Open(j.SourcePath)
	if err != nil {
		m.fail(ctx, id, err)
		return
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		m.fail(ctx, id, err)
		return
	}
	hash := hex.EncodeToString(h.Sum(nil))
	if existing, e := m.store.SongByHash(ctx, hash); e == nil {
		_ = m.store.UpdateImportJob(ctx, id, "duplicate", "Identical audio already exists", 0, existing.ID)
		_ = os.Remove(j.SourcePath)
		m.emit(id)
		return
	} else if !errors.Is(e, sql.ErrNoRows) {
		m.fail(ctx, id, e)
		return
	}
	cmd := exec.CommandContext(ctx, m.cfg.FFprobeBinary, "-v", "error", "-show_format", "-show_streams", "-of", "json", j.SourcePath)
	raw, err := cmd.Output()
	if err != nil {
		m.fail(ctx, id, fmt.Errorf("ffprobe: %w", err))
		return
	}
	var p probe
	if err = json.Unmarshal(raw, &p); err != nil {
		m.fail(ctx, id, err)
		return
	}
	tags := lowerTags(p.Format.Tags)
	artist := value(tags, "album_artist", "artist")
	album := value(tags, "album")
	title := value(tags, "title")
	year := numberPrefix(value(tags, "date", "year"))
	track := numberPrefix(value(tags, "track"))
	disc := numberPrefix(value(tags, "disc"))
	genre := value(tags, "genre")
	if artist == "" {
		artist = "Unknown Artist"
	}
	if album == "" {
		album = "Unknown Album"
	}
	if title == "" {
		title = strings.TrimSuffix(j.Filename, filepath.Ext(j.Filename))
	}
	var codec string
	var rate, channels int
	for _, s := range p.Streams {
		if s.CodecType == "audio" {
			codec = s.CodecName
			rate, _ = strconv.Atoi(s.SampleRate)
			channels = s.Channels
			break
		}
	}
	duration, _ := strconv.ParseFloat(p.Format.Duration, 64)
	durationMS := int64(duration * 1000)
	if existing, lookupErr := m.store.SongByImportIdentity(ctx, j.Filename, durationMS); lookupErr == nil {
		_ = m.store.UpdateImportJob(ctx, id, "duplicate", "Same recording and source name already exist", 0, existing.ID)
		_ = os.Remove(j.SourcePath)
		m.emit(id)
		return
	} else if !errors.Is(lookupErr, sql.ErrNoRows) {
		m.fail(ctx, id, lookupErr)
		return
	}
	bitrate, _ := strconv.ParseInt(p.Format.BitRate, 10, 64)
	st, err := os.Stat(j.SourcePath)
	if err != nil {
		m.fail(ctx, id, err)
		return
	}
	ext := strings.ToLower(filepath.Ext(j.Filename))
	// Folder placement is user-managed. Imports stay flat at the library root
	// unless the upload explicitly targets a folder from the admin folder UI.
	dir := m.cfg.MusicPath
	if j.TargetFolder != "" {
		dir = filepath.Join(m.cfg.MusicPath, filepath.FromSlash(j.TargetFolder))
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		m.fail(ctx, id, err)
		return
	}
	prefix := ""
	if track > 0 {
		if disc > 1 {
			prefix = fmt.Sprintf("%02d-", disc)
		}
		prefix += fmt.Sprintf("%02d - ", track)
	}
	dest := uniquePath(filepath.Join(dir, prefix+safe(title)+ext))
	if err = copyAtomic(j.SourcePath, dest, 0644); err != nil {
		m.fail(ctx, id, err)
		return
	}
	art := m.extractArtwork(ctx, j.SourcePath, hash)
	songID, err := m.store.InsertImportedSong(ctx, db.ImportedSong{Title: title, Artist: artist, Album: album, Year: year, TrackNumber: track, DiscNumber: disc, DurationMS: durationMS, FilePath: dest, Format: strings.TrimPrefix(strings.ToUpper(ext), "."), Artwork: art, SHA256: hash, OriginalFilename: j.Filename, Codec: codec, Genre: genre, Bitrate: bitrate, SampleRate: rate, Channels: channels, FileSize: st.Size()})
	if err != nil {
		_ = os.Remove(dest)
		if existing, lookupErr := m.store.SongByHash(ctx, hash); lookupErr == nil {
			_ = m.store.UpdateImportJob(ctx, id, "duplicate", "Identical audio already exists", 0, existing.ID)
			_ = os.Remove(j.SourcePath)
			m.emit(id)
			return
		}
		m.fail(ctx, id, err)
		return
	}
	_ = m.store.UpdateImportJob(ctx, id, "imported", "Import complete", songID, 0)
	_ = os.Remove(j.SourcePath)
	m.emit(id)
	if m.broadcast != nil {
		m.broadcast("library.changed", map[string]any{"song_id": songID})
	}
}
func lowerTags(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[strings.ToLower(k)] = strings.TrimSpace(v)
	}
	return out
}
func value(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if m[k] != "" {
			return m[k]
		}
	}
	return ""
}
func numberPrefix(v string) int {
	v = strings.Split(v, "/")[0]
	n, _ := strconv.Atoi(regexp.MustCompile(`[^0-9].*$`).ReplaceAllString(v, ""))
	return n
}

var unsafe = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

func safe(v string) string {
	v = strings.Trim(strings.TrimSpace(unsafe.ReplaceAllString(v, "_")), ". ")
	if v == "" {
		return "Unknown"
	}
	r := []rune(v)
	if len(r) > 120 {
		v = string(r[:120])
	}
	return v
}
func uniquePath(p string) string {
	if _, e := os.Stat(p); os.IsNotExist(e) {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 2; ; i++ {
		x := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, e := os.Stat(x); os.IsNotExist(e) {
			return x
		}
	}
}
func copyAtomic(src, dst string, mode os.FileMode) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.CreateTemp(filepath.Dir(dst), ".import-*")
	if e != nil {
		return e
	}
	tmp := out.Name()
	ok := false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, e = io.Copy(out, in); e != nil {
		return e
	}
	if e = out.Sync(); e != nil {
		return e
	}
	if e = out.Chmod(mode); e != nil {
		return e
	}
	if e = out.Close(); e != nil {
		return e
	}
	if e = os.Rename(tmp, dst); e != nil {
		return e
	}
	ok = true
	return nil
}
func (m *Manager) extractArtwork(ctx context.Context, src, hash string) string {
	dir := filepath.Join(m.cfg.ArtworkPath, hash[:2])
	if os.MkdirAll(dir, 0755) != nil {
		return ""
	}
	dst := filepath.Join(dir, hash+".webp")
	cmd := exec.CommandContext(ctx, m.cfg.FFmpegBinary, "-v", "error", "-i", src, "-map", "0:v:0", "-frames:v", "1", "-vf", "scale=1024:1024:force_original_aspect_ratio=decrease", "-y", dst)
	if cmd.Run() == nil {
		return "/art/" + hash[:2] + "/" + hash + ".webp"
	}
	os.Remove(dst)
	return ""
}
