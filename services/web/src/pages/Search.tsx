import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { client } from '../api/client'

interface SearchResult {
  video_id: string
  video_filename?: string
  filename?: string
  segment_id: string
  start_time: number
  end_time: number
  description: string
  similarity: number
  detections: { label: string }[]
  tracks: any[]
  events: any[]
}
interface SearchResponse { query: string; results: SearchResult[] }

function formatTimestamp(sec: number) {
  const s = Math.floor(sec), h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec2 = s % 60
  if (h >= 1) return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(sec2).padStart(2, '0')}`
  return `${String(m).padStart(2, '0')}:${String(sec2).padStart(2, '0')}`
}

export default function Search() {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<SearchResult[] | null>(null)
  const [resultQuery, setResultQuery] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [hasSearched, setHasSearched] = useState(false)

  const doSearch = async () => {
    const q = query.trim(); if (!q) { setError('Query is required'); return }
    setLoading(true); setError(null); setHasSearched(true)
    try {
      const res = await client.post<SearchResponse>('/api/v1/search', { query: q, limit: 10 })
      setResults(res.results || []); setResultQuery(res.query)
    } catch (e: any) { setError(e.message || 'Search failed. Please try again.'); setResults([]) } finally { setLoading(false) }
  }

  return (
    <div>
      <div style={{ marginBottom: 14 }}>
        <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0, letterSpacing: -0.02 }}>Search</h1>
        <div style={{ fontSize: 12, color: 'var(--muted)', marginTop: 2 }}>Find moments in your videos using natural language</div>
      </div>

      <div className="card" style={{ padding: 12 }}>
        <div className="search-hero">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#9aa3b1" strokeWidth="1.8"><circle cx="11" cy="11" r="6" /><path d="M20 20L15.3 15.3" /></svg>
          <input placeholder="person wearing a white shirt near the counter" value={query} onChange={e => setQuery(e.target.value)} onKeyDown={e => e.key === 'Enter' && doSearch()} />
          <button className="btn btn-primary" onClick={doSearch} style={{ padding: '8px 14px', borderRadius: 8 }}><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2"><circle cx="11" cy="11" r="6" /><path d="M20 20L15 15" /></svg></button>
        </div>
      </div>

      <div className="card" style={{ marginTop: 12, padding: '12px 16px' }}>
        <div className="search-meta">
          <div className="search-count">{hasSearched ? `${results?.length ?? 0} results${resultQuery ? ` for "${resultQuery}"` : ''}` : 'Enter a query to search'}</div>
          <select className="select" defaultValue="relevance"><option>Most relevant</option><option>Newest first</option></select>
        </div>

        {loading && <div style={{ padding: 20, textAlign: 'center', color: 'var(--muted)', fontSize: 13 }}>Searching…</div>}
        {error && <div style={{ padding: 12, color: '#ef4444', fontSize: 13 }}>{error}</div>}

        {!loading && !error && hasSearched && results && results.length === 0 && (
          <div style={{ padding: 32, textAlign: 'center', color: 'var(--muted)', fontSize: 13 }}>No matching scenes found. Try a different query.</div>
        )}

        {!loading && !error && results && results.length > 0 && (
          <div>
            {results.map(r => {
              const filename = (r as any).filename || (r as any).video_filename || r.video_id.slice(0, 8)
              // Use first frame thumbnail if we had frames; otherwise generic placeholder background
              return (
                <div key={r.segment_id} className="result-card">
                  <div className="result-thumb">
                    <div style={{ width: 96, height: 60, borderRadius: 8, background: '#eef2f0', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#9aa3b1" strokeWidth="1.6"><rect x="2" y="2" width="20" height="15" rx="2" /><circle cx="8" cy="9.5" r="2" /><path d="M2 14l6-4 4 3 4-4 6 5" /></svg>
                    </div>
                    <div className="duration-badge">{formatTimestamp(r.end_time - r.start_time)}</div>
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className="result-title">{filename} • {formatTimestamp(r.start_time)}</div>
                    <div className="result-desc">{r.description || 'No description available.'}</div>
                    <div className="result-meta"><span>Similarity {Math.round((r.similarity || 0) * 100)}%</span><span>•</span><span>{formatTimestamp(r.start_time)} - {formatTimestamp(r.end_time)}</span></div>
                  </div>
                  <div className="result-action"><button className="btn-open" onClick={() => navigate(`/videos/${r.video_id}?t=${Math.floor(r.start_time)}`)}>Open at this</button></div>
                </div>
              )
            })}
          </div>
        )}

        {!hasSearched && !loading && (
          <div style={{ padding: 24, textAlign: 'center', color: 'var(--muted)', fontSize: 12, border: '1px dashed var(--border)', borderRadius: 8, marginTop: 12 }}>
            Try queries like “person entering store”, “vehicle near entrance”, “person at counter”
          </div>
        )}
      </div>
    </div>
  )
}
