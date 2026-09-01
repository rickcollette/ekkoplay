package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) torrentJobs(w http.ResponseWriter, r *http.Request) {
	if s.torrents == nil {
		writeErr(w, 503, fmt.Errorf("torrent service is unavailable"))
		return
	}
	jobs, err := s.torrents.Jobs(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, jobs)
}

func (s *Server) addTorrent(w http.ResponseWriter, r *http.Request) {
	if s.torrents == nil {
		writeErr(w, 503, fmt.Errorf("torrent service is unavailable"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeErr(w, 400, err)
		return
	}
	target := strings.TrimSpace(r.FormValue("folder"))
	if target == "" {
		writeErr(w, 400, fmt.Errorf("a destination folder is required"))
		return
	}
	destination, err := s.resolveMusicFolder(target, false)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	if err = os.MkdirAll(destination, 0o755); err != nil {
		writeErr(w, 500, fmt.Errorf("create destination folder: %w", err))
		return
	}
	target = filepath.ToSlash(filepath.Clean(target))
	file, header, err := r.FormFile("torrent")
	if err != nil {
		writeErr(w, 400, fmt.Errorf("torrent file is required"))
		return
	}
	defer file.Close()
	if !strings.EqualFold(filepath.Ext(header.Filename), ".torrent") {
		writeErr(w, 400, fmt.Errorf("file must have a .torrent extension"))
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, 16<<20))
	if err != nil || len(data) == 0 {
		writeErr(w, 400, fmt.Errorf("invalid torrent file"))
		return
	}
	job, err := s.torrents.Add(r.Context(), data, header.Filename, target)
	if err != nil {
		writeErr(w, 503, err)
		return
	}
	writeJSON(w, 202, job)
}

func (s *Server) deleteTorrent(w http.ResponseWriter, r *http.Request) {
	if s.torrents == nil {
		writeErr(w, 503, fmt.Errorf("torrent service is unavailable"))
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	if err = s.torrents.Delete(r.Context(), id); err != nil {
		writeErr(w, 500, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) extendTorrent(w http.ResponseWriter, r *http.Request) {
	if s.torrents == nil {
		writeErr(w, 503, fmt.Errorf("torrent service is unavailable"))
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	if err = s.torrents.Extend(r.Context(), id); err != nil {
		writeErr(w, 500, err)
		return
	}
	job, err := s.torrents.Job(r.Context(), id)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	writeJSON(w, 200, job)
}
