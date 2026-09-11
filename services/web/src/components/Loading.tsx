export function Loading() { return <div className="loading">Loading…</div> }
export function ErrorState({ error, retry }: { error: string; retry?: () => void }) {
  return <div className="error">Error: {error} {retry && <button onClick={retry}>Retry</button>}</div>
}
export function EmptyState({ msg }: { msg: string }) { return <div className="empty">{msg}</div> }
