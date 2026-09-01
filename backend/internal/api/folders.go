package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type folderInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func (s *Server) resolveMusicFolder(relative string, mustExist bool) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("folder path must be relative")
	}
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relative)))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("folder is outside the music library")
	}
	root, err := filepath.Abs(s.cfg.MusicPath)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("folder is outside the music library")
	}
	if mustExist {
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("not a folder")
		}
	}
	return path, nil
}

func (s *Server) folders(w http.ResponseWriter, r *http.Request) {
	root, _ := filepath.Abs(s.cfg.MusicPath)
	out := []folderInfo{{Path: "", Name: "Library root"}}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || path == root {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, folderInfo{Path: filepath.ToSlash(rel), Name: d.Name()})
		return nil
	})
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	writeJSON(w, 200, out)
}
func (s *Server) createFolder(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Path string `json:"path"`
	}
	if err := decode(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	path, err := s.resolveMusicFolder(b.Path, false)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	if filepath.Clean(path) == filepath.Clean(s.cfg.MusicPath) {
		writeErr(w, 409, fmt.Errorf("library root already exists"))
		return
	}
	if err = os.Mkdir(path, 0755); err != nil {
		writeErr(w, 409, err)
		return
	}
	s.folders(w, r)
}
func (s *Server) moveFolder(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Path   string `json:"path"`
		Target string `json:"target"`
	}
	if err := decode(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	oldPath, err := s.resolveMusicFolder(b.Path, true)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	newPath, err := s.resolveMusicFolder(b.Target, false)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	root, _ := filepath.Abs(s.cfg.MusicPath)
	if oldPath == root {
		writeErr(w, 409, fmt.Errorf("cannot move library root"))
		return
	}
	if strings.HasPrefix(newPath, oldPath+string(filepath.Separator)) {
		writeErr(w, 409, fmt.Errorf("cannot move a folder into itself"))
		return
	}
	if _, err = os.Stat(newPath); !os.IsNotExist(err) {
		writeErr(w, 409, fmt.Errorf("destination already exists"))
		return
	}
	if _, err = os.Stat(filepath.Dir(newPath)); err != nil {
		writeErr(w, 409, fmt.Errorf("destination parent does not exist"))
		return
	}
	if err = os.Rename(oldPath, newPath); err != nil {
		writeErr(w, 409, err)
		return
	}
	if err = s.store.MoveSongPathPrefix(r.Context(), oldPath, newPath); err != nil {
		_ = os.Rename(newPath, oldPath)
		writeErr(w, 500, err)
		return
	}
	s.hub.Broadcast("library.changed", map[string]any{"folder": b.Target})
	s.folders(w, r)
}
func (s *Server) deleteFolder(w http.ResponseWriter, r *http.Request) {
	path, err := s.resolveMusicFolder(r.URL.Query().Get("path"), true)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	root, _ := filepath.Abs(s.cfg.MusicPath)
	if path == root {
		writeErr(w, 409, fmt.Errorf("cannot delete library root"))
		return
	}
	all, err := s.store.Songs(r.Context(), 0)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	ids := make([]int64, 0)
	for _, song := range all {
		if strings.HasPrefix(filepath.Clean(song.FilePath), path+string(filepath.Separator)) {
			ids = append(ids, song.ID)
		}
	}
	trash := filepath.Join(s.cfg.BackupPath, "deleted-folders")
	if err = os.MkdirAll(trash, 0750); err != nil {
		writeErr(w, 500, err)
		return
	}
	saved := filepath.Join(trash, fmt.Sprintf("%s-%d", filepath.Base(path), time.Now().UnixNano()))
	if err = os.Rename(path, saved); err != nil {
		writeErr(w, 409, err)
		return
	}
	if err = s.store.DeleteSongs(r.Context(), ids); err != nil {
		_ = os.Rename(saved, path)
		writeErr(w, 500, err)
		return
	}
	s.hub.Broadcast("library.changed", map[string]any{"deleted_folder": r.URL.Query().Get("path"), "songs": len(ids)})
	w.WriteHeader(204)
}
func (s *Server) moveSongs(w http.ResponseWriter, r *http.Request) {
	var b struct {
		SongIDs []int64 `json:"song_ids"`
		Target  string  `json:"target"`
	}
	if err := decode(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	target, err := s.resolveMusicFolder(b.Target, true)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	moves := map[int64]string{}
	old := map[int64]string{}
	seen := map[string]bool{}
	for _, id := range b.SongIDs {
		song, e := s.store.Song(r.Context(), id)
		if e != nil {
			writeErr(w, 404, e)
			return
		}
		dest := filepath.Join(target, filepath.Base(song.FilePath))
		if seen[dest] {
			writeErr(w, 409, fmt.Errorf("duplicate destination filename"))
			return
		}
		seen[dest] = true
		if song.FilePath == dest {
			continue
		}
		if _, e = os.Stat(dest); !os.IsNotExist(e) {
			writeErr(w, 409, fmt.Errorf("%s already exists", filepath.Base(dest)))
			return
		}
		moves[id] = dest
		old[id] = song.FilePath
	}
	done := []int64{}
	for id, dest := range moves {
		if err = os.Rename(old[id], dest); err != nil {
			for _, moved := range done {
				_ = os.Rename(moves[moved], old[moved])
			}
			writeErr(w, 409, err)
			return
		}
		done = append(done, id)
	}
	if err = s.store.UpdateSongPaths(r.Context(), moves); err != nil {
		for _, id := range done {
			_ = os.Rename(moves[id], old[id])
		}
		writeErr(w, 500, err)
		return
	}
	s.hub.Broadcast("library.changed", map[string]any{"moved": len(moves)})
	writeJSON(w, 200, map[string]any{"moved": len(moves)})
}

func (s *Server) renameMusicFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err = decode(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		writeErr(w, 400, fmt.Errorf("invalid filename"))
		return
	}
	song, err := s.store.Song(r.Context(), id)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	oldPath := song.FilePath
	newPath := filepath.Join(filepath.Dir(oldPath), name)
	root, _ := filepath.Abs(s.cfg.MusicPath)
	absNew, err := filepath.Abs(newPath)
	if err != nil || !strings.HasPrefix(absNew, root+string(filepath.Separator)) {
		writeErr(w, 400, fmt.Errorf("file is outside the music library"))
		return
	}
	if oldPath == newPath {
		writeJSON(w, 200, song)
		return
	}
	if _, err = os.Stat(newPath); !os.IsNotExist(err) {
		writeErr(w, 409, fmt.Errorf("destination already exists"))
		return
	}
	if err = os.Rename(oldPath, newPath); err != nil {
		writeErr(w, 409, err)
		return
	}
	if err = s.store.UpdateSongPaths(r.Context(), map[int64]string{id: newPath}); err != nil {
		_ = os.Rename(newPath, oldPath)
		writeErr(w, 500, err)
		return
	}
	s.hub.Broadcast("library.changed", map[string]any{"renamed": id})
	song.FilePath = newPath
	writeJSON(w, 200, song)
}
