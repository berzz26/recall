import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { client } from '../api/client'
import type { Video, LocalSource, Health } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'
import { Loading, ErrorState } from '../components/Loading'

export default function Dashboard() {
  const [videos, setVideos] = useState<Video[]>([])
  const [sources, setSources] = useState<LocalSource[]>([])
  const [health, setHealth] = useState<Health | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const fetch = async () => {
    try {
      setLoading(true)
      const [v, s] = await Promise.all([
        client.get<Video[]>('/api/v1/videos/'),
        client.get<LocalSource[]>('/api/v1/local-sources/').catch(() => [] as LocalSource[]),
      ])
      setVideos(Array.isArray(v) ? v : (v as any).videos || [])
      setSources(s)
      try {
        const h = await client.get<Health>('/health')
        setHealth(h)
      } catch {}
      setErr(null)
    } catch (e: any) { setErr(e.message) } finally { setLoading(false) }
  }
  useEffect(() => { fetch() }, [])

  if (loading) return <Loading />
  if (err) return <ErrorState error={err} retry={fetch} />

  const counts = {
    total: videos.length,
    UPLOADED: videos.filter(v => v.status === 'UPLOADED').length,
    PROCESSING: videos.filter(v => v.status === 'PROCESSING').length,
    READY: videos.filter(v => v.status === 'READY').length,
    FAILED: videos.filter(v => v.status === 'FAILED').length,
  }
  const recent = [...videos].sort((a, b) => +new Date(b.created_at) - +new Date(a.created_at)).slice(0, 5)
  const failed = videos.filter(v => v.status === 'FAILED').slice(0, 5)

  return (
    <div>
      <div className="card">
        <h3>System Health</h3>
        {health ? <span className={health.database === 'ok' ? 'health-ok' : 'health-bad'}>API: {health.status} / DB: {health.database}</span> : 'Checking…'}
      </div>

      <div className="card">
        <h3>Video Counts</h3>
        <div className="summary">
          <span className="item">Total: <b>{counts.total}</b></span>
          <span className="item">UPLOADED: <b>{counts.UPLOADED}</b></span>
          <span className="item">PROCESSING: <b>{counts.PROCESSING}</b></span>
          <span className="item">READY: <b>{counts.READY}</b></span>
          <span className="item">FAILED: <b>{counts.FAILED}</b></span>
        </div>
      </div>

      <div className="card">
        <h3>Recent Videos</h3>
        <table className="table">
          <thead><tr><th>Filename</th><th>Status</th><th>Source</th><th>Duration</th><th>Created</th></tr></thead>
          <tbody>
            {recent.map(v => (
              <tr key={v.id}>
                <td><Link to={`/videos/${v.id}`}>{v.filename}</Link></td>
                <td><StatusBadge status={v.status} /></td>
                <td>{v.source_type}</td>
                <td>-</td>
                <td>{new Date(v.created_at).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {recent.length === 0 && <div className="empty">No videos</div>}
      </div>

      <div className="card">
        <h3>Processing Failures</h3>
        <table className="table">
          <thead><tr><th>Filename</th><th>Failure reason</th><th>Updated</th></tr></thead>
          <tbody>
            {failed.map(v => (
              <tr key={v.id}>
                <td><Link to={`/videos/${v.id}`}>{v.filename}</Link></td>
                <td style={{ maxWidth: 400, overflow: 'hidden', textOverflow: 'ellipsis' }}>{v.processing_error || '-'}</td>
                <td>{new Date(v.updated_at).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {failed.length === 0 && <div className="empty">No failures</div>}
      </div>

      <div className="card">
        <h3>Local Sources</h3>
        <table className="table">
          <thead><tr><th>Name</th><th>Path</th><th>Enabled</th></tr></thead>
          <tbody>{sources.map(s => (
            <tr key={s.id}><td>{s.name}</td><td>{s.path}</td><td>{s.enabled ? 'yes' : 'no'}</td></tr>
          ))}</tbody>
        </table>
        {sources.length === 0 && <div className="empty">No local sources</div>}
      </div>
    </div>
  )
}
