package api

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

var playlistPalettes = []string{"aurora", "cobalt", "sunset", "orchid", "forest", "ember", "lagoon", "berry"}

func (s *Server) playlistArtwork(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	defer f.Close()
	tmp, err := os.CreateTemp(s.cfg.ImportPath, ".playlist-art-*")
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err = io.Copy(tmp, f); err != nil {
		tmp.Close()
		writeErr(w, 500, err)
		return
	}
	if err = tmp.Close(); err != nil {
		writeErr(w, 500, err)
		return
	}
	dest := filepath.Join(s.cfg.ArtworkPath, fmt.Sprintf("playlist-%d.webp", id))
	cmd := exec.CommandContext(r.Context(), s.cfg.FFmpegBinary, "-v", "error", "-i", tmpPath, "-vf", "scale=1024:1024:force_original_aspect_ratio=increase,crop=1024:1024", "-frames:v", "1", "-y", dest)
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		_ = os.Remove(dest)
		writeErr(w, 400, fmt.Errorf("invalid artwork: %s", string(out)))
		return
	}
	url := fmt.Sprintf("/art/playlist-%d.webp", id)
	if err = s.store.UpdatePlaylistArtwork(r.Context(), id, url); err != nil {
		_ = os.Remove(dest)
		if err == sql.ErrNoRows {
			writeErr(w, 404, err)
		} else {
			writeErr(w, 500, err)
		}
		return
	}
	s.hub.Broadcast("playlist.changed", map[string]any{"playlist_id": id, "artwork": url})
	s.playlists(w, r)
}

func (s *Server) resetPlaylistArtwork(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	var b [1]byte
	_, _ = rand.Read(b[:])
	art := "playlist-gradient:" + playlistPalettes[int(b[0])%len(playlistPalettes)]
	if err = s.store.UpdatePlaylistArtwork(r.Context(), id, art); err != nil {
		if err == sql.ErrNoRows {
			writeErr(w, 404, err)
		} else {
			writeErr(w, 500, err)
		}
		return
	}
	_ = os.Remove(filepath.Join(s.cfg.ArtworkPath, fmt.Sprintf("playlist-%d.webp", id)))
	s.hub.Broadcast("playlist.changed", map[string]any{"playlist_id": id, "artwork": art})
	s.playlists(w, r)
}
