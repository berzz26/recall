import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { client } from '../api/client'
import type { Video, MediaMetadata, Segment, Frame, Detection } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'
import { Loading, ErrorState } from '../components/Loading'

export default function VideoDetail() {
  const { id } = useParams()
  const [video, setVideo] = useState<Video | null>(null)
  const [media, setMedia] = useState<MediaMetadata | null>(null)
  const [segments, setSegments] = useState<Segment[]>([])
  const [frames, setFrames] = useState<Frame[]>([])
  const [detections, setDetections] = useState<Detection[]>([])
  const [err, setErr] = useState<string | null>(null)
  const [selectedFrame, setSelectedFrame] = useState<Frame | null>(null)

  const fetchAll = async () => {
    if (!id) return
    try {
      const v = await client.get<Video>(`/api/v1/videos/${id}`)
      setVideo(v)
      try { setMedia(await client.get<MediaMetadata>(`/api/v1/videos/${id}/media`)) } catch { setMedia(null) }
      try { setSegments(await client.get<Segment[]>(`/api/v1/videos/${id}/segments`)) } catch { setSegments([]) }
      try { setFrames(await client.get<Frame[]>(`/api/v1/videos/${id}/frames`)) } catch { setFrames([]) }
      try { setDetections(await client.get<Detection[]>(`/api/v1/videos/${id}/detections`)) } catch { setDetections([]) }
      setErr(null)
    } catch (e: any) { setErr(e.message) }
  }

  useEffect(() => { fetchAll() }, [id])

  // polling while processing
  useEffect(() => {
    if (!video || (video.status !== 'UPLOADED' && video.status !== 'PROCESSING')) return
    const t = setInterval(fetchAll, 3000)
    return () => clearInterval(t)
  }, [video?.status])

  if (err) return <ErrorState error={err} retry={fetchAll} />
  if (!video) return <Loading />

  const detectionsByFrame = detections.reduce<Record<string, Detection[]>>((acc, d) => {
    (acc[d.frame_id] = acc[d.frame_id] || []).push(d)
    return acc
  }, {})
  const summary = detections.reduce<Record<string, number>>((acc, d) => {
    acc[d.label] = (acc[d.label] || 0) + 1
    return acc
  }, {})
  const frameCountBySeg = frames.reduce<Record<string, number>>((a, f) => { a[f.segment_id] = (a[f.segment_id] || 0) + 1; return a }, {})
  const detCountBySeg = detections.reduce<Record<string, number>>((a, d) => { a[d.segment_id] = (a[d.segment_id] || 0) + 1; return a }, {})

  const imgSrc = (f: Frame) => client.imageUrl(`/api/v1/videos/${video.id}/frames/${f.id}/image`)

  return (
    <div>
      <div className="card">
        <h3>{video.filename} <StatusBadge status={video.status} /></h3>
        <dl className="kv">
          <dt>Video ID</dt><dd>{video.id}</dd>
          <dt>Source type</dt><dd>{video.source_type}</dd>
          <dt>Source path</dt><dd>{video.source_path || '-'}</dd>
          <dt>Storage key</dt><dd>{video.storage_key || '-'}</dd>
          <dt>Size</dt><dd>{(video.size_bytes / 1024 / 1024).toFixed(2)} MB</dd>
          <dt>MIME</dt><dd>{video.mime_type}</dd>
          <dt>Content hash</dt><dd style={{ wordBreak: 'break-all' }}>{video.content_hash || '-'}</dd>
          <dt>Created</dt><dd>{new Date(video.created_at).toLocaleString()}</dd>
          <dt>Updated</dt><dd>{new Date(video.updated_at).toLocaleString()}</dd>
          {video.processing_error && <><dt>Processing error</dt><dd style={{ color: '#f87171' }}>{video.processing_error}</dd></>}
        </dl>
      </div>

      {media && (
        <div className="card">
          <h3>Media Metadata</h3>
          <dl className="kv">
            <dt>Format</dt><dd>{media.format_name || '-'} {media.format_long_name ? `(${media.format_long_name})` : ''}</dd>
            <dt>Duration</dt><dd>{media.duration_seconds?.toFixed(2) + 's' || '-'}</dd>
            <dt>Bitrate</dt><dd>{media.bit_rate || '-'}</dd>
            <dt>Video codec</dt><dd>{media.video_codec || '-'}</dd>
            <dt>Resolution</dt><dd>{media.video_width && media.video_height ? `${media.video_width}×${media.video_height}` : '-'}</dd>
            <dt>FPS</dt><dd>{media.video_fps?.toFixed(2) || '-'} {media.video_fps_raw ? `(${media.video_fps_raw})` : ''}</dd>
            <dt>Audio codec</dt><dd>{media.audio_codec || 'no audio'}</dd>
            <dt>Sample rate</dt><dd>{media.audio_sample_rate || '-'}</dd>
            <dt>Channels</dt><dd>{media.audio_channels ?? '-'}</dd>
            <dt>Channel layout</dt><dd>{media.audio_channel_layout || '-'}</dd>
          </dl>
        </div>
      )}

      <div className="card">
        <h3>Timeline / Segments ({segments.length})</h3>
        {segments.length > 0 && (
          <div className="timeline">
            {segments.map(s => <div key={s.id} className="seg" style={{ flex: s.duration }}>S{s.segment_index}</div>)}
          </div>
        )}
        <table className="table">
          <thead><tr><th>Segment</th><th>Start</th><th>End</th><th>Duration</th><th>Frames</th><th>Detections</th></tr></thead>
          <tbody>
            {segments.map(s => (
              <tr key={s.id}><td>{s.segment_index}</td><td>{s.start_time.toFixed(2)}s</td><td>{s.end_time.toFixed(2)}s</td><td>{s.duration.toFixed(2)}s</td><td>{frameCountBySeg[s.id] || 0}</td><td>{detCountBySeg[s.id] || 0}</td></tr>
            ))}
          </tbody>
        </table>
        {segments.length === 0 && <div className="empty">No segments</div>}
      </div>

      <div className="card">
        <h3>Detection Summary</h3>
        {Object.keys(summary).length === 0 ? <div className="empty">No detections</div> : (
          <div className="summary">
            {Object.entries(summary).map(([label, count]) => <span key={label} className="item">{label} <b>{count}</b></span>)}
          </div>
        )}
      </div>

      <div className="card">
        <h3>Frames ({frames.length})</h3>
        <div className="frame-grid">
          {frames.map(f => {
            const dets = detectionsByFrame[f.id] || []
            return (
              <div key={f.id} className="frame-card" onClick={() => setSelectedFrame(f)} style={{ cursor: 'pointer' }}>
                <div className="img-wrap">
                  <img src={imgSrc(f)} alt={`frame ${f.frame_index}`} loading="lazy" />
                  {dets.map(d => (
                    <div key={d.id} className="bbox" style={{
                      left: `${d.bbox_x * 100}%`,
                      top: `${d.bbox_y * 100}%`,
                      width: `${d.bbox_width * 100}%`,
                      height: `${d.bbox_height * 100}%`,
                    }}>
                      <span className="bbox-label">{d.label} {(d.confidence * 100).toFixed(0)}%</span>
                    </div>
                  ))}
                </div>
                <div className="meta">
                  <div><b>Frame {String(f.frame_index).padStart(6, '0')}</b> {f.timestamp_seconds.toFixed(2)}s</div>
                  <div>Segment {segments.find(s => s.id === f.segment_id)?.segment_index ?? '-'} · {f.width}×{f.height}</div>
                  <div>{dets.length} detections</div>
                </div>
              </div>
            )
          })}
        </div>
        {frames.length === 0 && <div className="empty">No frames</div>}
      </div>

      {selectedFrame && (
        <div className="card" style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.85)', zIndex: 20, overflow: 'auto', padding: 20 }} onClick={() => setSelectedFrame(null)}>
          <div style={{ maxWidth: 900, margin: '0 auto', background: '#1a1d23', padding: 16, borderRadius: 8 }} onClick={e => e.stopPropagation()}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
              <h3>Frame {selectedFrame.frame_index} — {selectedFrame.timestamp_seconds.toFixed(2)}s</h3>
              <button className="btn" onClick={() => setSelectedFrame(null)}>Close</button>
            </div>
            <div className="img-wrap" style={{ position: 'relative', background: '#000' }}>
              <img src={imgSrc(selectedFrame)} alt="selected" style={{ width: '100%' }} />
              {(detectionsByFrame[selectedFrame.id] || []).map(d => (
                <div key={d.id} className="bbox" style={{
                  left: `${d.bbox_x * 100}%`,
                  top: `${d.bbox_y * 100}%`,
                  width: `${d.bbox_width * 100}%`,
                  height: `${d.bbox_height * 100}%`,
                }}>
                  <span className="bbox-label">{d.label} {(d.confidence * 100).toFixed(0)}%</span>
                </div>
              ))}
            </div>
            <div style={{ marginTop: 12 }}>
              <h4>Detections ({(detectionsByFrame[selectedFrame.id] || []).length})</h4>
              <table className="table">
                <thead><tr><th>Label</th><th>Confidence</th><th>BBox</th><th>Detector</th></tr></thead>
                <tbody>
                  {(detectionsByFrame[selectedFrame.id] || []).map(d => (
                    <tr key={d.id}><td>{d.label}</td><td>{(d.confidence * 100).toFixed(1)}%</td><td>{d.bbox_x.toFixed(2)},{d.bbox_y.toFixed(2)},{d.bbox_width.toFixed(2)},{d.bbox_height.toFixed(2)}</td><td>{d.detector_name} v{d.detector_version}</td></tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
