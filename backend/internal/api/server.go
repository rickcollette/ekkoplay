package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	authn "ekkoplayer/internal/auth"
	"ekkoplayer/internal/backup"
	"ekkoplayer/internal/buildinfo"
	"ekkoplayer/internal/config"
	"ekkoplayer/internal/db"
	"ekkoplayer/internal/media"
	"ekkoplayer/internal/model"
	"ekkoplayer/internal/player"
	updater "ekkoplayer/internal/update"
)

type Server struct {
	cfg           config.Config
	store         *db.Store
	player        *player.Controller
	hub           *Hub
	imports       *media.Manager
	enricher      *media.Enricher
	torrents      *media.TorrentManager
	mux           *http.ServeMux
	auth          *authn.Service
	audio         *player.MirroredEngine
	updates       *updater.Service
	loginMu       sync.Mutex
	loginAttempts map[string]loginAttempt
}

func NewServer(cfg config.Config, store *db.Store, p *player.Controller, hub *Hub, imports *media.Manager) *Server {
	s := &Server{cfg: cfg, store: store, player: p, hub: hub, imports: imports, mux: http.NewServeMux(), loginAttempts: make(map[string]loginAttempt), updates: updater.New(cfg.UpdateRepository, filepath.Join(filepath.Dir(cfg.DatabasePath), "..", "update"), buildinfo.Version)}
	s.routes()
	return s
}
func (s *Server) SetEnricher(e *media.Enricher)       { s.enricher = e }
func (s *Server) SetTorrents(t *media.TorrentManager) { s.torrents = t }
func (s *Server) SetAuth(a *authn.Service)            { s.auth = a }
func (s *Server) SetAudio(a *player.MirroredEngine)   { s.audio = a }
func (s *Server) Handler() http.Handler               { return logRequest(s.mux) }

func (s *Server) routes() {
	s.mux.Handle("GET /ws", s.hub)
	s.mux.HandleFunc("GET /admin/ws", s.requireAdmin(s.hub.ServeAdmin))
	s.mux.HandleFunc("GET /api/v1/health", s.health)
	s.mux.HandleFunc("GET /api/v1/version", s.version)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.login)
	s.mux.HandleFunc("POST /api/v1/auth/refresh", s.refresh)
	s.mux.HandleFunc("GET /api/v1/auth/session", s.requireAdmin(s.session))
	s.mux.HandleFunc("POST /api/v1/auth/logout", s.requireAdmin(s.logout))
	s.mux.HandleFunc("POST /api/v1/auth/logout-all", s.requireAdmin(s.logoutAll))
	s.mux.HandleFunc("POST /api/v1/auth/password", s.requireAdmin(s.changePassword))
	s.mux.HandleFunc("GET /api/v1/home", s.home)
	s.mux.HandleFunc("GET /api/v1/player", s.playerState)
	s.mux.HandleFunc("POST /api/v1/player/play", s.play)
	s.mux.HandleFunc("POST /api/v1/player/pause", s.pause)
	s.mux.HandleFunc("POST /api/v1/player/stop", s.stop)
	s.mux.HandleFunc("POST /api/v1/player/next", s.next)
	s.mux.HandleFunc("POST /api/v1/player/previous", s.previous)
	s.mux.HandleFunc("POST /api/v1/player/seek", s.seek)
	s.mux.HandleFunc("PUT /api/v1/player/volume", s.volume)
	s.mux.HandleFunc("PUT /api/v1/player/shuffle", s.shuffle)
	s.mux.HandleFunc("PUT /api/v1/player/mute", s.mute)
	s.mux.HandleFunc("POST /api/v1/admin/player/recover", s.requireAdmin(s.recoverPlayer))
	s.mux.HandleFunc("POST /api/v1/admin/system/restart", s.requireAdmin(s.restartSystem))
	s.mux.HandleFunc("GET /api/v1/admin/system/update", s.requireAdmin(s.updateStatus))
	s.mux.HandleFunc("POST /api/v1/admin/system/update/check", s.requireAdmin(s.checkUpdate))
	s.mux.HandleFunc("POST /api/v1/admin/system/update/apply", s.requireAdmin(s.applyUpdate))

	s.mux.HandleFunc("GET /api/v1/songs", s.songs)
	s.mux.HandleFunc("GET /api/v1/songs/{id}", s.song)
	s.mux.HandleFunc("PATCH /api/v1/songs/{id}", s.updateSong)
	s.mux.HandleFunc("DELETE /api/v1/songs/{id}", s.requireAdmin(s.deleteSong))
	s.mux.HandleFunc("POST /api/v1/songs/{id}/artwork", s.requireAdmin(s.songArtwork))
	s.mux.HandleFunc("GET /api/v1/albums", s.albums)
	s.mux.HandleFunc("GET /api/v1/albums/{id}/songs", s.albumSongs)
	s.mux.HandleFunc("GET /api/v1/artists", s.artists)
	s.mux.HandleFunc("GET /api/v1/artists/{id}/songs", s.artistSongs)
	s.mux.HandleFunc("GET /api/v1/search", s.search)

	s.mux.HandleFunc("GET /api/v1/playlists", s.playlists)
	s.mux.HandleFunc("POST /api/v1/playlists", s.requireAdmin(s.createPlaylist))
	s.mux.HandleFunc("DELETE /api/v1/playlists/{id}", s.requireAdmin(s.deletePlaylist))
	s.mux.HandleFunc("POST /api/v1/playlists/{id}/artwork", s.requireAdmin(s.playlistArtwork))
	s.mux.HandleFunc("DELETE /api/v1/playlists/{id}/artwork", s.requireAdmin(s.resetPlaylistArtwork))
	s.mux.HandleFunc("POST /api/v1/playlists/{id}/play", s.playPlaylist)
	s.mux.HandleFunc("GET /api/v1/playlists/{id}/songs", s.playlistSongs)
	s.mux.HandleFunc("POST /api/v1/playlists/{id}/songs", s.requireAdmin(s.addPlaylistSong))
	s.mux.HandleFunc("PUT /api/v1/playlists/{id}/songs", s.requireAdmin(s.reorderPlaylistSongs))
	s.mux.HandleFunc("DELETE /api/v1/playlists/{id}/songs/{songID}", s.requireAdmin(s.removePlaylistSong))
	s.mux.HandleFunc("GET /api/v1/radio", s.radio)
	s.mux.HandleFunc("POST /api/v1/radio", s.requireAdmin(s.createRadio))
	s.mux.HandleFunc("PUT /api/v1/radio/{id}", s.requireAdmin(s.updateRadio))
	s.mux.HandleFunc("DELETE /api/v1/radio/{id}", s.requireAdmin(s.deleteRadio))
	s.mux.HandleFunc("POST /api/v1/radio/{id}/play", s.playRadio)
	s.mux.HandleFunc("GET /api/v1/queue", s.queue)
	s.mux.HandleFunc("POST /api/v1/queue", s.addQueue)
	s.mux.HandleFunc("DELETE /api/v1/queue", s.clearQueue)
	s.mux.HandleFunc("DELETE /api/v1/queue/{id}", s.removeQueue)
	s.mux.HandleFunc("PUT /api/v1/queue", s.reorderQueue)

	s.mux.HandleFunc("GET /api/v1/admin/storage", s.requireAdmin(s.storage))
	s.mux.HandleFunc("GET /api/v1/admin/audio/outputs", s.requireAdmin(s.audioOutputs))
	s.mux.HandleFunc("PATCH /api/v1/admin/audio/outputs/{name}", s.requireAdmin(s.updateAudioOutput))
	s.mux.HandleFunc("GET /api/v1/admin/audio/devices", s.requireAdmin(s.audioDevices))
	s.mux.HandleFunc("GET /api/v1/admin/folders", s.requireAdmin(s.folders))
	s.mux.HandleFunc("POST /api/v1/admin/folders", s.requireAdmin(s.createFolder))
	s.mux.HandleFunc("PATCH /api/v1/admin/folders", s.requireAdmin(s.moveFolder))
	s.mux.HandleFunc("DELETE /api/v1/admin/folders", s.requireAdmin(s.deleteFolder))
	s.mux.HandleFunc("POST /api/v1/admin/folders/move-songs", s.requireAdmin(s.moveSongs))
	s.mux.HandleFunc("PATCH /api/v1/admin/files/{id}", s.requireAdmin(s.renameMusicFile))
	s.mux.HandleFunc("POST /api/v1/admin/upload", s.requireAdmin(s.upload))
	s.mux.HandleFunc("GET /api/v1/admin/imports", s.requireAdmin(s.importJobs))
	s.mux.HandleFunc("GET /api/v1/admin/torrents", s.requireAdmin(s.torrentJobs))
	s.mux.HandleFunc("POST /api/v1/admin/torrents", s.requireAdmin(s.addTorrent))
	s.mux.HandleFunc("DELETE /api/v1/admin/torrents/{id}", s.requireAdmin(s.deleteTorrent))
	s.mux.HandleFunc("POST /api/v1/admin/torrents/{id}/extend", s.requireAdmin(s.extendTorrent))
	s.mux.HandleFunc("POST /api/v1/admin/imports/scan", s.requireAdmin(s.scanImports))
	s.mux.HandleFunc("POST /api/v1/admin/imports/{id}/retry", s.requireAdmin(s.retryImport))
	s.mux.HandleFunc("DELETE /api/v1/admin/imports/{id}", s.requireAdmin(s.cancelImport))
	s.mux.HandleFunc("GET /api/v1/admin/stats", s.requireAdmin(s.stats))
	s.mux.HandleFunc("GET /api/v1/admin/enrichment", s.requireAdmin(s.enrichmentStats))
	s.mux.HandleFunc("POST /api/v1/admin/enrichment/run", s.requireAdmin(s.runEnrichment))
	s.mux.HandleFunc("POST /api/v1/admin/enrichment/retry", s.requireAdmin(s.retryEnrichment))
	s.mux.HandleFunc("GET /api/v1/admin/backups", s.requireAdmin(s.backups))
	s.mux.HandleFunc("POST /api/v1/admin/backups", s.requireAdmin(s.createBackup))
	s.mux.HandleFunc("GET /api/v1/admin/backups/{name}", s.requireAdmin(s.downloadBackup))
	s.mux.HandleFunc("GET /api/v1/admin/songs/{id}/download", s.requireAdmin(s.download))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeErr(w http.ResponseWriter, status int, e error) {
	code := "internal_error"
	switch status {
	case 400:
		code = "invalid_request"
	case 401:
		code = "unauthorized"
	case 403:
		code = "forbidden"
	case 404:
		code = "not_found"
	case 409:
		code = "conflict"
	case 413:
		code = "payload_too_large"
	case 429:
		code = "rate_limited"
	case 503:
		code = "unavailable"
	}
	writeJSON(w, status, map[string]any{"error": e.Error(), "code": code, "request_id": w.Header().Get("X-Request-ID")})
}
func decode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}
func pathID(r *http.Request) (int64, error) { return strconv.ParseInt(r.PathValue("id"), 10, 64) }

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if e := s.store.DB.PingContext(r.Context()); e != nil {
		writeErr(w, 503, e)
		return
	}
	writeJSON(w, 200, map[string]any{"status": "healthy", "database": "healthy", "controllers": s.hub.Count(), "version": buildinfo.Version, "room_name": s.cfg.RoomName, "time": time.Now()})
}
func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"version": buildinfo.Version})
}
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	p, e := s.player.State(r.Context())
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	recent, _ := s.store.RecentlyPlayed(r.Context())
	added, _ := s.store.RecentSongs(r.Context(), false)
	fav, _ := s.store.RecentSongs(r.Context(), true)
	pls, _ := s.store.Playlists(r.Context())
	rad, _ := s.store.Radio(r.Context())
	writeJSON(w, 200, model.HomeResponse{Player: p, RecentlyPlayed: recent, RecentlyAdded: added, Favorites: fav, Playlists: pls, Radio: rad})
}
func (s *Server) playerState(w http.ResponseWriter, r *http.Request) {
	x, e := s.player.State(r.Context())
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) play(w http.ResponseWriter, r *http.Request) {
	var b struct {
		SongID int64 `json:"song_id"`
	}
	if e := decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	if e := s.player.PlaySong(r.Context(), b.SongID); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.playerState(w, r)
}
func (s *Server) pause(w http.ResponseWriter, r *http.Request) {
	if e := s.player.Pause(r.Context()); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.playerState(w, r)
}
func (s *Server) recoverPlayer(w http.ResponseWriter, r *http.Request) {
	if e := s.player.Recover(r.Context()); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.playerState(w, r)
}
func (s *Server) restartSystem(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "restarting"})
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}()
}
func (s *Server) updateStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, s.updates.Status())
}
func (s *Server) checkUpdate(w http.ResponseWriter, r *http.Request) {
	v, e := s.updates.Check(r.Context())
	if e != nil {
		writeErr(w, 503, e)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) applyUpdate(w http.ResponseWriter, r *http.Request) {
	v, e := s.updates.Request(r.Context())
	if e != nil {
		writeErr(w, 409, e)
		return
	}
	writeJSON(w, http.StatusAccepted, v)
}
func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	if e := s.player.Stop(r.Context()); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.playerState(w, r)
}
func (s *Server) next(w http.ResponseWriter, r *http.Request) {
	if e := s.player.Next(r.Context()); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.playerState(w, r)
}
func (s *Server) previous(w http.ResponseWriter, r *http.Request) {
	if e := s.player.Previous(r.Context()); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.playerState(w, r)
}
func (s *Server) seek(w http.ResponseWriter, r *http.Request) {
	var b struct {
		PositionMS int64 `json:"position_ms"`
	}
	if e := decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	if e := s.player.Seek(r.Context(), b.PositionMS); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.playerState(w, r)
}
func (s *Server) volume(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Volume int `json:"volume"`
	}
	if e := decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	if e := s.player.Volume(r.Context(), b.Volume); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.playerState(w, r)
}
func (s *Server) shuffle(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Shuffle bool `json:"shuffle"`
	}
	if e := decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	if e := s.player.Shuffle(r.Context(), b.Shuffle); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.playerState(w, r)
}
func (s *Server) repeat(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Repeat string `json:"repeat"`
	}
	if e := decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	if e := s.player.Repeat(r.Context(), b.Repeat); e != nil {
		writeErr(w, 400, e)
		return
	}
	s.playerState(w, r)
}
func (s *Server) mute(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Muted bool `json:"muted"`
	}
	if e := decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	if e := s.player.Mute(r.Context(), b.Muted); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.playerState(w, r)
}

func (s *Server) songs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("page") || r.URL.Query().Has("page_size") {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if page < 1 {
			page = 1
		}
		if size < 1 {
			size = 50
		}
		if size > 200 {
			size = 200
		}
		x, e := s.store.SongsPageSorted(r.Context(), r.URL.Query().Get("q"), page, size, r.URL.Query().Get("sort"), r.URL.Query().Get("order") == "desc")
		if e != nil {
			writeErr(w, 500, e)
			return
		}
		writeJSON(w, 200, x)
		return
	}
	x, e := s.store.Songs(r.Context(), 0)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) song(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	x, e := s.store.Song(r.Context(), id)
	if e != nil {
		writeErr(w, 404, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) updateSong(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	var b struct {
		Title       string  `json:"title"`
		Favorite    *bool   `json:"favorite"`
		Artist      string  `json:"artist"`
		Album       string  `json:"album"`
		Year        *int    `json:"year"`
		TrackNumber *int    `json:"track_number"`
		DiscNumber  *int    `json:"disc_number"`
		Genre       *string `json:"genre"`
	}
	if e := decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	current, e := s.store.Song(r.Context(), id)
	if e != nil {
		writeErr(w, 404, e)
		return
	}
	metadataEdit := b.Title != "" || b.Artist != "" || b.Album != "" || b.Year != nil || b.TrackNumber != nil || b.DiscNumber != nil || b.Genre != nil
	if metadataEdit {
		var ok bool
		r, ok = s.authenticateAdmin(w, r)
		if !ok {
			return
		}
		if b.Title != "" {
			current.Title = b.Title
		}
		if b.Artist != "" {
			current.Artist = b.Artist
		}
		if b.Album != "" {
			current.Album = b.Album
		}
		if b.Year != nil {
			current.Year = *b.Year
		}
		if b.TrackNumber != nil {
			current.TrackNumber = *b.TrackNumber
		}
		if b.DiscNumber != nil {
			current.DiscNumber = *b.DiscNumber
		}
		if b.Genre != nil {
			current.Genre = *b.Genre
		}
		if e = media.RewriteTags(r.Context(), s.cfg, current, media.SongEdits{Title: current.Title, Artist: current.Artist, Album: current.Album, Year: current.Year, TrackNumber: current.TrackNumber, DiscNumber: current.DiscNumber, Genre: current.Genre}); e == nil {
			e = s.store.UpdateSongMetadata(r.Context(), id, current)
			if e == nil {
				e = s.store.LockSongMetadata(r.Context(), id)
			}
		}
	} else {
		e = s.store.UpdateSong(r.Context(), id, b.Title, b.Favorite)
	}
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	s.hub.Broadcast("library.changed", map[string]any{"song_id": id})
	s.song(w, r)
}
func (s *Server) deleteSong(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	song, e := s.store.Song(r.Context(), id)
	if e != nil {
		writeErr(w, 404, e)
		return
	}
	base, err := filepath.Abs(s.cfg.MusicPath)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	path, err := filepath.Abs(song.FilePath)
	if err != nil || !strings.HasPrefix(path, base+string(os.PathSeparator)) {
		writeErr(w, 409, fmt.Errorf("song file is outside the managed library"))
		return
	}
	trash := filepath.Join(s.cfg.BackupPath, "deleted")
	if e = os.MkdirAll(trash, 0750); e != nil {
		writeErr(w, 500, e)
		return
	}
	saved := filepath.Join(trash, fmt.Sprintf("song-%d-%d%s", id, time.Now().Unix(), filepath.Ext(path)))
	if e = os.Rename(path, saved); e != nil {
		writeErr(w, 500, e)
		return
	}
	if e = s.store.DeleteSong(r.Context(), id); e != nil {
		_ = os.Rename(saved, path)
		writeErr(w, 409, e)
		return
	}
	removeEmptyMusicParents(filepath.Dir(path), base)
	s.hub.Broadcast("library.changed", map[string]any{"deleted": id})
	w.WriteHeader(204)
}

func removeEmptyMusicParents(dir, root string) {
	dir = filepath.Clean(dir)
	root = filepath.Clean(root)
	for dir != root {
		rel, err := filepath.Rel(root, dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return
		}
		if err = os.Remove(dir); err != nil {
			return // Non-empty, missing, or not removable: preserve it and its parents.
		}
		dir = filepath.Dir(dir)
	}
}
func (s *Server) songArtwork(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
	f, h, e := r.FormFile("file")
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	defer f.Close()
	tmp, e := os.CreateTemp(s.cfg.ImportPath, ".art-*")
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, e = io.Copy(tmp, f); e != nil {
		tmp.Close()
		writeErr(w, 500, e)
		return
	}
	tmp.Close()
	dest := filepath.Join(s.cfg.ArtworkPath, fmt.Sprintf("song-%d.webp", id))
	cmd := exec.CommandContext(r.Context(), s.cfg.FFmpegBinary, "-v", "error", "-i", tmpPath, "-vf", "scale=1024:1024:force_original_aspect_ratio=decrease", "-frames:v", "1", "-y", dest)
	if out, e := cmd.CombinedOutput(); e != nil {
		writeErr(w, 400, fmt.Errorf("invalid artwork: %s", string(out)))
		return
	}
	url := fmt.Sprintf("/art/song-%d.webp", id)
	if e = s.store.UpdateSongArtwork(r.Context(), id, url); e != nil {
		writeErr(w, 500, e)
		return
	}
	s.hub.Broadcast("library.changed", map[string]any{"song_id": id})
	s.song(w, r)
	_ = h
}
func (s *Server) albums(w http.ResponseWriter, r *http.Request) {
	x, e := s.store.Albums(r.Context())
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) artists(w http.ResponseWriter, r *http.Request) {
	x, e := s.store.Artists(r.Context())
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) albumSongs(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	all, e := s.store.Songs(r.Context(), 0)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	out := make([]model.Song, 0)
	for _, x := range all {
		if x.AlbumID == id {
			out = append(out, x)
		}
	}
	writeJSON(w, 200, out)
}
func (s *Server) artistSongs(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	all, e := s.store.Songs(r.Context(), 0)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	out := make([]model.Song, 0)
	for _, x := range all {
		if x.ArtistID == id {
			out = append(out, x)
		}
	}
	writeJSON(w, 200, out)
}
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	x, e := s.store.Search(r.Context(), r.URL.Query().Get("q"))
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) playlists(w http.ResponseWriter, r *http.Request) {
	x, e := s.store.Playlists(r.Context())
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) playlistSongs(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	x, e := s.store.PlaylistSongs(r.Context(), id)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) createPlaylist(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
	}
	if e := decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		writeErr(w, 400, fmt.Errorf("name is required"))
		return
	}
	id, e := s.store.CreatePlaylist(r.Context(), b.Name)
	if e != nil {
		writeErr(w, 409, e)
		return
	}
	s.hub.Broadcast("playlist.changed", map[string]any{"playlist_id": id})
	writeJSON(w, 201, map[string]any{"id": id, "name": b.Name})
}
func (s *Server) deletePlaylist(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	if e = s.store.DeletePlaylist(r.Context(), id); e != nil {
		writeErr(w, 500, e)
		return
	}
	s.hub.Broadcast("playlist.changed", map[string]any{"playlist_id": id})
	w.WriteHeader(204)
}
func (s *Server) playPlaylist(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	var b struct {
		Shuffle bool `json:"shuffle"`
	}
	if e = decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	songs, e := s.store.PlaylistSongs(r.Context(), id)
	if e != nil {
		writeErr(w, 404, e)
		return
	}
	ids := make([]int64, len(songs))
	for i := range songs {
		ids[i] = songs[i].ID
	}
	if e = s.player.PlaySongs(r.Context(), ids, b.Shuffle); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.hub.Broadcast("queue.changed", map[string]any{"playlist_id": id, "shuffle": b.Shuffle})
	s.playerState(w, r)
}
func (s *Server) addPlaylistSong(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	var b struct {
		SongID  int64   `json:"song_id"`
		SongIDs []int64 `json:"song_ids"`
	}
	if e = decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	if b.SongID != 0 {
		b.SongIDs = append(b.SongIDs, b.SongID)
	}
	if len(b.SongIDs) == 0 {
		writeErr(w, 400, fmt.Errorf("at least one song is required"))
		return
	}
	if e = s.store.AddPlaylistSongs(r.Context(), id, b.SongIDs); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.hub.Broadcast("playlist.changed", map[string]any{"playlist_id": id})
	s.playlistSongs(w, r)
}
func (s *Server) reorderPlaylistSongs(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	var b struct {
		IDs []int64 `json:"ids"`
	}
	if e = decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	if e = s.store.ReorderPlaylist(r.Context(), id, b.IDs); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.hub.Broadcast("playlist.changed", map[string]any{"playlist_id": id})
	s.playlistSongs(w, r)
}
func (s *Server) removePlaylistSong(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	songID, e := strconv.ParseInt(r.PathValue("songID"), 10, 64)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	if e = s.store.RemovePlaylistSong(r.Context(), id, songID); e != nil {
		writeErr(w, 500, e)
		return
	}
	s.hub.Broadcast("playlist.changed", map[string]any{"playlist_id": id})
	s.playlistSongs(w, r)
}
func (s *Server) radio(w http.ResponseWriter, r *http.Request) {
	x, e := s.store.Radio(r.Context())
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) createRadio(w http.ResponseWriter, r *http.Request) {
	var x model.RadioStation
	if e := decode(r, &x); e != nil {
		writeErr(w, 400, e)
		return
	}
	id, e := s.store.SaveRadio(r.Context(), x)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	x, e = s.store.RadioByID(r.Context(), id)
	s.hub.Broadcast("radio.changed", x)
	writeJSON(w, 201, x)
}
func (s *Server) updateRadio(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	var x model.RadioStation
	if e = decode(r, &x); e != nil {
		writeErr(w, 400, e)
		return
	}
	x.ID = id
	if _, e = s.store.SaveRadio(r.Context(), x); e != nil {
		writeErr(w, 400, e)
		return
	}
	x, _ = s.store.RadioByID(r.Context(), id)
	s.hub.Broadcast("radio.changed", x)
	writeJSON(w, 200, x)
}
func (s *Server) deleteRadio(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	if e = s.store.DeleteRadio(r.Context(), id); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.hub.Broadcast("radio.changed", map[string]any{"radio_id": id})
	w.WriteHeader(204)
}
func (s *Server) playRadio(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	if e := s.player.PlayRadio(r.Context(), id); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.playerState(w, r)
}
func (s *Server) queue(w http.ResponseWriter, r *http.Request) {
	x, e := s.store.Queue(r.Context())
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) addQueue(w http.ResponseWriter, r *http.Request) {
	var b struct {
		SongID int64 `json:"song_id"`
	}
	if e := decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	if e := s.store.AddQueue(r.Context(), b.SongID); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.hub.Broadcast("queue.changed", map[string]any{"song_id": b.SongID})
	s.queue(w, r)
}
func (s *Server) clearQueue(w http.ResponseWriter, r *http.Request) {
	if e := s.store.ClearQueue(r.Context()); e != nil {
		writeErr(w, 500, e)
		return
	}
	s.hub.Broadcast("queue.changed", map[string]any{"cleared": true})
	writeJSON(w, 200, []any{})
}
func (s *Server) removeQueue(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	if e = s.store.RemoveQueue(r.Context(), id); e != nil {
		writeErr(w, 500, e)
		return
	}
	s.hub.Broadcast("queue.changed", map[string]any{"removed": id})
	s.queue(w, r)
}
func (s *Server) reorderQueue(w http.ResponseWriter, r *http.Request) {
	var b struct {
		IDs []int64 `json:"ids"`
	}
	if e := decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	if e := s.store.ReorderQueue(r.Context(), b.IDs); e != nil {
		writeErr(w, 409, e)
		return
	}
	s.hub.Broadcast("queue.changed", map[string]any{"reordered": true})
	s.queue(w, r)
}

func (s *Server) storage(w http.ResponseWriter, r *http.Request) {
	var st syscall.Statfs_t
	if e := syscall.Statfs(s.cfg.MusicPath, &st); e != nil {
		writeErr(w, 500, e)
		return
	}
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	var songs, albums, artists int
	_ = s.store.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM songs").Scan(&songs)
	_ = s.store.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM albums").Scan(&albums)
	_ = s.store.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM artists").Scan(&artists)
	writeJSON(w, 200, map[string]any{"total_bytes": total, "free_bytes": free, "used_bytes": total - free, "songs": songs, "albums": albums, "artists": artists})
}
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)
	mr, e := r.MultipartReader()
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	targetFolder := strings.TrimSpace(r.URL.Query().Get("folder"))
	if targetFolder != "" {
		if _, e = s.resolveMusicFolder(targetFolder, true); e != nil {
			writeErr(w, 400, e)
			return
		}
		targetFolder = filepath.ToSlash(filepath.Clean(targetFolder))
		if targetFolder == "." {
			targetFolder = ""
		}
	}
	jobs := []model.ImportJob{}
	for {
		part, partErr := mr.NextPart()
		if partErr == io.EOF {
			break
		}
		if partErr != nil {
			writeErr(w, 400, fmt.Errorf("invalid multipart upload: %w", partErr))
			return
		}
		if part.FormName() != "files" || part.FileName() == "" {
			_ = part.Close()
			continue
		}
		name, path, e := saveUploadStream(s.cfg.ImportPath, part.FileName(), part)
		_ = part.Close()
		if e != nil {
			writeErr(w, 500, e)
			return
		}
		id, e := s.store.CreateImportJob(r.Context(), name, path)
		if e != nil {
			_ = os.Remove(path)
			writeErr(w, 500, e)
			return
		}
		if e = s.store.SetImportTarget(r.Context(), id, targetFolder); e != nil {
			_ = os.Remove(path)
			writeErr(w, 500, e)
			return
		}
		if e = s.imports.Enqueue(id); e != nil {
			_ = s.store.UpdateImportJob(r.Context(), id, "failed", e.Error(), 0, 0)
		}
		j, _ := s.store.ImportJob(r.Context(), id)
		jobs = append(jobs, j)
	}
	if len(jobs) == 0 {
		writeErr(w, 400, fmt.Errorf("use multipart field 'files'"))
		return
	}
	s.hub.Broadcast("import.changed", jobs)
	writeJSON(w, 202, jobs)
}

func saveUploadStream(dir, filename string, src io.Reader) (string, string, error) {
	name := filepath.Base(filename)
	if name == "." || name == "" {
		return "", "", fmt.Errorf("invalid filename")
	}
	dstPath := filepath.Join(dir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), name))
	dst, e := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o640)
	if e != nil {
		return "", "", e
	}
	if _, e = io.Copy(dst, src); e != nil {
		_ = dst.Close()
		_ = os.Remove(dstPath)
		return "", "", e
	}
	if e = dst.Close(); e != nil {
		_ = os.Remove(dstPath)
		return "", "", e
	}
	return name, dstPath, nil
}
func saveUpload(dir string, fh *multipart.FileHeader) (string, string, error) {
	src, e := fh.Open()
	if e != nil {
		return "", "", e
	}
	defer src.Close()
	name := filepath.Base(fh.Filename)
	if name == "." || name == "" {
		return "", "", fmt.Errorf("invalid filename")
	}
	dstPath := filepath.Join(dir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), name))
	dst, e := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o640)
	if e != nil {
		return "", "", e
	}
	defer dst.Close()
	if _, e := io.Copy(dst, src); e != nil {
		_ = os.Remove(dstPath)
		return "", "", e
	}
	return name, dstPath, nil
}

func (s *Server) importJobs(w http.ResponseWriter, r *http.Request) {
	x, e := s.store.ImportJobs(r.Context())
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	x, e := s.store.LibraryStats(r.Context())
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) enrichmentStats(w http.ResponseWriter, r *http.Request) {
	if s.enricher == nil {
		writeErr(w, 503, fmt.Errorf("metadata enrichment is unavailable"))
		return
	}
	x, e := s.enricher.Stats(r.Context())
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) runEnrichment(w http.ResponseWriter, r *http.Request) {
	if s.enricher == nil {
		writeErr(w, 503, fmt.Errorf("metadata enrichment is unavailable"))
		return
	}
	go s.enricher.RunBatch(context.Background(), 25)
	writeJSON(w, 202, map[string]string{"status": "started"})
}
func (s *Server) retryEnrichment(w http.ResponseWriter, r *http.Request) {
	if s.enricher == nil {
		writeErr(w, 503, fmt.Errorf("metadata enrichment is unavailable"))
		return
	}
	if e := s.enricher.Retry(r.Context()); e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "queued"})
}
func (s *Server) backups(w http.ResponseWriter, r *http.Request) {
	x, e := backup.List(s.cfg.BackupPath)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	x, e := backup.Create(r.Context(), s.cfg, s.store)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 201, x)
}
func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	p, e := backup.Resolve(s.cfg.BackupPath, r.PathValue("name"))
	if e != nil {
		writeErr(w, 404, e)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(p)))
	http.ServeFile(w, r, p)
}
func (s *Server) scanImports(w http.ResponseWriter, r *http.Request) {
	jobs := make([]model.ImportJob, 0)
	err := filepath.WalkDir(s.cfg.ImportPath, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		id, e := s.store.CreateImportJob(r.Context(), d.Name(), path)
		if e != nil {
			return e
		}
		if e = s.imports.Enqueue(id); e != nil {
			_ = s.store.UpdateImportJob(r.Context(), id, "failed", e.Error(), 0, 0)
		}
		j, _ := s.store.ImportJob(r.Context(), id)
		jobs = append(jobs, j)
		return nil
	})
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 202, jobs)
}
func (s *Server) retryImport(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	j, e := s.store.ImportJob(r.Context(), id)
	if e != nil {
		writeErr(w, 404, e)
		return
	}
	if _, e = os.Stat(j.SourcePath); e != nil {
		writeErr(w, 409, fmt.Errorf("staged source is unavailable"))
		return
	}
	_ = s.store.UpdateImportJob(r.Context(), id, "queued", "", 0, 0)
	if e = s.imports.Enqueue(id); e != nil {
		writeErr(w, 503, e)
		return
	}
	j, _ = s.store.ImportJob(r.Context(), id)
	writeJSON(w, 202, j)
}
func (s *Server) cancelImport(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	j, e := s.store.ImportJob(r.Context(), id)
	if e != nil {
		writeErr(w, 404, e)
		return
	}
	if j.Status == "imported" || j.Status == "duplicate" {
		writeErr(w, 409, fmt.Errorf("completed jobs cannot be cancelled"))
		return
	}
	_ = s.store.UpdateImportJob(r.Context(), id, "cancelled", "Cancelled by administrator", 0, 0)
	j, _ = s.store.ImportJob(r.Context(), id)
	s.hub.Broadcast("import.changed", j)
	writeJSON(w, 200, j)
}
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	song, e := s.store.Song(r.Context(), id)
	if e != nil {
		writeErr(w, 404, e)
		return
	}
	if _, e := os.Stat(song.FilePath); e != nil {
		writeErr(w, 404, fmt.Errorf("audio file is not present on disk"))
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(song.FilePath)))
	http.ServeFile(w, r, song.FilePath)
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := fmt.Sprintf("%x", time.Now().UnixNano())
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r)
		slog.Info("request", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}
