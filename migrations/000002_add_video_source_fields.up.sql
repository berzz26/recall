CREATE TYPE video_source_type AS ENUM (
  'LOCAL',
  'UPLOAD'
);

ALTER TABLE videos ADD COLUMN source_type video_source_type NOT NULL DEFAULT 'UPLOAD';
ALTER TABLE videos ADD COLUMN source_path TEXT;
ALTER TABLE videos ADD COLUMN storage_key TEXT;

CREATE INDEX idx_videos_source_type ON videos(source_type);
