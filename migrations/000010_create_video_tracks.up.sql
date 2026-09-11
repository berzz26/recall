CREATE TABLE video_tracks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    segment_id UUID REFERENCES video_segments(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    track_index INTEGER NOT NULL,
    start_timestamp DOUBLE PRECISION NOT NULL,
    end_timestamp DOUBLE PRECISION NOT NULL,
    tracker_name TEXT NOT NULL DEFAULT 'bytetrack-iou',
    tracker_version TEXT NOT NULL DEFAULT '1',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(video_id, track_index),
    CHECK (track_index >= 0),
    CHECK (end_timestamp >= start_timestamp)
);

CREATE INDEX idx_video_tracks_video_id ON video_tracks(video_id);
CREATE INDEX idx_video_tracks_segment_id ON video_tracks(segment_id);

CREATE TABLE video_track_detections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    track_id UUID NOT NULL REFERENCES video_tracks(id) ON DELETE CASCADE,
    detection_id UUID NOT NULL REFERENCES video_frame_detections(id) ON DELETE CASCADE,
    frame_id UUID NOT NULL REFERENCES video_frames(id) ON DELETE CASCADE,
    timestamp_seconds DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(detection_id)
);

CREATE INDEX idx_video_track_detections_track_id ON video_track_detections(track_id);
CREATE INDEX idx_video_track_detections_detection_id ON video_track_detections(detection_id);
CREATE INDEX idx_video_track_detections_frame_id ON video_track_detections(frame_id);
