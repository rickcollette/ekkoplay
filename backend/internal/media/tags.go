package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"ekkoplayer/internal/config"
	"ekkoplayer/internal/model"
)

type SongEdits struct {
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	Year        int    `json:"year"`
	TrackNumber int    `json:"track_number"`
	DiscNumber  int    `json:"disc_number"`
	Genre       string `json:"genre"`
}

func RewriteTags(ctx context.Context, c config.Config, s model.Song, e SongEdits) error {
	if e.Title == "" || e.Artist == "" || e.Album == "" {
		return fmt.Errorf("title, artist and album are required")
	}
	ext := filepath.Ext(s.FilePath)
	switch s.Format {
	case "MP3", "FLAC", "M4A", "OGG", "OPUS", "WAV", "AAC":
	default:
		return fmt.Errorf("tag writeback is not supported for %s", s.Format)
	}
	backupDir := filepath.Join(c.BackupPath, "pre-edit")
	if err := os.MkdirAll(backupDir, 0750); err != nil {
		return err
	}
	backupPath := filepath.Join(backupDir, fmt.Sprintf("song-%d-%s%s", s.ID, time.Now().UTC().Format("20060102T150405Z"), ext))
	if err := copyAtomic(s.FilePath, backupPath, 0600); err != nil {
		return fmt.Errorf("pre-edit backup: %w", err)
	}
	tmp := filepath.Join(filepath.Dir(s.FilePath), fmt.Sprintf(".tag-%d%s", time.Now().UnixNano(), ext))
	defer os.Remove(tmp)
	args := []string{"-v", "error", "-i", s.FilePath, "-map", "0", "-c", "copy", "-metadata", "title=" + e.Title, "-metadata", "artist=" + e.Artist, "-metadata", "album_artist=" + e.Artist, "-metadata", "album=" + e.Album, "-metadata", "date=" + strconv.Itoa(e.Year), "-metadata", "track=" + strconv.Itoa(e.TrackNumber), "-metadata", "disc=" + strconv.Itoa(e.DiscNumber), "-metadata", "genre=" + e.Genre, "-y", tmp}
	if out, err := exec.CommandContext(ctx, c.FFmpegBinary, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg tag write: %v: %s", err, string(out))
	}
	if out, err := exec.CommandContext(ctx, c.FFprobeBinary, "-v", "error", "-show_format", tmp).CombinedOutput(); err != nil {
		return fmt.Errorf("rewritten file validation failed: %v: %s", err, string(out))
	}
	st, err := os.Stat(s.FilePath)
	if err != nil {
		return err
	}
	if err = os.Chmod(tmp, st.Mode()); err != nil {
		return err
	}
	return os.Rename(tmp, s.FilePath)
}
