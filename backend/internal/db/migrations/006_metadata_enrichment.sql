CREATE TABLE IF NOT EXISTS song_enrichment (
  song_id INTEGER PRIMARY KEY REFERENCES songs(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0,
  match_score INTEGER NOT NULL DEFAULT 0,
  recording_mbid TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  next_attempt_at TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_enrichment_work ON song_enrichment(status,next_attempt_at);
