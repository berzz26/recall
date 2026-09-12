CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE video_segment_embeddings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    segment_id UUID NOT NULL REFERENCES video_segments(id) ON DELETE CASCADE,
    description_id UUID NOT NULL REFERENCES video_segment_descriptions(id) ON DELETE CASCADE,
    embedding vector(384) NOT NULL,
    model_name TEXT NOT NULL,
    model_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(description_id, model_name, model_version)
);

CREATE INDEX idx_video_segment_embeddings_video_id ON video_segment_embeddings(video_id);
CREATE INDEX idx_video_segment_embeddings_segment_id ON video_segment_embeddings(segment_id);
CREATE INDEX idx_video_segment_embeddings_description_id ON video_segment_embeddings(description_id);

CREATE INDEX video_segment_embeddings_hnsw_idx
ON video_segment_embeddings
USING hnsw (embedding vector_cosine_ops);
