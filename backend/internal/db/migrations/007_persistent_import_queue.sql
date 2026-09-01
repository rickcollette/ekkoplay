UPDATE import_jobs
SET status='queued', message='', updated_at=CURRENT_TIMESTAMP
WHERE status='failed' AND message='import queue is full';

CREATE INDEX IF NOT EXISTS idx_import_jobs_status_id ON import_jobs(status,id);
