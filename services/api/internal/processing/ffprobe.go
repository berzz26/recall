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

	"github.com/berzz26/recall/services/api/internal/segment_description"
	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/berzz26/recall/services/api/internal/video_event"
	"github.com/berzz26/recall/services/api/internal/video_frame"
	"github.com/berzz26/recall/services/api/internal/video_media"
	"github.com/berzz26/recall/services/api/internal/video_segment"
	"github.com/berzz26/recall/services/api/internal/video_track"
	"github.com/berzz26/recall/services/api/internal/visual"
)

type FFprobeProcessor struct {
	ffprobePath    string
	timeout        time.Duration
	storage        storage.Storage
	mediaService   *video_media.Service
	segmentService *video_segment.Service
	frameService   *video_frame.Service
	visualService  *visual.Service
	trackService   *video_track.Service
	eventService   *video_event.Service
	descService    *segment_description.Service
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

func NewFFprobeProcessorWithFrames(ffprobePath string, timeout time.Duration, store storage.Storage, mediaService *video_media.Service, segmentService *video_segment.Service, frameService *video_frame.Service) *FFprobeProcessor {
	p := NewFFprobeProcessor(ffprobePath, timeout, store, mediaService)
	p.segmentService = segmentService
	p.frameService = frameService
	return p
}

func NewFFprobeProcessorWithVisual(ffprobePath string, timeout time.Duration, store storage.Storage, mediaService *video_media.Service, segmentService *video_segment.Service, frameService *video_frame.Service, visualService *visual.Service) *FFprobeProcessor {
	p := NewFFprobeProcessor(ffprobePath, timeout, store, mediaService)
	p.segmentService = segmentService
	p.frameService = frameService
	p.visualService = visualService
	return p
}

func NewFFprobeProcessorWithTracking(ffprobePath string, timeout time.Duration, store storage.Storage, mediaService *video_media.Service, segmentService *video_segment.Service, frameService *video_frame.Service, visualService *visual.Service, trackService *video_track.Service) *FFprobeProcessor {
	p := NewFFprobeProcessor(ffprobePath, timeout, store, mediaService)
	p.segmentService = segmentService
	p.frameService = frameService
	p.visualService = visualService
	p.trackService = trackService
	return p
}

func NewFFprobeProcessorWithEvents(ffprobePath string, timeout time.Duration, store storage.Storage, mediaService *video_media.Service, segmentService *video_segment.Service, frameService *video_frame.Service, visualService *visual.Service, trackService *video_track.Service, eventService *video_event.Service) *FFprobeProcessor {
	p := NewFFprobeProcessor(ffprobePath, timeout, store, mediaService)
	p.segmentService = segmentService
	p.frameService = frameService
	p.visualService = visualService
	p.trackService = trackService
	p.eventService = eventService
	return p
}

func NewFFprobeProcessorWithDescriptions(ffprobePath string, timeout time.Duration, store storage.Storage, mediaService *video_media.Service, segmentService *video_segment.Service, frameService *video_frame.Service, visualService *visual.Service, trackService *video_track.Service, eventService *video_event.Service, descService *segment_description.Service) *FFprobeProcessor {
	p := NewFFprobeProcessor(ffprobePath, timeout, store, mediaService)
	p.segmentService = segmentService
	p.frameService = frameService
	p.visualService = visualService
	p.trackService = trackService
	p.eventService = eventService
	p.descService = descService
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
	pipelineStart := time.Now()
	slog.Info("pipeline: start", "video_id", v.ID.String(), "source_type", v.SourceType)
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

	ffprobeStart := time.Now()
	slog.Info("ffprobe: start", "video_id", v.ID.String(), "path", videoPath)
	err := cmd.Run()
	ffprobeMs := time.Since(ffprobeStart).Milliseconds()
	if err != nil {
		if probeCtx.Err() == context.DeadlineExceeded {
			slog.Error("ffprobe: timeout", "video_id", v.ID.String(), "duration_ms", ffprobeMs, "error", probeCtx.Err())
			return fmt.Errorf("ffprobe timeout")
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = msg[:500]
		}
		if msg == "" {
			msg = err.Error()
		}
		slog.Error("ffprobe: failed", "video_id", v.ID.String(), "duration_ms", ffprobeMs, "error", msg)
		return fmt.Errorf("ffprobe failed: %s", msg)
	}
	slog.Info("ffprobe: complete", "video_id", v.ID.String(), "duration_ms", ffprobeMs, "output_bytes", stdout.Len())

	parseStart := time.Now()
	var result probeResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return fmt.Errorf("failed to parse ffprobe json: %w", err)
	}
	parseMs := time.Since(parseStart).Milliseconds()
	slog.Debug("ffprobe: parsed", "video_id", v.ID.String(), "duration_ms", parseMs)

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

	mediaStart := time.Now()
	if _, err := p.mediaService.Upsert(ctx, meta); err != nil {
		return fmt.Errorf("failed to persist media metadata: %w", err)
	}
	mediaMs := time.Since(mediaStart).Milliseconds()
	slog.Info("pipeline: media persisted", "video_id", v.ID.String(), "duration_ms", mediaMs)

	var segments []video_segment.VideoSegment
	if p.segmentService != nil {
		if meta.DurationSeconds == nil || *meta.DurationSeconds <= 0 {
			return fmt.Errorf("video duration unavailable; cannot generate segments")
		}
		segStart := time.Now()
		var err error
		segments, err = p.segmentService.GenerateForVideo(ctx, v.ID, *meta.DurationSeconds)
		segMs := time.Since(segStart).Milliseconds()
		if err != nil {
			slog.Error("pipeline: segment generation failed", "video_id", v.ID.String(), "duration_ms", segMs, "error", err)
			return fmt.Errorf("failed to generate segments: %w", err)
		}
		slog.Info("pipeline: segments generated", "video_id", v.ID.String(), "segments", len(segments), "duration_ms", segMs, "duration_seconds", *meta.DurationSeconds)
	} else if p.frameService != nil {
		return fmt.Errorf("frame extraction requires segments")
	}

	if p.frameService != nil {
		if meta.DurationSeconds == nil || *meta.DurationSeconds <= 0 {
			return fmt.Errorf("video duration unavailable; cannot extract frames")
		}
		w := 0
		h := 0
		if meta.VideoWidth != nil {
			w = *meta.VideoWidth
		}
		if meta.VideoHeight != nil {
			h = *meta.VideoHeight
		}
		if w <= 0 || h <= 0 {
			w = 1280
			h = 720
		}
		frameStart := time.Now()
		if _, err := p.frameService.GenerateForVideo(ctx, v, segments, *meta.DurationSeconds, w, h); err != nil {
			slog.Error("pipeline: frame extraction failed", "video_id", v.ID.String(), "duration_ms", time.Since(frameStart).Milliseconds(), "error", err)
			return fmt.Errorf("failed to extract frames: %w", err)
		}
		frameMs := time.Since(frameStart).Milliseconds()
		slog.Info("pipeline: frame extraction complete", "video_id", v.ID.String(), "duration_ms", frameMs)
	}

	if p.visualService != nil {
		visualStart := time.Now()
		slog.Info("pipeline: detection start", "video_id", v.ID.String())
		if _, err := p.visualService.AnalyzeVideo(ctx, v.ID); err != nil {
			slog.Error("pipeline: detection failed", "video_id", v.ID.String(), "duration_ms", time.Since(visualStart).Milliseconds(), "error", err)
			return fmt.Errorf("failed to analyze visuals: %w", err)
		}
		visualMs := time.Since(visualStart).Milliseconds()
		slog.Info("pipeline: detection complete", "video_id", v.ID.String(), "duration_ms", visualMs)
	}

	if p.trackService != nil {
		trackStart := time.Now()
		slog.Info("pipeline: tracking start", "video_id", v.ID.String())
		if _, err := p.trackService.GenerateForVideoID(ctx, v.ID); err != nil {
			slog.Error("pipeline: tracking failed", "video_id", v.ID.String(), "duration_ms", time.Since(trackStart).Milliseconds(), "error", err)
			return fmt.Errorf("failed to generate tracks: %w", err)
		}
		trackMs := time.Since(trackStart).Milliseconds()
		slog.Info("pipeline: tracking complete", "video_id", v.ID.String(), "duration_ms", trackMs)
	}

	if p.eventService != nil {
		eventStart := time.Now()
		slog.Info("pipeline: event generation start", "video_id", v.ID.String())
		if _, err := p.eventService.GenerateForVideo(ctx, v.ID); err != nil {
			slog.Error("pipeline: event generation failed", "video_id", v.ID.String(), "duration_ms", time.Since(eventStart).Milliseconds(), "error", err)
			return fmt.Errorf("failed to generate events: %w", err)
		}
		eventMs := time.Since(eventStart).Milliseconds()
		slog.Info("pipeline: events complete", "video_id", v.ID.String(), "duration_ms", eventMs)
	}

	if p.descService != nil {
		vlmStart := time.Now()
		slog.Info("pipeline: VLM generation start", "video_id", v.ID.String())
		if _, err := p.descService.GenerateForVideo(ctx, v.ID); err != nil {
			slog.Error("pipeline: VLM generation failed", "video_id", v.ID.String(), "duration_ms", time.Since(vlmStart).Milliseconds(), "error", err)
			return fmt.Errorf("failed to generate descriptions: %w", err)
		}
		vlmMs := time.Since(vlmStart).Milliseconds()
		slog.Info("pipeline: VLM generation complete", "video_id", v.ID.String(), "duration_ms", vlmMs)
	} else {
		slog.Info("pipeline: VLM generation skipped (disabled)", "video_id", v.ID.String())
	}

	totalMs := time.Since(pipelineStart).Milliseconds()
	slog.Info("pipeline: success", "video_id", v.ID.String(), "total_duration_ms", totalMs)
	return nil
}
