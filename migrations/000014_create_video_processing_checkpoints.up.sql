CREATE TABLE video_processing_checkpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    segment_id UUID NOT NULL REFERENCES video_segments(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'PROCESSING', 'COMPLETE', 'FAILED')),
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    UNIQUE(video_id, segment_id)
);

CREATE INDEX idx_video_processing_checkpoints_video_id ON video_processing_checkpoints(video_id);
CREATE INDEX idx_video_processing_checkpoints_segment_id ON video_processing_checkpoints(segment_id);
