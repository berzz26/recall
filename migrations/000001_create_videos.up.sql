CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TYPE video_status AS ENUM (
  'UPLOADING',
  'UPLOADED',
  'PROCESSING',
  'READY',
  'FAILED'
);

CREATE TABLE videos (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  filename TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  status video_status NOT NULL DEFAULT 'UPLOADING',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_videos_status ON videos(status);
CREATE INDEX idx_videos_created_at ON videos(created_at);
