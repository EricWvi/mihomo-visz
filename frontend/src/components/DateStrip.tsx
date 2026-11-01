import { ChevronLeft, ChevronRight } from 'lucide-react'
import { cn, shiftDate, weekday } from '@/lib/utils'

/** Recent days as chips (the retention window), with arrows for stepping beyond it. */
export function DateStrip({ date, today, days, onChange }: { date: string; today: string; days: number; onChange: (date: string) => void }) {
  const list = today ? Array.from({ length: days }, (_, i) => shiftDate(today, i - days + 1)) : []
  return <div className="date-strip">
    <button type="button" className="icon-btn" aria-label="前一天" disabled={!date} onMouseDown={(e) => e.preventDefault()} onClick={() => onChange(shiftDate(date, -1))}><ChevronLeft size={16} /></button>
    <div className="date-chips" role="radiogroup" aria-label="日期">
      {!list.length && Array.from({ length: days }, (_, i) => <span key={i} className="date-chip placeholder" />)}
      {list.map((d, i) => {
        const name = i === days - 1 ? '今天' : i === days - 2 ? '昨天' : weekday(d)
        return <button key={d} type="button" role="radio" aria-checked={d === date} aria-label={`${d} ${name}`} className={cn('date-chip', d === date && 'selected')} onMouseDown={(e) => e.preventDefault()} onClick={() => onChange(d)}>
          <span>{name}</span><b>{Number(d.slice(5, 7))}/{Number(d.slice(8))}</b>
        </button>
      })}
    </div>
    <button type="button" className="icon-btn" aria-label="后一天" disabled={!date || date >= today} onMouseDown={(e) => e.preventDefault()} onClick={() => onChange(shiftDate(date, 1))}><ChevronRight size={16} /></button>
  </div>
}
