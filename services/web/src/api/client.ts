const BASE = (import.meta.env.VITE_API_BASE_URL as string) || 'http://localhost:8080'

async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...opts,
    headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) },
  })
  if (!res.ok) {
    const txt = await res.text()
    let msg = txt
    try { const j = JSON.parse(txt); msg = j.error || txt } catch {}
    throw new Error(`${res.status} ${msg}`)
  }
  if (res.status === 204) return undefined as unknown as T
  const ct = res.headers.get('content-type') || ''
  if (ct.includes('image')) return res as unknown as T
  return res.json() as Promise<T>
}

export const client = {
  get: <T>(p: string) => req<T>(p),
  post: <T>(p: string, body?: unknown) => req<T>(p, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  del: <T>(p: string) => req<T>(p, { method: 'DELETE' }),
  upload: async <T>(p: string, file: File): Promise<T> => {
    const fd = new FormData()
    fd.append('file', file)
    const res = await fetch(`${BASE}${p}`, { method: 'POST', body: fd })
    if (!res.ok) {
      const txt = await res.text()
      throw new Error(txt)
    }
    return res.json()
  },
  imageUrl: (path: string) => `${BASE}${path}`,
  base: BASE,
}
