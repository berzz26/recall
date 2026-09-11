package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env                 string
	Port                string
	Addr                string
	DatabaseURL         string
	StorageRoot         string
	MaxUploadSize       int64
	StabilitySeconds    int
	StabilityDuration   time.Duration
	PollInterval        time.Duration
	FFprobePath         string
	FFprobeTimeout      time.Duration
	SegmentDuration     time.Duration
	FrameSampleInterval time.Duration
	FFmpegPath          string
	FFmpegTimeout       time.Duration
	FrameJPEGQuality    int
	DetectionThreshold  float64
	DetectorName        string
	DetectorVersion     string
	ModelPath           string
	PythonPath          string
	EventMovementThreshold float64
	VisionModel         string
	VisionModelVersion  string
	VisionMaxFrames     int
	VisionTimeout       time.Duration
}

func Load() Config {
	_ = godotenv.Load()

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
	}

	storageRoot := os.Getenv("STORAGE_ROOT")
	if storageRoot == "" {
		storageRoot = "./storage"
	}

	maxUploadSize := int64(1 << 30)
	if v := os.Getenv("MAX_UPLOAD_SIZE"); v != "" {
		var parsed int64
		if _, err := fmt.Sscanf(v, "%d", &parsed); err == nil && parsed > 0 {
			maxUploadSize = parsed
		}
	}

	stabilitySeconds := 5
	if v := os.Getenv("LOCAL_FILE_STABILITY_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			stabilitySeconds = parsed
		}
	}

	pollInterval := 2 * time.Second
	if v := os.Getenv("PROCESSING_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			pollInterval = d
		}
	}

	ffprobePath := os.Getenv("FFPROBE_PATH")
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}

	ffprobeTimeout := 60 * time.Second
	if v := os.Getenv("FFPROBE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			ffprobeTimeout = d
		}
	}

	segmentDuration := 30 * time.Second
	if v := os.Getenv("VIDEO_SEGMENT_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			if d <= 0 {
				panic(fmt.Sprintf("VIDEO_SEGMENT_DURATION must be > 0, got %s", v))
			}
			segmentDuration = d
		} else {
			panic(fmt.Sprintf("invalid VIDEO_SEGMENT_DURATION %q: %v", v, err))
		}
	}

	frameSampleInterval := 2 * time.Second
	if v := os.Getenv("FRAME_SAMPLE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			if d <= 0 {
				panic(fmt.Sprintf("FRAME_SAMPLE_INTERVAL must be > 0, got %s", v))
			}
			frameSampleInterval = d
		} else {
			panic(fmt.Sprintf("invalid FRAME_SAMPLE_INTERVAL %q: %v", v, err))
		}
	}

	ffmpegPath := os.Getenv("FFMPEG_PATH")
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	ffmpegTimeout := 60 * time.Second
	if v := os.Getenv("FFMPEG_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			ffmpegTimeout = d
		}
	}

	frameJPEGQuality := 85
	if v := os.Getenv("FRAME_JPEG_QUALITY"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			if parsed < 1 || parsed > 100 {
				panic(fmt.Sprintf("FRAME_JPEG_QUALITY must be 1-100, got %s", v))
			}
			frameJPEGQuality = parsed
		} else {
			panic(fmt.Sprintf("invalid FRAME_JPEG_QUALITY %q: %v", v, err))
		}
	}

	detectionThreshold := 0.25
	if v := os.Getenv("DETECTION_CONFIDENCE_THRESHOLD"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			if parsed < 0 || parsed > 1 {
				panic(fmt.Sprintf("DETECTION_CONFIDENCE_THRESHOLD must be 0..1, got %s", v))
			}
			detectionThreshold = parsed
		} else {
			panic(fmt.Sprintf("invalid DETECTION_CONFIDENCE_THRESHOLD %q: %v", v, err))
		}
	}

	detectorName := os.Getenv("DETECTOR_NAME")
	if detectorName == "" {
		detectorName = "yolov8n"
	}
	detectorVersion := os.Getenv("DETECTOR_VERSION")
	if detectorVersion == "" {
		detectorVersion = "1"
	}
	modelPath := os.Getenv("MODEL_PATH")
	pythonPath := os.Getenv("PYTHON_PATH")
	if pythonPath == "" {
		pythonPath = "python3"
	}

	movementThreshold := 0.05
	if v := os.Getenv("EVENT_MOVEMENT_THRESHOLD"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			if parsed <= 0 || parsed > 1 {
				panic(fmt.Sprintf("EVENT_MOVEMENT_THRESHOLD must be 0..1, got %s", v))
			}
			movementThreshold = parsed
		} else {
			panic(fmt.Sprintf("invalid EVENT_MOVEMENT_THRESHOLD %q: %v", v, err))
		}
	}

	visionModel := os.Getenv("VISION_MODEL")
	if visionModel == "" {
		visionModel = "deterministic-local"
	}
	visionVersion := os.Getenv("VISION_MODEL_VERSION")
	if visionVersion == "" {
		visionVersion = "1"
	}
	visionMaxFrames := 6
	if v := os.Getenv("VISION_MAX_FRAMES_PER_SEGMENT"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 20 {
			visionMaxFrames = parsed
		} else {
			panic(fmt.Sprintf("invalid VISION_MAX_FRAMES_PER_SEGMENT %q", v))
		}
	}
	visionTimeout := 30 * time.Second
	if v := os.Getenv("VISION_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			visionTimeout = d
		} else {
			panic(fmt.Sprintf("invalid VISION_TIMEOUT %q", v))
		}
	}

	return Config{
		Env:               env,
		Port:              port,
		Addr:              ":" + port,
		DatabaseURL:       dbURL,
		StorageRoot:       storageRoot,
		MaxUploadSize:     maxUploadSize,
		StabilitySeconds:  stabilitySeconds,
		StabilityDuration: time.Duration(stabilitySeconds) * time.Second,
		PollInterval:        pollInterval,
		FFprobePath:         ffprobePath,
		FFprobeTimeout:      ffprobeTimeout,
		SegmentDuration:     segmentDuration,
		FrameSampleInterval: frameSampleInterval,
		FFmpegPath:          ffmpegPath,
		FFmpegTimeout:       ffmpegTimeout,
		FrameJPEGQuality:    frameJPEGQuality,
		DetectionThreshold:  detectionThreshold,
		DetectorName:        detectorName,
		DetectorVersion:     detectorVersion,
		ModelPath:           modelPath,
		PythonPath:          pythonPath,
		EventMovementThreshold: movementThreshold,
		VisionModel:         visionModel,
		VisionModelVersion:  visionVersion,
		VisionMaxFrames:     visionMaxFrames,
		VisionTimeout:       visionTimeout,
	}
}