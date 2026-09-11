DROP INDEX IF EXISTS idx_videos_source_type;
ALTER TABLE videos DROP COLUMN IF EXISTS storage_key;
ALTER TABLE videos DROP COLUMN IF EXISTS source_path;
ALTER TABLE videos DROP COLUMN IF EXISTS source_type;
DROP TYPE IF EXISTS video_source_type;
