import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
export function cn(...inputs: ClassValue[]) { return twMerge(clsx(inputs)) }
export function bytes(n: number) {
  if (!Number.isFinite(n) || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1000)), units.length - 1)
  return `${(n / 1000 ** i).toLocaleString('zh-CN', { maximumFractionDigits: i ? 2 : 0 })} ${units[i]}`
}
export function timeLabel(epoch: number, timezone: string) {
  return new Intl.DateTimeFormat('zh-CN', { timeZone: timezone, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(epoch * 1000)
}
export function hourLabel(epoch: number, timezone: string) {
  return `${timeLabel(epoch, timezone)}–${timeLabel(epoch + 3600, timezone)}`
}
