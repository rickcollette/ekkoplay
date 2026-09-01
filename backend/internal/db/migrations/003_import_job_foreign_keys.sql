PRAGMA foreign_keys = OFF;
ALTER TABLE import_jobs RENAME TO import_jobs_old;
CREATE TABLE import_jobs (
  id INTEGER PRIMARY KEY,
  filename TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'queued',
  message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  source_path TEXT NOT NULL DEFAULT '',
  song_id INTEGER REFERENCES songs(id) ON DELETE SET NULL,
  existing_song_id INTEGER REFERENCES songs(id) ON DELETE SET NULL
);
INSERT INTO import_jobs(id,filename,status,message,created_at,updated_at,source_path,song_id,existing_song_id)
SELECT id,filename,status,message,created_at,updated_at,source_path,song_id,existing_song_id FROM import_jobs_old;
DROP TABLE import_jobs_old;
CREATE INDEX idx_import_jobs_status ON import_jobs(status, created_at);
PRAGMA foreign_keys = ON;
