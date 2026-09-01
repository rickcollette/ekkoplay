CREATE TABLE torrent_jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL DEFAULT '',
  target_folder TEXT NOT NULL,
  torrent_hash TEXT NOT NULL DEFAULT '',
  download_dir TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'adding',
  percent REAL NOT NULL DEFAULT 0,
  download_rate INTEGER NOT NULL DEFAULT 0,
  upload_rate INTEGER NOT NULL DEFAULT 0,
  downloaded_bytes INTEGER NOT NULL DEFAULT 0,
  uploaded_bytes INTEGER NOT NULL DEFAULT 0,
  total_bytes INTEGER NOT NULL DEFAULT 0,
  peers INTEGER NOT NULL DEFAULT 0,
  imported_count INTEGER NOT NULL DEFAULT 0,
  total_audio INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  completed_at TEXT,
  seed_until TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
ALTER TABLE import_jobs ADD COLUMN torrent_job_id INTEGER REFERENCES torrent_jobs(id) ON DELETE SET NULL;
CREATE INDEX idx_import_jobs_torrent ON import_jobs(torrent_job_id);
CREATE INDEX idx_torrent_jobs_hash ON torrent_jobs(torrent_hash);
