-- A service restart can leave work marked processing. Processing is not a durable
-- state: safely return it to the bounded retry queue.
UPDATE song_enrichment
SET status='retry', next_attempt_at=CURRENT_TIMESTAMP,
    message='Recovered interrupted enrichment', updated_at=CURRENT_TIMESTAMP
WHERE status='processing';
