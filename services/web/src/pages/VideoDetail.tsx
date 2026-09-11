import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { client } from '../api/client'
import type { Video, MediaMetadata, Segment, Frame, Detection, Track } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'
import { Loading, ErrorState } from '../components/Loading'

export default function VideoDetail() {
  const { id } = useParams()
  const [video, setVideo] = useState<Video | null>(null)
  const [media, setMedia] = useState<MediaMetadata | null>(null)
  const [segments, setSegments] = useState<Segment[]>([])
  const [frames, setFrames] = useState<Frame[]>([])
  const [detections, setDetections] = useState<Detection[]>([])
  const [tracks, setTracks] = useState<Track[]>([])
  const [detToTrack, setDetToTrack] = useState<Record<string, number>>({})
  const [selectedTrack, setSelectedTrack] = useState<number | 'all'>('all')
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
      try {
        const t = await client.get<Track[]>(`/api/v1/videos/${id}/tracks`)
        setTracks(t)
        // build detection -> track map
        const map: Record<string, number> = {}
        await Promise.all(t.map(async (tr) => {
          try {
            const links: any[] = await client.get<any[]>(`/api/v1/tracks/${tr.id}/detections`)
            links.forEach((l: any) => { map[l.detection_id] = tr.track_index })
          } catch {}
        }))
        setDetToTrack(map)
      } catch { setTracks([]); setDetToTrack({}) }
      setErr(null)
    } catch (e: any) { setErr(e.message) }
  }

  useEffect(() => { fetchAll() }, [id])

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
        <h3>Tracks ({tracks.length})</h3>
        {tracks.length === 0 ? <div className="empty">No tracks</div> : (
          <>
            <div style={{ marginBottom: 12 }}>
              <select
                value={selectedTrack === 'all' ? 'all' : String(selectedTrack)}
                onChange={e => setSelectedTrack(e.target.value === 'all' ? 'all' : Number(e.target.value))}
                style={{ minWidth: 320, padding: '8px 10px', fontSize: 13 }}
              >
                <option value="all">All tracks ({tracks.length})</option>
                {tracks.map(t => (
                  <option key={t.id} value={String(t.track_index)}>
                    Track {t.track_index} — {t.label} — {t.start_timestamp.toFixed(1)}s → {t.end_timestamp.toFixed(1)}s — {t.detection_count} dets
                  </option>
                ))}
              </select>
            </div>
            {selectedTrack === 'all' ? (
              <div className="summary" style={{ flexDirection: 'column', gap: 8 }}>
                {tracks.map(t => (
                  <div key={t.id} className="item" style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
                    <span style={{ minWidth: 80, fontWeight: 700, textTransform: 'uppercase' }}>{t.label}</span>
                    <span>Track {t.track_index}</span>
                    <span>{t.start_timestamp.toFixed(1)}s → {t.end_timestamp.toFixed(1)}s</span>
                    <span>{t.detection_count} detections</span>
                    <span style={{ color: '#9aa0b0', fontSize: 11 }}>{t.tracker_name} v{t.tracker_version}</span>
                  </div>
                ))}
              </div>
            ) : (
              (() => {
                const t = tracks.find(x => x.track_index === selectedTrack)
                if (!t) return <div className="empty">Track not found</div>
                return (
                  <div className="item" style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
                    <span style={{ minWidth: 80, fontWeight: 700, textTransform: 'uppercase' }}>{t.label}</span>
                    <span>Track {t.track_index}</span>
                    <span>{t.start_timestamp.toFixed(1)}s → {t.end_timestamp.toFixed(1)}s</span>
                    <span>{t.detection_count} detections</span>
                    <span style={{ color: '#9aa0b0', fontSize: 11 }}>{t.tracker_name} v{t.tracker_version}</span>
                  </div>
                )
              })()
            )}
          </>
        )}
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
        <h3>Frames ({frames.length}) {selectedTrack !== 'all' ? `— filtered to Track ${selectedTrack}` : ''}</h3>
        <div className="frame-grid">
          {frames
            .filter(f => {
              if (selectedTrack === 'all') return true
              const dets = detectionsByFrame[f.id] || []
              return dets.some(d => detToTrack[d.id] === selectedTrack)
            })
            .map(f => {
            const dets = detectionsByFrame[f.id] || []
            return (
              <div key={f.id} className="frame-card" onClick={() => setSelectedFrame(f)} style={{ cursor: 'pointer', opacity: selectedTrack !== 'all' && !dets.some(d => detToTrack[d.id] === selectedTrack) ? 0.6 : 1 }}>
                <div className="img-wrap">
                  <img src={imgSrc(f)} alt={`frame ${f.frame_index}`} loading="lazy" />
                  {dets.map(d => {
                    const trackIdx = detToTrack[d.id]
                    const highlighted = selectedTrack !== 'all' && trackIdx === selectedTrack
                    const dimmed = selectedTrack !== 'all' && trackIdx !== selectedTrack
                    return (
                      <div key={d.id} className="bbox" style={{
                        left: `${d.bbox_x * 100}%`,
                        top: `${d.bbox_y * 100}%`,
                        width: `${d.bbox_width * 100}%`,
                        height: `${d.bbox_height * 100}%`,
                        borderColor: highlighted ? '#22c55e' : undefined,
                        background: highlighted ? 'rgba(34,197,94,0.18)' : dimmed ? 'rgba(250,204,21,0.06)' : undefined,
                        opacity: dimmed ? 0.4 : 1,
                      }}>
                        <span className="bbox-label" style={highlighted ? { background: '#22c55e' } : undefined}>{d.label} {(d.confidence * 100).toFixed(0)}%{trackIdx !== undefined ? ` T${trackIdx}` : ''}</span>
                      </div>
                    )
                  })}
                </div>
                <div className="meta">
                  <div><b>Frame {String(f.frame_index).padStart(6, '0')}</b> {f.timestamp_seconds.toFixed(2)}s</div>
                  <div>Segment {segments.find(s => s.id === f.segment_id)?.segment_index ?? '-'} · {f.width}×{f.height}</div>
                  <div>{dets.length} detections {selectedTrack !== 'all' ? `(${dets.filter(d => detToTrack[d.id] === selectedTrack).length} in T${selectedTrack})` : ''}</div>
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
              {(detectionsByFrame[selectedFrame.id] || []).map(d => {
                const trackIdx = detToTrack[d.id]
                const highlighted = selectedTrack !== 'all' && trackIdx === selectedTrack
                const dimmed = selectedTrack !== 'all' && trackIdx !== selectedTrack
                return (
                  <div key={d.id} className="bbox" style={{
                    left: `${d.bbox_x * 100}%`,
                    top: `${d.bbox_y * 100}%`,
                    width: `${d.bbox_width * 100}%`,
                    height: `${d.bbox_height * 100}%`,
                    borderColor: highlighted ? '#22c55e' : undefined,
                    background: highlighted ? 'rgba(34,197,94,0.18)' : dimmed ? 'rgba(250,204,21,0.06)' : undefined,
                    opacity: dimmed ? 0.4 : 1,
                  }}>
                    <span className="bbox-label" style={highlighted ? { background: '#22c55e' } : undefined}>{d.label} {(d.confidence * 100).toFixed(0)}%{trackIdx !== undefined ? ` T${trackIdx}` : ''}</span>
                  </div>
                )
              })}
            </div>
            <div style={{ marginTop: 12 }}>
              <h4>Detections ({(detectionsByFrame[selectedFrame.id] || []).length})</h4>
              <table className="table">
                <thead><tr><th>Label</th><th>Confidence</th><th>BBox</th><th>Track</th><th>Detector</th></tr></thead>
                <tbody>
                  {(detectionsByFrame[selectedFrame.id] || []).map(d => {
                    const trackIdx = detToTrack[d.id]
                    const highlighted = selectedTrack !== 'all' && trackIdx === selectedTrack
                    return (
                      <tr key={d.id} style={highlighted ? { background: 'rgba(34,197,94,0.12)', fontWeight: 600 } : undefined}><td>{d.label}</td><td>{(d.confidence * 100).toFixed(1)}%</td><td>{d.bbox_x.toFixed(2)},{d.bbox_y.toFixed(2)},{d.bbox_width.toFixed(2)},{d.bbox_height.toFixed(2)}</td><td>{trackIdx !== undefined ? `Track ${trackIdx}` : '-'}</td><td>{d.detector_name} v{d.detector_version}</td></tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
