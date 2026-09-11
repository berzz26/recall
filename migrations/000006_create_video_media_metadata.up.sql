CREATE TABLE video_media_metadata (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id UUID NOT NULL UNIQUE REFERENCES videos(id) ON DELETE CASCADE,
    format_name TEXT,
    format_long_name TEXT,
    duration_seconds DOUBLE PRECISION,
    size_bytes BIGINT,
    bit_rate BIGINT,
    start_time DOUBLE PRECISION,
    video_codec TEXT,
    video_width INT,
    video_height INT,
    video_fps DOUBLE PRECISION,
    video_fps_raw TEXT,
    audio_codec TEXT,
    audio_sample_rate INT,
    audio_channels INT,
    audio_channel_layout TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_video_media_metadata_video_id ON video_media_metadata(video_id);
