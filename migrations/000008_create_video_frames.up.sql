CREATE TABLE video_frames (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    segment_id UUID NOT NULL REFERENCES video_segments(id) ON DELETE CASCADE,
    frame_index INTEGER NOT NULL CHECK (frame_index >= 0),
    timestamp_seconds DOUBLE PRECISION NOT NULL CHECK (timestamp_seconds >= 0),
    storage_key TEXT NOT NULL,
    width INTEGER NOT NULL CHECK (width > 0),
    height INTEGER NOT NULL CHECK (height > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(video_id, frame_index)
);

CREATE INDEX idx_video_frames_video_id ON video_frames(video_id);
CREATE INDEX idx_video_frames_segment_id ON video_frames(segment_id);
