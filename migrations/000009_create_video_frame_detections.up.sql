CREATE TABLE video_frame_detections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    segment_id UUID NOT NULL REFERENCES video_segments(id) ON DELETE CASCADE,
    frame_id UUID NOT NULL REFERENCES video_frames(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    bbox_x DOUBLE PRECISION NOT NULL CHECK (bbox_x >= 0 AND bbox_x <= 1),
    bbox_y DOUBLE PRECISION NOT NULL CHECK (bbox_y >= 0 AND bbox_y <= 1),
    bbox_width DOUBLE PRECISION NOT NULL CHECK (bbox_width > 0 AND bbox_width <= 1),
    bbox_height DOUBLE PRECISION NOT NULL CHECK (bbox_height > 0 AND bbox_height <= 1),
    detector_name TEXT NOT NULL,
    detector_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (bbox_x + bbox_width <= 1.000001),
    CHECK (bbox_y + bbox_height <= 1.000001)
);

CREATE INDEX idx_video_frame_detections_video_id ON video_frame_detections(video_id);
CREATE INDEX idx_video_frame_detections_segment_id ON video_frame_detections(segment_id);
CREATE INDEX idx_video_frame_detections_frame_id ON video_frame_detections(frame_id);
