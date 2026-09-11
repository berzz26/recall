import type { Status } from '../api/types'

export function StatusBadge({ status }: { status: Status }) {
  const cls = `badge badge-${status.toLowerCase()}`
  return <span className={cls}>{status}</span>
}
