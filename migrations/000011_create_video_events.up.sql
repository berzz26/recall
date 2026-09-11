CREATE TABLE video_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    track_id UUID REFERENCES video_tracks(id) ON DELETE CASCADE,
    segment_id UUID REFERENCES video_segments(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (event_type IN ('OBJECT_APPEARED','OBJECT_PRESENT','OBJECT_DISAPPEARED','OBJECT_MOVED')),
    label TEXT NOT NULL,
    start_timestamp DOUBLE PRECISION NOT NULL CHECK (start_timestamp >= 0),
    end_timestamp DOUBLE PRECISION CHECK (end_timestamp IS NULL OR end_timestamp >= start_timestamp),
    confidence DOUBLE PRECISION CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_video_events_video_id ON video_events(video_id);
CREATE INDEX idx_video_events_track_id ON video_events(track_id);
CREATE INDEX idx_video_events_segment_id ON video_events(segment_id);
CREATE INDEX idx_video_events_event_type ON video_events(event_type);
