import { useEffect, useState, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { client } from '../api/client'
import type { Video, MediaMetadata, Segment } from '../api/types'

export default function Videos() {
  const [videos, setVideos] = useState<Video[]>([])
  const [mediaMap, setMediaMap] = useState<Record<string, MediaMetadata>>({})
  const [segCountMap, setSegCountMap] = useState<Record<string, number>>({})
  const [thumbMap, setThumbMap] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [q, setQ] = useState('')
  const [sort, setSort] = useState('newest')
  const [uploading, setUploading] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const fetchAll = async () => {
    try {
      setLoading(true)
      setErr(null)
      const v = await client.get<Video[]>('/api/v1/videos/')
      const list = Array.isArray(v) ? v : (v as any).videos || []
      setVideos(list)
      const m: Record<string, MediaMetadata> = {}
      const seg: Record<string, number> = {}
      const thumbs: Record<string, string> = {}
      await Promise.all(list.map(async (vid: Video) => {
        try { m[vid.id] = await client.get<MediaMetadata>(`/api/v1/videos/${vid.id}/media`) } catch {}
        try {
          const s = await client.get<Segment[]>(`/api/v1/videos/${vid.id}/segments`)
          seg[vid.id] = s.length
        } catch { seg[vid.id] = 0 }
        try {
          const frames: any[] = await client.get<any[]>(`/api/v1/videos/${vid.id}/frames`)
          if (frames.length > 0) thumbs[vid.id] = client.imageUrl(`/api/v1/videos/${vid.id}/frames/${frames[0].id}/image`)
        } catch {}
      }))
      setMediaMap(m)
      setSegCountMap(seg)
      setThumbMap(thumbs)
    } catch (e: any) { setErr(e.message) } finally { setLoading(false) }
  }
  useEffect(() => { fetchAll() }, [])

  const onUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]; if (!f) return
    setUploading(true)
    try { await client.upload('/api/v1/videos/', f); await fetchAll() } catch (err: any) { alert(err.message) } finally { setUploading(false); e.target.value = '' }
  }

  const onDelete = async (id: string, filename: string) => {
    if (!confirm(`Delete video "${filename}"? This will permanently delete the video and all its frames, tracks and metadata.`)) return
    try { await client.del(`/api/v1/videos/${id}`); setVideos(prev => prev.filter(v => v.id !== id)) } catch (e: any) { alert(`Delete failed: ${e.message}`) }
  }

  const filtered = useMemo(() => {
    let list = [...videos]
    if (q) list = list.filter(v => v.filename.toLowerCase().includes(q.toLowerCase()))
    if (sort === 'newest') list.sort((a, b) => +new Date(b.created_at) - +new Date(a.created_at))
    else list.sort((a, b) => +new Date(a.created_at) - +new Date(b.created_at))
    return list
  }, [videos, q, sort])

  if (loading) return <div className="card" style={{padding:40,textAlign:'center',color:'var(--muted)'}}>Loading videos…</div>
  if (err) return <div className="card" style={{padding:24}}><div style={{color:'#ef4444',marginBottom:12}}>Error: {err}</div><button className="btn" onClick={fetchAll}>Retry</button></div>

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 14 }}>
        <div>
          <h1 style={{ fontSize: 20, fontWeight: 700, margin: '0 0 2px 0', letterSpacing: -0.02 }}>Videos</h1>
          <div style={{ fontSize: 12, color: 'var(--muted)' }}>Browse and manage your videos</div>
        </div>
        <label className="btn btn-primary" style={{ cursor: 'pointer' }}>
          {uploading ? 'Uploading…' : '+ Add Video'}
          <input type="file" accept=".mp4,.mkv,.avi,.mov" style={{ display: 'none' }} onChange={onUpload} />
        </label>
      </div>

      <div style={{ display: 'flex', gap: 10, marginBottom: 12 }}>
        <div className="input-wrap" style={{ maxWidth: 340 }}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="11" cy="11" r="6" /><path d="M20 20L15 15" /></svg>
          <input className="input" placeholder="Search videos..." value={q} onChange={e => setQ(e.target.value)} />
        </div>
        <select className="select" value={sort} onChange={e => setSort(e.target.value)} style={{ marginLeft: 'auto' }}>
          <option value="newest">Newest first</option>
          <option value="oldest">Oldest first</option>
        </select>
      </div>

      <div className="card" style={{ padding: 0, overflow: 'hidden' }}>
        <div className="table-wrap" style={{ border: 'none' }}>
          <table className="table">
            <thead><tr><th>Thumbnail</th><th>Name</th><th>Date</th><th>Duration</th><th>Segments</th><th>Status</th><th>Actions</th></tr></thead>
            <tbody>
              {filtered.map(v => {
                const m = mediaMap[v.id]
                const dur = m?.duration_seconds
                  ? `${String(Math.floor(m.duration_seconds / 3600)).padStart(2, '0')}:${String(Math.floor((m.duration_seconds % 3600) / 60)).padStart(2, '0')}:${String(Math.floor(m.duration_seconds % 60)).padStart(2, '0')}`
                  : '-'
                const thumb = thumbMap[v.id]
                const date = new Date(v.created_at)
                const dateStr = date.toLocaleDateString('en-US', { month: 'short', day: '2-digit', year: 'numeric' })
                const timeStr = date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false })
                const segs = segCountMap[v.id] ?? 0
                const isReady = v.status === 'READY'
                return (
                  <tr key={v.id}>
                    <td>
                      {thumb
                        ? <img className="thumb" src={thumb} alt="" loading="lazy" />
                        : <div className="thumb" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', background: '#f0f2f1' }}>
                          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#9aa3b1" strokeWidth="1.6"><rect x="2" y="2" width="20" height="15" rx="2" /><circle cx="8" cy="9.5" r="2" /><path d="M2 14l6-4 4 3 4-4 6 5" /></svg>
                        </div>
                      }
                    </td>
                    <td><Link to={`/videos/${v.id}`} className="text-bold">{v.filename}</Link><div className="text-muted" style={{ maxWidth: 220, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{v.source_path || v.storage_key || '-'}</div></td>
                    <td><div style={{ fontSize: 12, fontWeight: 500 }}>{dateStr}</div><div className="text-muted">{timeStr}</div></td>
                    <td style={{ fontSize: 12, color: 'var(--muted)' }}>{dur}</td>
                    <td style={{ fontSize: 12, color: 'var(--muted)' }}>{segs}</td>
                    <td><span className={`badge ${isReady ? 'badge-green' : 'badge-gray'}`}>{isReady ? 'Processed' : v.status}</span></td>
                    <td>
                      <button className="btn" title="Delete video" onClick={() => onDelete(v.id, v.filename)} style={{ border: '1px solid var(--border)', background: 'white', color: '#dc2626', padding: '6px 8px', borderRadius: 8 }}>
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
          {filtered.length === 0 && <div style={{ padding: 32, textAlign: 'center', color: 'var(--muted)', fontSize: 13 }}>No videos found. Upload a video or add a local source in <Link to="/local-sources" style={{ color: 'var(--accent)', fontWeight: 600 }}>Settings</Link>.</div>}
        </div>
      </div>
      <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 8 }}>Showing {filtered.length} videos</div>
    </div>
  )
}
