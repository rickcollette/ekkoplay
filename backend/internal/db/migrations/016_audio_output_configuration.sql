ALTER TABLE audio_output_overrides ADD COLUMN device TEXT NOT NULL DEFAULT '';
ALTER TABLE audio_output_overrides ADD COLUMN buffer_ms INTEGER NOT NULL DEFAULT 0 CHECK(buffer_ms BETWEEN 0 AND 5000);
ALTER TABLE audio_output_overrides ADD COLUMN channels TEXT NOT NULL DEFAULT '';
ALTER TABLE audio_output_overrides ADD COLUMN sample_rate INTEGER NOT NULL DEFAULT 0;
ALTER TABLE audio_output_overrides ADD COLUMN sample_format TEXT NOT NULL DEFAULT '';
ALTER TABLE audio_output_overrides ADD COLUMN exclusive INTEGER NOT NULL DEFAULT 0 CHECK(exclusive IN (0,1));
ALTER TABLE audio_output_overrides ADD COLUMN audio_filter TEXT NOT NULL DEFAULT '';
ALTER TABLE audio_output_overrides ADD COLUMN drift_correction_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE audio_output_overrides ADD COLUMN configured INTEGER NOT NULL DEFAULT 0 CHECK(configured IN (0,1));
