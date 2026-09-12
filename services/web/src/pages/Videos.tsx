import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { client } from '../api/client'
import type { Video, MediaMetadata } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'
import { Loading, ErrorState } from '../components/Loading'

export default function Videos() {
  const [videos, setVideos] = useState<Video[]>([])
  const [mediaMap, setMediaMap] = useState<Record<string, MediaMetadata>>({})
  const [err, setErr] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [filterStatus, setFilterStatus] = useState('')
  const [filterSource, setFilterSource] = useState('')
  const [uploading, setUploading] = useState(false)

  const fetch = async () => {
    try {
      setLoading(true)
      const v = await client.get<Video[]>('/api/v1/videos/')
      const list = Array.isArray(v) ? v : (v as any).videos || []
      setVideos(list)
      // fetch media for duration/resolution
      const m: Record<string, MediaMetadata> = {}
      await Promise.all(list.map(async (vid: Video) => {
        try {
          const mm = await client.get<MediaMetadata>(`/api/v1/videos/${vid.id}/media`)
          m[vid.id] = mm
        } catch {}
      }))
      setMediaMap(m)
      setErr(null)
    } catch (e: any) { setErr(e.message) } finally { setLoading(false) }
  }
  useEffect(() => { fetch() }, [])

  const onUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    if (!f) return
    setUploading(true)
    try {
      await client.upload('/api/v1/videos/', f)
      await fetch()
    } catch (e: any) { alert(e.message) } finally { setUploading(false); e.target.value = '' }
  }

  if (loading) return <Loading />
  if (err) return <ErrorState error={err} retry={fetch} />

  const onDelete = async (id: string, filename: string) => {
    if (!confirm(`Delete video "${filename}"? This will permanently delete the video and all its frames, tracks and metadata.`)) return
    try {
      await client.del(`/api/v1/videos/${id}`)
      setVideos(prev => prev.filter(v => v.id !== id))
    } catch (e: any) { alert(`Delete failed: ${e.message}`) }
  }

  const filtered = videos.filter(v => (!filterStatus || v.status === filterStatus) && (!filterSource || v.source_type === filterSource))

  return (
    <div>
      <div className="card">
        <h3>Videos ({filtered.length})</h3>
        <div className="filters">
          <select value={filterStatus} onChange={e => setFilterStatus(e.target.value)}>
            <option value="">All status</option>
            <option>UPLOADED</option><option>PROCESSING</option><option>READY</option><option>FAILED</option>
          </select>
          <select value={filterSource} onChange={e => setFilterSource(e.target.value)}>
            <option value="">All sources</option><option>LOCAL</option><option>UPLOAD</option>
          </select>
          <label className="btn btn-primary" style={{ cursor: 'pointer' }}>
            {uploading ? 'Uploading…' : 'Upload Video'}
            <input type="file" accept=".mp4,.mkv,.avi,.mov" style={{ display: 'none' }} onChange={onUpload} />
          </label>
          <button className="btn" onClick={fetch}>Refresh</button>
        </div>
        <table className="table">
          <thead><tr><th>Filename</th><th>Source</th><th>Status</th><th>Size</th><th>Duration</th><th>Resolution</th><th>Created</th><th>Actions</th></tr></thead>
          <tbody>
            {filtered.map(v => {
              const m = mediaMap[v.id]
              return (
                <tr key={v.id}>
                  <td><Link to={`/videos/${v.id}`}>{v.filename}</Link></td>
                  <td>{v.source_type}</td>
                  <td><StatusBadge status={v.status} /></td>
                  <td>{(v.size_bytes / 1024 / 1024).toFixed(2)} MB</td>
                  <td>{m?.duration_seconds ? `${m.duration_seconds.toFixed(2)}s` : '-'}</td>
                  <td>{m?.video_width && m?.video_height ? `${m.video_width}×${m.video_height}` : '-'}</td>
                  <td>{new Date(v.created_at).toLocaleString()}</td>
                  <td><button className="btn" style={{ color: '#f87171', borderColor: '#f87171' }} onClick={() => onDelete(v.id, v.filename)}>Delete</button></td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {filtered.length === 0 && <div className="empty">No videos match filter</div>}
      </div>
    </div>
  )
}
