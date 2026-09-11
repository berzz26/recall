CREATE TABLE video_segment_descriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    segment_id UUID NOT NULL REFERENCES video_segments(id) ON DELETE CASCADE,
    description TEXT NOT NULL CHECK (char_length(description) > 0 AND char_length(description) <= 2000),
    model_name TEXT NOT NULL,
    model_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(segment_id, model_name, model_version)
);

CREATE INDEX idx_video_segment_descriptions_video_id ON video_segment_descriptions(video_id);
CREATE INDEX idx_video_segment_descriptions_segment_id ON video_segment_descriptions(segment_id);
