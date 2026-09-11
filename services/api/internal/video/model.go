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

type SourceType string

const (
	SourceTypeLocal  SourceType = "LOCAL"
	SourceTypeUpload SourceType = "UPLOAD"
)

type Video struct {
	ID              uuid.UUID  `json:"id"`
	Filename        string     `json:"filename"`
	ContentHash     string     `json:"content_hash"`
	MimeType        string     `json:"mime_type"`
	SizeBytes       int64      `json:"size_bytes"`
	SourceType      SourceType `json:"source_type"`
	SourcePath      *string    `json:"source_path"`
	StorageKey      *string    `json:"storage_key"`
	SourceMtime     *time.Time `json:"source_mtime"`
	ProcessingError *string    `json:"processing_error"`
	Status          Status     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
