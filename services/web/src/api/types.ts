export type Status = 'UPLOADING' | 'UPLOADED' | 'PROCESSING' | 'READY' | 'FAILED'
export type SourceType = 'LOCAL' | 'UPLOAD'

export interface Video {
  id: string
  filename: string
  content_hash: string
  mime_type: string
  size_bytes: number
  source_type: SourceType
  source_path: string | null
  storage_key: string | null
  source_mtime: string | null
  processing_error: string | null
  status: Status
  created_at: string
  updated_at: string
}

export interface MediaMetadata {
  id: string
  video_id: string
  format_name: string | null
  format_long_name: string | null
  duration_seconds: number | null
  size_bytes: number | null
  bit_rate: number | null
  start_time: number | null
  video_codec: string | null
  video_width: number | null
  video_height: number | null
  video_fps: number | null
  video_fps_raw: string | null
  audio_codec: string | null
  audio_sample_rate: number | null
  audio_channels: number | null
  audio_channel_layout: string | null
}

export interface Segment {
  id: string
  video_id: string
  segment_index: number
  start_time: number
  end_time: number
  duration: number
}

export interface Frame {
  id: string
  video_id: string
  segment_id: string
  frame_index: number
  timestamp_seconds: number
  storage_key: string
  width: number
  height: number
}

export interface Detection {
  id: string
  video_id: string
  segment_id: string
  frame_id: string
  label: string
  confidence: number
  bbox_x: number
  bbox_y: number
  bbox_width: number
  bbox_height: number
  detector_name: string
  detector_version: string
}

export interface LocalSource {
  id: string
  name: string
  path: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface Health {
  status: string
  database: string
}

export interface Track {
  id: string
  video_id: string
  segment_id: string | null
  label: string
  track_index: number
  start_timestamp: number
  end_timestamp: number
  tracker_name: string
  tracker_version: string
  created_at: string
  updated_at: string
  detection_count: number
}

export interface TrackDetection {
  id: string
  track_id: string
  detection_id: string
  frame_id: string
  timestamp_seconds: number
  created_at: string
}
