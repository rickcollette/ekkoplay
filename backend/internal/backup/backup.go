package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"ekkoplayer/internal/config"
	"ekkoplayer/internal/db"
	_ "modernc.org/sqlite"
)

type Info struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func Create(ctx context.Context, c config.Config, s *db.Store) (Info, error) {
	if err := os.MkdirAll(c.BackupPath, 0750); err != nil {
		return Info{}, err
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	tmpDB := filepath.Join(c.BackupPath, "."+stamp+".db")
	os.Remove(tmpDB)
	if _, err := s.DB.ExecContext(ctx, "VACUUM INTO ?", tmpDB); err != nil {
		return Info{}, err
	}
	defer os.Remove(tmpDB)
	name := "ekkoplayer-" + stamp + ".tar.gz"
	tmp, err := os.CreateTemp(c.BackupPath, ".backup-*.tmp")
	if err != nil {
		return Info{}, err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(tmpName)
		}
	}()
	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)
	if err = addFile(tw, tmpDB, "player.db"); err == nil {
		if p := os.Getenv("EKKOPLAYER_CONFIG"); p != "" {
			if _, e := os.Stat(p); e == nil {
				err = addFile(tw, p, "player.json")
			}
		}
	}
	if err == nil {
		manifest, _ := json.Marshal(map[string]any{"created_at": time.Now().UTC(), "version": 1})
		err = addBytes(tw, manifest, "manifest.json")
	}
	if e := tw.Close(); err == nil {
		err = e
	}
	if e := gz.Close(); err == nil {
		err = e
	}
	if e := tmp.Sync(); err == nil {
		err = e
	}
	if e := tmp.Close(); err == nil {
		err = e
	}
	if err != nil {
		return Info{}, err
	}
	dest := filepath.Join(c.BackupPath, name)
	if err = os.Rename(tmpName, dest); err != nil {
		return Info{}, err
	}
	ok = true
	_ = Prune(c.BackupPath, 7)
	st, _ := os.Stat(dest)
	return Info{Name: name, Size: st.Size(), CreatedAt: st.ModTime()}, nil
}
func addFile(tw *tar.Writer, path, name string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	if e = tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: st.Size(), ModTime: st.ModTime()}); e != nil {
		return e
	}
	_, e = io.Copy(tw, f)
	return e
}
func addBytes(tw *tar.Writer, b []byte, name string) error {
	if e := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(b)), ModTime: time.Now()}); e != nil {
		return e
	}
	_, e := tw.Write(b)
	return e
}
func List(dir string) ([]Info, error) {
	es, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return []Info{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := make([]Info, 0)
	for _, x := range es {
		if x.IsDir() || !strings.HasPrefix(x.Name(), "ekkoplayer-") || !strings.HasSuffix(x.Name(), ".tar.gz") {
			continue
		}
		st, e := x.Info()
		if e == nil {
			out = append(out, Info{Name: x.Name(), Size: st.Size(), CreatedAt: st.ModTime()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func Prune(dir string, keep int) error {
	x, e := List(dir)
	if e != nil {
		return e
	}
	if len(x) <= keep {
		return nil
	}
	for _, v := range x[keep:] {
		if e = os.Remove(filepath.Join(dir, v.Name)); e != nil {
			return e
		}
	}
	return nil
}
func Resolve(dir, name string) (string, error) {
	if filepath.Base(name) != name || !strings.HasPrefix(name, "ekkoplayer-") || !strings.HasSuffix(name, ".tar.gz") {
		return "", fmt.Errorf("invalid backup name")
	}
	p := filepath.Join(dir, name)
	if _, e := os.Stat(p); e != nil {
		return "", e
	}
	return p, nil
}
func Restore(archive, database string) error {
	var mode os.FileMode = 0600
	uid, gid := -1, -1
	if st, err := os.Stat(database); err == nil {
		mode = st.Mode()
		if raw, ok := st.Sys().(*syscall.Stat_t); ok {
			uid = int(raw.Uid)
			gid = int(raw.Gid)
		}
	}
	f, e := os.Open(archive)
	if e != nil {
		return e
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		return e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	tmp, e := os.CreateTemp(filepath.Dir(database), ".restore-*.db")
	if e != nil {
		return e
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	found := false
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if h.Name != "player.db" {
			continue
		}
		out, e := os.OpenFile(tmpPath, os.O_WRONLY|os.O_TRUNC, 0600)
		if e != nil {
			return e
		}
		_, e = io.Copy(out, tr)
		out.Close()
		if e != nil {
			return e
		}
		found = true
	}
	if !found {
		return fmt.Errorf("backup contains no database")
	}
	check, e := sql.Open("sqlite", tmpPath)
	if e != nil {
		return e
	}
	var result string
	e = check.QueryRow("PRAGMA integrity_check").Scan(&result)
	check.Close()
	if e != nil || result != "ok" {
		return fmt.Errorf("backup integrity check failed: %s", result)
	}
	if err := os.Remove(database + "-wal"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(database + "-shm"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	if uid >= 0 {
		if err := os.Chown(tmpPath, uid, gid); err != nil {
			return err
		}
	}
	return os.Rename(tmpPath, database)
}
