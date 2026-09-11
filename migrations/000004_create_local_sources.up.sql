CREATE TABLE local_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_local_sources_path_unique ON local_sources(path);
CREATE UNIQUE INDEX idx_videos_local_source_path_unique ON videos(source_type, source_path) WHERE source_type = 'LOCAL' AND source_path IS NOT NULL;
