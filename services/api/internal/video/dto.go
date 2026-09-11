package video

type CreateVideoRequest struct {
	Filename    string `json:"filename" validate:"required"`
	ContentHash string `json:"content_hash" validate:"required"`
	MimeType    string `json:"mime_type" validate:"required"`
	SizeBytes   int64  `json:"size_bytes" validate:"required,gt=0"`
}

type VideoResponse struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentHash string `json:"content_hash"`
	MimeType    string `json:"mime_type"`
	SizeBytes   int64  `json:"size_bytes"`
	Status      Status `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
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
		Status:      v.Status,
		CreatedAt:   v.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   v.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
