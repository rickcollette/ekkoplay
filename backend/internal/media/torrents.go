package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"ekkoplayer/internal/config"
	"ekkoplayer/internal/db"
	"ekkoplayer/internal/model"
)

type TorrentManager struct {
	cfg       config.Config
	store     *db.Store
	imports   *Manager
	broadcast BroadcastFunc
	client    *http.Client
	sessionID string
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.Mutex
}

type transmissionFile struct {
	Name           string `json:"name"`
	Length         int64  `json:"length"`
	BytesCompleted int64  `json:"bytesCompleted"`
}
type transmissionTorrent struct {
	ID             int64              `json:"id"`
	HashString     string             `json:"hashString"`
	Name           string             `json:"name"`
	Status         int                `json:"status"`
	PercentDone    float64            `json:"percentDone"`
	RateDownload   int64              `json:"rateDownload"`
	RateUpload     int64              `json:"rateUpload"`
	DownloadedEver int64              `json:"downloadedEver"`
	UploadedEver   int64              `json:"uploadedEver"`
	TotalSize      int64              `json:"totalSize"`
	PeersConnected int                `json:"peersConnected"`
	ErrorString    string             `json:"errorString"`
	Files          []transmissionFile `json:"files"`
}

func NewTorrentManager(cfg config.Config, store *db.Store, imports *Manager, broadcast BroadcastFunc) *TorrentManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &TorrentManager{cfg: cfg, store: store, imports: imports, broadcast: broadcast, client: &http.Client{Timeout: 12 * time.Second}, cancel: cancel}
	m.startDaemon(ctx)
	m.wg.Add(1)
	go m.loop(ctx)
	return m
}

func (m *TorrentManager) startDaemon(_ context.Context) {
	if _, err := os.Stat(m.cfg.TorrentBinary); err != nil {
		slog.Warn("torrent client unavailable", "error", err)
		return
	}
	configDir := filepath.Join(m.cfg.TorrentPath, ".transmission")
	_ = os.MkdirAll(configDir, 0o750)
	m.cmd = exec.Command(m.cfg.TorrentBinary, "--foreground", "--config-dir", configDir, "--download-dir", m.cfg.TorrentPath, "--port", "9092", "--rpc-bind-address", "127.0.0.1", "--no-auth", "--peerport", strconv.Itoa(m.cfg.TorrentPeerPort))
	m.cmd.Stdout, m.cmd.Stderr = os.Stdout, os.Stderr
	if err := m.cmd.Start(); err != nil {
		slog.Warn("torrent client failed to start", "error", err)
		m.cmd = nil
	}
}

func (m *TorrentManager) Close() {
	m.cancel()
	m.wg.Wait()
	if m.cmd != nil && m.cmd.Process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		_ = m.rpc(ctx, "session-close", map[string]any{}, nil)
		cancel()
		done := make(chan error, 1)
		go func() { done <- m.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = m.cmd.Process.Kill()
			<-done
		}
	}
}

func (m *TorrentManager) loop(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.sync(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Debug("torrent sync", "error", err)
			}
		}
	}
}

func (m *TorrentManager) rpc(ctx context.Context, method string, args any, result any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, _ := json.Marshal(map[string]any{"method": method, "arguments": args})
	for attempt := 0; attempt < 2; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.TorrentRPCURL, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if m.sessionID != "" {
			req.Header.Set("X-Transmission-Session-Id", m.sessionID)
		}
		resp, err := m.client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusConflict {
			m.sessionID = resp.Header.Get("X-Transmission-Session-Id")
			resp.Body.Close()
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("torrent RPC: %s", resp.Status)
		}
		var envelope struct {
			Arguments json.RawMessage `json:"arguments"`
			Result    string          `json:"result"`
		}
		if err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&envelope); err != nil {
			return err
		}
		if envelope.Result != "success" {
			return fmt.Errorf("torrent RPC: %s", envelope.Result)
		}
		if result != nil {
			return json.Unmarshal(envelope.Arguments, result)
		}
		return nil
	}
	return fmt.Errorf("torrent RPC session negotiation failed")
}

func (m *TorrentManager) Add(ctx context.Context, torrent []byte, filename, target string) (model.TorrentJob, error) {
	dir := filepath.Join(m.cfg.TorrentPath, fmt.Sprintf("job-%d", time.Now().UnixNano()))
	res, err := m.store.DB.ExecContext(ctx, `INSERT INTO torrent_jobs(name,target_folder,download_dir) VALUES(?,?,?)`, filepath.Base(filename), target, dir)
	if err != nil {
		return model.TorrentJob{}, err
	}
	id, _ := res.LastInsertId()
	dir = filepath.Join(m.cfg.TorrentPath, fmt.Sprintf("job-%d", id))
	_, _ = m.store.DB.ExecContext(ctx, `UPDATE torrent_jobs SET download_dir=? WHERE id=?`, dir, id)
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return model.TorrentJob{}, err
	}
	var out struct {
		Added     transmissionTorrent `json:"torrent-added"`
		Duplicate transmissionTorrent `json:"torrent-duplicate"`
	}
	err = m.rpc(ctx, "torrent-add", map[string]any{"metainfo": base64.StdEncoding.EncodeToString(torrent), "download-dir": dir, "paused": false}, &out)
	if err != nil {
		_, _ = m.store.DB.ExecContext(ctx, `UPDATE torrent_jobs SET status='failed',error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, err.Error(), id)
		return model.TorrentJob{}, err
	}
	t := out.Added
	if t.HashString == "" {
		t = out.Duplicate
	}
	_, err = m.store.DB.ExecContext(ctx, `UPDATE torrent_jobs SET name=?,torrent_hash=?,status='downloading',updated_at=CURRENT_TIMESTAMP WHERE id=?`, t.Name, t.HashString, id)
	if err == nil {
		m.emit(id)
	}
	return m.Job(ctx, id)
}

func (m *TorrentManager) Jobs(ctx context.Context) ([]model.TorrentJob, error) {
	rows, err := m.store.DB.QueryContext(ctx, `SELECT id,name,target_folder,torrent_hash,status,percent,download_rate,upload_rate,downloaded_bytes,uploaded_bytes,total_bytes,peers,imported_count,total_audio,error,COALESCE(completed_at,''),COALESCE(seed_until,''),created_at,updated_at FROM torrent_jobs ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.TorrentJob, 0)
	for rows.Next() {
		var x model.TorrentJob
		if err = rows.Scan(&x.ID, &x.Name, &x.TargetFolder, &x.TorrentHash, &x.Status, &x.Percent, &x.DownloadRate, &x.UploadRate, &x.DownloadedBytes, &x.UploadedBytes, &x.TotalBytes, &x.Peers, &x.ImportedCount, &x.TotalAudio, &x.Error, &x.CompletedAt, &x.SeedUntil, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (m *TorrentManager) Job(ctx context.Context, id int64) (model.TorrentJob, error) {
	rows, err := m.Jobs(ctx)
	if err != nil {
		return model.TorrentJob{}, err
	}
	for _, x := range rows {
		if x.ID == id {
			return x, nil
		}
	}
	return model.TorrentJob{}, os.ErrNotExist
}

func (m *TorrentManager) sync(ctx context.Context) error {
	var result struct {
		Torrents []transmissionTorrent `json:"torrents"`
	}
	fields := []string{"id", "hashString", "name", "status", "percentDone", "rateDownload", "rateUpload", "downloadedEver", "uploadedEver", "totalSize", "peersConnected", "errorString", "files"}
	if err := m.rpc(ctx, "torrent-get", map[string]any{"fields": fields}, &result); err != nil {
		return err
	}
	byHash := map[string]transmissionTorrent{}
	for _, t := range result.Torrents {
		byHash[t.HashString] = t
	}
	jobs, err := m.Jobs(ctx)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.Status == "deleted" || j.Status == "failed" {
			continue
		}
		t, ok := byHash[j.TorrentHash]
		if !ok {
			continue
		}
		status := j.Status
		if t.ErrorString != "" {
			status = "failed"
		}
		_, _ = m.store.DB.ExecContext(ctx, `UPDATE torrent_jobs SET name=?,percent=?,download_rate=?,upload_rate=?,downloaded_bytes=?,uploaded_bytes=?,total_bytes=?,peers=?,error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, t.Name, t.PercentDone*100, t.RateDownload, t.RateUpload, t.DownloadedEver, t.UploadedEver, t.TotalSize, t.PeersConnected, t.ErrorString, j.ID)
		if t.PercentDone >= .9999 && (status == "adding" || status == "downloading") {
			if err := m.queueImports(ctx, j, t.Files); err != nil {
				_, _ = m.store.DB.ExecContext(ctx, `UPDATE torrent_jobs SET status='failed',error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, err.Error(), j.ID)
			} else {
				status = "importing"
			}
		}
		if status == "importing" {
			var total, done int
			_ = m.store.DB.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE status IN ('imported','duplicate','failed','cancelled')) FROM import_jobs WHERE torrent_job_id=?`, j.ID).Scan(&total, &done)
			_, _ = m.store.DB.ExecContext(ctx, `UPDATE torrent_jobs SET imported_count=?,total_audio=? WHERE id=?`, done, total, j.ID)
			if total > 0 && done == total {
				status = "seeding"
				_, _ = m.store.DB.ExecContext(ctx, `UPDATE torrent_jobs SET status='seeding',completed_at=COALESCE(completed_at,CURRENT_TIMESTAMP),seed_until=COALESCE(seed_until,datetime('now',?)),updated_at=CURRENT_TIMESTAMP WHERE id=?`, fmt.Sprintf("+%d days", m.cfg.TorrentSeedDays), j.ID)
			}
		}
		if status == "seeding" && j.SeedUntil != "" {
			var expired int
			_ = m.store.DB.QueryRowContext(ctx, `SELECT seed_until<=CURRENT_TIMESTAMP FROM torrent_jobs WHERE id=?`, j.ID).Scan(&expired)
			if expired == 1 {
				_, _ = m.store.DB.ExecContext(ctx, `UPDATE torrent_jobs SET status='expired',updated_at=CURRENT_TIMESTAMP WHERE id=?`, j.ID)
			}
		}
		m.emit(j.ID)
	}
	return nil
}

func audioFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp3", ".flac", ".m4a", ".aac", ".ogg", ".opus", ".wav", ".wma", ".aiff", ".alac":
		return true
	}
	return false
}

func (m *TorrentManager) queueImports(ctx context.Context, job model.TorrentJob, files []transmissionFile) error {
	var dir string
	if err := m.store.DB.QueryRowContext(ctx, `SELECT download_dir FROM torrent_jobs WHERE id=?`, job.ID).Scan(&dir); err != nil {
		return err
	}
	count := 0
	for i, f := range files {
		if !audioFile(f.Name) || f.BytesCompleted < f.Length {
			continue
		}
		source := filepath.Join(dir, filepath.FromSlash(f.Name))
		clean := filepath.Base(f.Name)
		staged := filepath.Join(m.cfg.ImportPath, fmt.Sprintf("torrent-%d-%d-%s", job.ID, i, clean))
		if err := copyTorrentFile(source, staged); err != nil {
			return err
		}
		id, err := m.store.CreateImportJob(ctx, clean, staged)
		if err != nil {
			_ = os.Remove(staged)
			return err
		}
		_, _ = m.store.DB.ExecContext(ctx, `UPDATE import_jobs SET target_folder=?,torrent_job_id=? WHERE id=?`, job.TargetFolder, job.ID, id)
		if err = m.imports.Enqueue(id); err != nil {
			return err
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("torrent contains no supported audio files")
	}
	_, _ = m.store.DB.ExecContext(ctx, `UPDATE torrent_jobs SET status='importing',total_audio=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, count, job.ID)
	return nil
}

func copyTorrentFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		_ = os.Remove(dst)
		return err
	}
	return closeErr
}

func (m *TorrentManager) Delete(ctx context.Context, id int64) error {
	j, err := m.Job(ctx, id)
	if err != nil {
		return err
	}
	if j.TorrentHash != "" {
		_ = m.rpc(ctx, "torrent-remove", map[string]any{"ids": []string{j.TorrentHash}, "delete-local-data": true}, nil)
	}
	var dir string
	_ = m.store.DB.QueryRowContext(ctx, `SELECT download_dir FROM torrent_jobs WHERE id=?`, id).Scan(&dir)
	if dir != "" && strings.HasPrefix(filepath.Clean(dir), filepath.Clean(m.cfg.TorrentPath)+string(filepath.Separator)) {
		_ = os.RemoveAll(dir)
	}
	_, err = m.store.DB.ExecContext(ctx, `UPDATE torrent_jobs SET status='deleted',download_rate=0,upload_rate=0,updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	m.emit(id)
	return err
}

func (m *TorrentManager) Extend(ctx context.Context, id int64) error {
	_, err := m.store.DB.ExecContext(ctx, `UPDATE torrent_jobs SET status='seeding',seed_until=datetime(CASE WHEN seed_until>CURRENT_TIMESTAMP THEN seed_until ELSE CURRENT_TIMESTAMP END,?),updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN ('seeding','expired')`, fmt.Sprintf("+%d days", m.cfg.TorrentSeedDays), id)
	m.emit(id)
	return err
}

func (m *TorrentManager) emit(id int64) {
	if m.broadcast == nil {
		return
	}
	if j, err := m.Job(context.Background(), id); err == nil {
		m.broadcast("torrent.changed", j)
	}
}
