export type Status = {
  connected: boolean; last_flush: number; flush_interval_ms: number; retention_days: number; storage_error: string; version: string; last_sample: number; started_at: number
  active: number; inner: number; unknown_domain: number; samples: number
  reconnects: number; error: string; interval_ms: number; timezone: string; today: string; now: number
}
export type Row = { hour: number; node: string; domain: string; upload: number; download: number }
export type Traffic = {
  rows: Row[]; nodes: string[]; hours: number[]; date: string; timezone: string
  gaps: { start: number; end: number; reason: string }[]
  global_upload: number; global_download: number; all_upload: number; all_download: number
}
export async function get<T>(url: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(url, { signal })
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error || `请求失败（${res.status}）`)
  }
  return res.json() as Promise<T>
}
export function trafficParams(date: string, node: string, domain: string, group: string) {
  const params = new URLSearchParams({ date, group })
  if (node !== 'all') params.set('node', node)
  if (domain.trim()) params.set('domain', domain.trim())
  return params.toString()
}
