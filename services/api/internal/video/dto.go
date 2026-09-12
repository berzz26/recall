package video

type CreateVideoRequest struct {
	Filename    string      `json:"filename"`
	ContentHash *string     `json:"content_hash"`
	MimeType    *string     `json:"mime_type"`
	SizeBytes   *int64      `json:"size_bytes"`
	SourceType  *SourceType `json:"source_type"`
	SourcePath  *string     `json:"source_path"`
}

type VideoResponse struct {
	ID          string     `json:"id"`
	Filename    string     `json:"filename"`
	ContentHash string     `json:"content_hash"`
	MimeType    string     `json:"mime_type"`
	SizeBytes   int64      `json:"size_bytes"`
	SourceType  SourceType `json:"source_type"`
	SourcePath  *string    `json:"source_path"`
	StorageKey  *string    `json:"storage_key"`
	Status      Status     `json:"status"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

type VideoListResponse struct {
	Videos []Video `json:"videos"`
}

func toResponse(v Video) VideoResponse {
	return VideoResponse{
		ID:          v.ID.String(),
		Filename:    v.Filename,
		ContentHash: v.ContentHash,
		MimeType:    v.MimeType,
		SizeBytes:   v.SizeBytes,
		SourceType:  v.SourceType,
		SourcePath:  v.SourcePath,
		StorageKey:  v.StorageKey,
		Status:      v.Status,
		CreatedAt:   v.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   v.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
