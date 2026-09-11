package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/berzz26/recall/services/api/internal/video_media"
	"github.com/berzz26/recall/services/api/internal/video_segment"
)

type FFprobeProcessor struct {
	ffprobePath    string
	timeout        time.Duration
	storage        storage.Storage
	mediaService   *video_media.Service
	segmentService *video_segment.Service
}

func NewFFprobeProcessor(ffprobePath string, timeout time.Duration, store storage.Storage, mediaService *video_media.Service) *FFprobeProcessor {
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &FFprobeProcessor{
		ffprobePath:  ffprobePath,
		timeout:      timeout,
		storage:      store,
		mediaService: mediaService,
	}
}

func NewFFprobeProcessorWithSegments(ffprobePath string, timeout time.Duration, store storage.Storage, mediaService *video_media.Service, segmentService *video_segment.Service) *FFprobeProcessor {
	p := NewFFprobeProcessor(ffprobePath, timeout, store, mediaService)
	p.segmentService = segmentService
	return p
}

type probeResult struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

type probeStream struct {
	Index         int     `json:"index"`
	CodecType     string  `json:"codec_type"`
	CodecName     *string `json:"codec_name"`
	CodecLongName *string `json:"codec_long_name"`
	Width         *int    `json:"width"`
	Height        *int    `json:"height"`
	RFrameRate    *string `json:"r_frame_rate"`
	AvgFrameRate  *string `json:"avg_frame_rate"`
	BitRate       *string `json:"bit_rate"`
	SampleRate    *string `json:"sample_rate"`
	Channels      *int    `json:"channels"`
	ChannelLayout *string `json:"channel_layout"`
	Duration      *string `json:"duration"`
}

type probeFormat struct {
	FormatName     *string `json:"format_name"`
	FormatLongName *string `json:"format_long_name"`
	Duration       *string `json:"duration"`
	Size           *string `json:"size"`
	BitRate        *string `json:"bit_rate"`
	StartTime      *string `json:"start_time"`
}

func parseFps(s *string) (*float64, *string) {
	if s == nil || *s == "" || *s == "0/0" {
		return nil, s
	}
	raw := *s
	parts := strings.Split(raw, "/")
	if len(parts) == 2 {
		num, err1 := strconv.ParseFloat(parts[0], 64)
		den, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 == nil && err2 == nil && den != 0 {
			fps := num / den
			return &fps, &raw
		}
	}
	if v, err := strconv.ParseFloat(raw, 64); err == nil {
		return &v, &raw
	}
	return nil, &raw
}

func parseFloatPtr(s *string) *float64 {
	if s == nil || *s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(*s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseIntPtr(s *string) *int64 {
	if s == nil || *s == "" {
		return nil
	}
	v, err := strconv.ParseInt(*s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseInt(s *string) *int {
	if s == nil || *s == "" {
		return nil
	}
	v, err := strconv.Atoi(*s)
	if err != nil {
		return nil
	}
	return &v
}

func getExt(key string) string {
	idx := strings.LastIndex(key, ".")
	if idx == -1 {
		return ".mp4"
	}
	return key[idx:]
}

func (p *FFprobeProcessor) Process(ctx context.Context, v *video.Video) error {
	var videoPath string
	var tempFile string
	var cleanup func()

	if v.SourceType == video.SourceTypeLocal {
		if v.SourcePath == nil || *v.SourcePath == "" {
			return fmt.Errorf("missing source_path for LOCAL video")
		}
		videoPath = *v.SourcePath
		if _, err := os.Stat(videoPath); err != nil {
			return fmt.Errorf("source file not found: %w", err)
		}
	} else {
		if v.StorageKey == nil || *v.StorageKey == "" {
			return fmt.Errorf("missing storage_key for UPLOAD video")
		}
		rc, err := p.storage.Open(ctx, *v.StorageKey)
		if err != nil {
			return fmt.Errorf("failed to open storage: %w", err)
		}
		defer rc.Close()

		tmp, err := os.CreateTemp("", "ffprobe-*"+getExt(*v.StorageKey))
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tempFile = tmp.Name()
		cleanup = func() { os.Remove(tempFile) }
		if _, err := io.Copy(tmp, rc); err != nil {
			tmp.Close()
			cleanup()
			return fmt.Errorf("failed to copy to temp file: %w", err)
		}
		tmp.Close()
		videoPath = tempFile
	}
	if cleanup != nil {
		defer cleanup()
	}

	probeCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, p.ffprobePath,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		videoPath,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.Info("ffprobe start", "video_id", v.ID.String(), "path", videoPath)
	err := cmd.Run()
	if err != nil {
		if probeCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("ffprobe timeout")
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = msg[:500]
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ffprobe failed: %s", msg)
	}

	var result probeResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return fmt.Errorf("failed to parse ffprobe json: %w", err)
	}

	meta := &video_media.MediaMetadata{
		VideoID: v.ID,
	}

	if result.Format.FormatName != nil {
		meta.FormatName = result.Format.FormatName
	}
	if result.Format.FormatLongName != nil {
		meta.FormatLongName = result.Format.FormatLongName
	}
	meta.DurationSeconds = parseFloatPtr(result.Format.Duration)
	meta.SizeBytes = parseIntPtr(result.Format.Size)
	meta.BitRate = parseIntPtr(result.Format.BitRate)
	meta.StartTime = parseFloatPtr(result.Format.StartTime)

	var videoStream *probeStream
	var audioStream *probeStream
	for i := range result.Streams {
		s := &result.Streams[i]
		if s.CodecType == "video" && videoStream == nil {
			videoStream = s
		} else if s.CodecType == "audio" && audioStream == nil {
			audioStream = s
		}
	}

	if videoStream != nil {
		meta.VideoCodec = videoStream.CodecName
		meta.VideoWidth = videoStream.Width
		meta.VideoHeight = videoStream.Height
		if videoStream.RFrameRate != nil {
			fps, raw := parseFps(videoStream.RFrameRate)
			meta.VideoFps = fps
			meta.VideoFpsRaw = raw
		} else if videoStream.AvgFrameRate != nil {
			fps, raw := parseFps(videoStream.AvgFrameRate)
			meta.VideoFps = fps
			meta.VideoFpsRaw = raw
		}
	}

	if audioStream != nil {
		meta.AudioCodec = audioStream.CodecName
		meta.AudioSampleRate = parseInt(audioStream.SampleRate)
		meta.AudioChannels = audioStream.Channels
		meta.AudioChannelLayout = audioStream.ChannelLayout
	}

	if _, err := p.mediaService.Upsert(ctx, meta); err != nil {
		return fmt.Errorf("failed to persist media metadata: %w", err)
	}

	if p.segmentService != nil {
		if meta.DurationSeconds == nil || *meta.DurationSeconds <= 0 {
			return fmt.Errorf("video duration unavailable; cannot generate segments")
		}
		if _, err := p.segmentService.GenerateForVideo(ctx, v.ID, *meta.DurationSeconds); err != nil {
			return fmt.Errorf("failed to generate segments: %w", err)
		}
	}

	slog.Info("ffprobe success", "video_id", v.ID.String())
	return nil
}
