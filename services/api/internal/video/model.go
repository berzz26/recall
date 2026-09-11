package video

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusUploading  Status = "UPLOADING"
	StatusUploaded   Status = "UPLOADED"
	StatusProcessing Status = "PROCESSING"
	StatusReady      Status = "READY"
	StatusFailed     Status = "FAILED"
)

type Video struct {
	ID          uuid.UUID `json:"id"`
	Filename    string    `json:"filename"`
	ContentHash string    `json:"content_hash"`
	MimeType    string    `json:"mime_type"`
	SizeBytes   int64     `json:"size_bytes"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
