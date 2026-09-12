import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { client } from '../api/client'
import type { Video } from '../api/types'

interface DetectionInfo { label: string; confidence: number }
interface TrackInfo { track_id: string; label: string; start_time: number; end_time: number }
interface EventInfo { event_id: string; event_type: string; label: string; start_time: number; end_time: number | null; confidence: number | null }
interface SearchResult {
  video_id: string
  video_filename?: string
  filename?: string
  segment_id: string
  start_time: number
  end_time: number
  description: string
  similarity: number
  detections: DetectionInfo[]
  tracks: TrackInfo[]
  events: EventInfo[]
}
interface SearchResponse {
  query: string
  results: SearchResult[]
}

function formatTimestamp(seconds: number): string {
  if (seconds < 0) seconds = 0
  const s = Math.floor(seconds)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (h >= 1) {
    return `${String(h).padStart(2,'0')}:${String(m).padStart(2,'0')}:${String(sec).padStart(2,'0')}`
  }
  return `${String(m).padStart(2,'0')}:${String(sec).padStart(2,'0')}`
}

export default function Search() {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [videoId, setVideoId] = useState('')
  const [videos, setVideos] = useState<Video[]>([])
  const [results, setResults] = useState<SearchResult[] | null>(null)
  const [resultQuery, setResultQuery] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    client.get<Video[]>('/api/v1/videos/').then(v => {
      const list = Array.isArray(v) ? v : (v as any).videos || []
      setVideos(list)
    }).catch(()=>{})
  }, [])

  const doSearch = async () => {
    const q = query.trim()
    if (!q) {
      setError('Query is required')
      return
    }
    setLoading(true)
    setError(null)
    try {
      const body: any = { query: q, limit: 10 }
      if (videoId) body.video_id = videoId
      const res = await client.post<SearchResponse>('/api/v1/search', body)
      setResults(res.results || [])
      setResultQuery(res.query)
    } catch (e: any) {
      setError(e.message || 'Search failed. Please try again.')
      setResults(null)
    } finally {
      setLoading(false)
    }
  }

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') doSearch()
  }

  return (
    <div>
      <div className="card">
        <h2 style={{ marginBottom: 16 }}>ReCall Search</h2>
        <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
          <input
            style={{ flex: 1, padding: '10px 12px', fontSize: 14 }}
            placeholder="person walking near checkout"
            value={query}
            onChange={e => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
          />
          <button className="btn btn-primary" onClick={doSearch} disabled={loading} style={{ minWidth: 90 }}>
            {loading ? 'Searching…' : 'Search'}
          </button>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          <label style={{ fontSize: 13, color: '#9aa0b0' }}>Video:</label>
          <select value={videoId} onChange={e => setVideoId(e.target.value)} style={{ minWidth: 240 }}>
            <option value="">All Videos</option>
            {videos.map(v => (
              <option key={v.id} value={v.id}>{v.filename}</option>
            ))}
          </select>
        </div>
      </div>

      {loading && <div className="loading">Searching…</div>}
      {error && <div className="error" style={{ marginBottom: 16 }}>{error.includes('400') ? 'Search failed. Please try again.' : error}</div>}

      {results !== null && !loading && !error && (
        <div className="card">
          <div style={{ fontSize: 13, color: '#9aa0b0', marginBottom: 12 }}>
            {results.length} results{resultQuery ? ` for "${resultQuery}"` : ''}
          </div>
          {results.length === 0 ? (
            <div className="empty">No matching scenes found.</div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {results.map(r => {
                const filename = r.filename || r.video_filename || r.video_id.slice(0, 8)
                const simPct = Math.round(r.similarity * 100)
                const timeLabel = `${formatTimestamp(r.start_time)} - ${formatTimestamp(r.end_time)}`
                return (
                  <div key={r.segment_id} style={{ padding: 16, background: '#0f1115', border: '1px solid #2a2e39', borderRadius: 8 }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', marginBottom: 6 }}>
                      <span style={{ fontWeight: 600, color: '#e6e8eb' }}>{filename}</span>
                      <span style={{ fontSize: 12, color: '#9aa0b0' }}>{timeLabel}</span>
                    </div>
                    <div style={{ fontSize: 12, color: '#7ab8ff', marginBottom: 8 }}>Similarity: {simPct}%</div>
                    <div style={{ fontSize: 13, lineHeight: 1.5, marginBottom: 10, whiteSpace: 'pre-wrap' }}>{r.description}</div>
                    <div style={{ fontSize: 12, color: '#9aa0b0', display: 'flex', flexDirection: 'column', gap: 4 }}>
                      <div>Objects: {r.detections.length ? r.detections.map(d=>d.label).join(', ') : '-'}</div>
                      <div>Tracks: {r.tracks.length}</div>
                      <div>Events: {r.events.length ? [...new Set(r.events.map(e=>e.event_type))].join(', ') : '-'}</div>
                    </div>
                    <button
                      className="btn btn-primary"
                      style={{ marginTop: 10 }}
                      onClick={() => navigate(`/videos/${r.video_id}?t=${Math.floor(r.start_time)}`)}
                    >
                      Open at {formatTimestamp(r.start_time)}
                    </button>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
