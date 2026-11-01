import { useState } from 'react'
import { bytes, cn, hourLabel, niceScale, percent, timeLabel } from '@/lib/utils'
import type { Series } from '@/lib/series'

export type HourBucket = { hour: number; values: Record<string, number>; total: number; gaps: number }

/** 24 columns built from plain elements so bars, hit targets and hour labels share one grid. */
export function HourlyChart({ buckets, series, selected, current, zone, directionName, highlight, onSelect }: {
  buckets: HourBucket[]; series: Series[]; selected?: number; current?: number; zone: string; directionName: string
  highlight: string | null; onSelect: (hour: number) => void
}) {
  const [hover, setHover] = useState<number | null>(null)
  const scale = niceScale(Math.max(0, ...buckets.map((b) => b.total)))
  const hovered = buckets.find((b) => b.hour === hover)
  const hoverIndex = hovered ? buckets.indexOf(hovered) : -1
  return <div className="hchart" onMouseLeave={() => setHover(null)}>
    <div className="hchart-axis" aria-hidden>{scale.ticks.map((t) => <span key={t} style={{ bottom: `${t / scale.max * 100}%` }}>{bytes(t)}</span>)}</div>
    <div className="hchart-plot">
      <div className="hchart-grid" aria-hidden>{scale.ticks.map((t) => <i key={t} style={{ bottom: `${t / scale.max * 100}%` }} />)}</div>
      <div className="hchart-cols" role="group" aria-label="每小时各节点流量，选择一个小时查看明细">
        {buckets.map((b) => {
          const future = current !== undefined && b.hour > current
          return <button key={b.hour} type="button" aria-pressed={b.hour === selected} aria-label={`${timeLabel(b.hour, zone)}，${bytes(b.total)}`}
            className={cn('hcol', b.hour === selected && 'selected', b.hour === hover && 'hovered', future && 'future')}
            onMouseEnter={() => setHover(b.hour)} onFocus={() => setHover(b.hour)} onBlur={() => setHover(null)}
            onMouseDown={(e) => e.preventDefault()} onClick={() => onSelect(b.hour)}>
            <span className="hcol-bar" style={{ height: `${b.total / scale.max * 100}%` }}>
              {series.map((s) => b.values[s.key] ? <span key={s.key} className={cn('hcol-seg', highlight && highlight !== s.key && 'dim')} style={{ flexGrow: b.values[s.key], background: s.color }} /> : null)}
            </span>
          </button>
        })}
      </div>
      {hovered && <div className={cn('hchart-tip', hoverIndex >= buckets.length / 2 && 'flip')} style={hoverIndex >= buckets.length / 2 ? { right: `${(1 - hoverIndex / buckets.length) * 100}%` } : { left: `${(hoverIndex + 1) / buckets.length * 100}%` }}>
        <div className="tip-head"><span>{hourLabel(hovered.hour, zone)}</span><span>{directionName}</span></div>
        {hovered.total ? series.filter((s) => hovered.values[s.key]).sort((a, b) => hovered.values[b.key] - hovered.values[a.key]).map((s) =>
          <div key={s.key} className="tip-row"><i style={{ background: s.color }} /><span>{s.name}</span><b>{bytes(hovered.values[s.key])}</b><em>{percent(hovered.values[s.key], hovered.total, 0)}</em></div>)
          : <div className="tip-empty">{current !== undefined && hovered.hour > current ? '尚未到达' : '无流量记录'}</div>}
        {hovered.total > 0 && <div className="tip-total"><span>合计</span><b>{bytes(hovered.total)}</b></div>}
        {hovered.gaps > 0 && <div className="tip-gap">含 {hovered.gaps} 段采集缺口</div>}
      </div>}
    </div>
    <div className="hchart-labels" aria-hidden>{buckets.map((b) => <span key={b.hour} className={cn(b.hour === selected && 'selected', b.hour === current && 'current', b.gaps > 0 && 'gap')}>{timeLabel(b.hour, zone).slice(0, 2)}</span>)}</div>
  </div>
}
