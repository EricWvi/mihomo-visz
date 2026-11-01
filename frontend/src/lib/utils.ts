import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
export function cn(...inputs: ClassValue[]) { return twMerge(clsx(inputs)) }
const units = ['B', 'KB', 'MB', 'GB', 'TB']
export function byteParts(n: number): [string, string] {
  if (!Number.isFinite(n) || n <= 0) return ['0', 'B']
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1000)), units.length - 1)
  return [(n / 1000 ** i).toLocaleString('zh-CN', { maximumFractionDigits: i ? 2 : 0 }), units[i]]
}
export function bytes(n: number) { return byteParts(n).join(' ') }
export function percent(part: number, whole: number, digits = 1) {
  if (!whole) return '—'
  const p = part / whole * 100
  return `${p > 0 && p < 0.1 ? '<0.1' : p.toFixed(digits)}%`
}
export function timeLabel(epoch: number, timezone: string) {
  return new Intl.DateTimeFormat('zh-CN', { timeZone: timezone, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(epoch * 1000)
}
export function hourLabel(epoch: number, timezone: string) {
  return `${timeLabel(epoch, timezone)}–${timeLabel(epoch + 3600, timezone)}`
}
/** Rounds the axis maximum up to 1 / 2 / 2.5 / 5 × 10ⁿ so ticks stay readable. */
export function niceScale(max: number, intervals = 4) {
  if (max <= 0) return { max: 1, ticks: [0] }
  const raw = max / intervals
  const mag = 10 ** Math.floor(Math.log10(raw))
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= raw)!
  const top = Math.ceil(max / step) * step
  return { max: top, ticks: Array.from({ length: Math.round(top / step) + 1 }, (_, i) => i * step) }
}
export function shiftDate(date: string, delta: number) {
  const d = new Date(`${date}T12:00:00Z`)
  if (Number.isNaN(d.valueOf())) return date
  d.setUTCDate(d.getUTCDate() + delta)
  return d.toISOString().slice(0, 10)
}
const weekdays = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
export function weekday(date: string) { return weekdays[new Date(`${date}T12:00:00Z`).getUTCDay()] }
export function longDate(date: string) {
  const [y, m, d] = date.split('-').map(Number)
  return y ? `${y} 年 ${m} 月 ${d} 日 · ${weekday(date)}` : ''
}
export function ago(ms: number) {
  const s = Math.max(0, Math.round(ms / 1000))
  if (s < 5) return '刚刚'
  if (s < 60) return `${s} 秒前`
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`
  return `${Math.floor(s / 3600)} 小时前`
}
