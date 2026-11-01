import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, ArrowUpRight, ChevronLeft, ChevronRight, CircleAlert, Download, Flame, Globe, HelpCircle, Network, RefreshCw, Search, SearchX, Telescope, X } from 'lucide-react'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Segmented } from '@/components/ui/segmented'
import { Skeleton } from '@/components/ui/skeleton'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { DateStrip } from '@/components/DateStrip'
import { HourlyChart, type HourBucket } from '@/components/HourlyChart'
import { get, trafficParams, type Row, type Status, type Traffic } from '@/lib/api'
import { useFilters, type Direction } from '@/lib/filters'
import { buildSeries, otherColor, type Series } from '@/lib/series'
import { ago, byteParts, bytes, cn, hourLabel, longDate, percent, timeLabel } from '@/lib/utils'

type Amount = { upload: number; download: number }
const directionNames: Record<Direction, string> = { total: '总流量', upload: '上传', download: '下载' }
const noFocus = (e: React.MouseEvent) => e.preventDefault()

function Hint({ text }: { text: string }) {
  return <Tooltip><TooltipTrigger asChild><button type="button" className="hint" aria-label={text} onMouseDown={noFocus}><HelpCircle size={13} /></button></TooltipTrigger><TooltipContent className="max-w-80 leading-relaxed">{text}</TooltipContent></Tooltip>
}
function Big({ value, className }: { value: number; className?: string }) {
  const [n, unit] = byteParts(value)
  return <span className={cn('big', className)}>{n}<small>{unit}</small></span>
}
function Empty({ icon, title, text }: { icon: React.ReactNode; title: string; text: string }) {
  return <div className="empty"><span className="empty-icon">{icon}</span><strong>{title}</strong><p>{text}</p></div>
}
function Sparkline({ values }: { values: number[] }) {
  const max = Math.max(...values, 1)
  if (values.length < 2) return null
  const pts = values.map((v, i) => [i / (values.length - 1) * 100, 38 - v / max * 34] as const)
  const line = pts.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(2)},${y.toFixed(2)}`).join('')
  return <svg className="sparkline" viewBox="0 0 100 40" preserveAspectRatio="none" aria-hidden>
    <defs><linearGradient id="spark-fill" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="var(--primary)" stopOpacity=".28" /><stop offset="1" stopColor="var(--primary)" stopOpacity="0" /></linearGradient></defs>
    <path d={`${line}L100,40L0,40Z`} fill="url(#spark-fill)" /><path d={line} fill="none" stroke="var(--primary)" strokeWidth="1.5" vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
  </svg>
}

function StatusPill({ status, failed }: { status?: Status; failed: boolean }) {
  const [now, setNow] = useState(Date.now())
  useEffect(() => { const t = setInterval(() => setNow(Date.now()), 1000); return () => clearInterval(t) }, [])
  const state = failed ? 'down' : !status ? 'wait' : status.connected ? 'live' : 'down'
  return <Tooltip><TooltipTrigger asChild><button type="button" className={cn('status-pill', state)} onMouseDown={noFocus}>
    <span className="pulse" /><span>{state === 'live' ? '正在采集' : state === 'wait' ? '连接中' : '采集已断开'}</span>
    {status?.version && <span className="status-extra">{status.version}</span>}
    {status?.connected && <span className="status-extra">{status.active} 个连接</span>}
    {!!status?.last_sample && <span className="status-extra">{ago(now - status.last_sample)}</span>}
  </button></TooltipTrigger><TooltipContent className="leading-relaxed">
    内核 {status?.version || '未知'} · {status?.active || 0} 个业务连接 · 已排除 {status?.inner || 0} 个内部连接<br />
    本次启动以来 {status?.samples?.toLocaleString('zh-CN') || 0} 次采样{status?.reconnects ? ` · 重连 ${status.reconnects} 次` : ''}
  </TooltipContent></Tooltip>
}

export default function App() {
  const f = useFilters()
  const [search, setSearch] = useState(f.search)
  const [highlight, setHighlight] = useState<string | null>(null)
  useEffect(() => { const timer = setTimeout(() => f.setSearch(search), 250); return () => clearTimeout(timer) }, [search, f.setSearch])
  const statusQuery = useQuery({ queryKey: ['status'], queryFn: ({ signal }) => get<Status>('/api/status', signal) })
  const status = statusQuery.data
  useEffect(() => { if (!f.date && status) f.setDate(status.today) }, [status, f.date, f.setDate])
  const params = trafficParams(f.date, f.node, f.search, f.group)
  const trafficQuery = useQuery({ queryKey: ['traffic', params], queryFn: ({ signal }) => get<Traffic>(`/api/traffic?${params}`, signal), enabled: !!f.date })
  const traffic = trafficQuery.data
  const zone = status?.timezone || 'Asia/Shanghai'
  const value = (r: Amount) => f.direction === 'total' ? r.upload + r.download : r[f.direction]
  const directionName = directionNames[f.direction]
  const rows = traffic?.rows || []
  const hours = traffic?.hours || []
  const nowSec = (status?.now || Date.now()) / 1000
  const currentHour = hours.find((h) => h <= nowSec && h + 3600 > nowSec)

  const view = useMemo(() => {
    const val = (r: Amount) => f.direction === 'total' ? r.upload + r.download : r[f.direction]
    const total = { upload: 0, download: 0 }
    const nodeTotals = new Map<string, number>()
    const domains = new Map<string, Amount & { nodes: Set<string> }>()
    rows.forEach((r) => {
      total.upload += r.upload; total.download += r.download
      nodeTotals.set(r.node, (nodeTotals.get(r.node) || 0) + val(r))
      const d = domains.get(r.domain) || { upload: 0, download: 0, nodes: new Set<string>() }
      d.upload += r.upload; d.download += r.download; d.nodes.add(r.node); domains.set(r.domain, d)
    })
    const { series, keyOf } = buildSeries(traffic?.nodes || [], nodeTotals)
    const buckets: HourBucket[] = hours.map((hour) => ({ hour, values: {}, total: 0, gaps: (traffic?.gaps || []).filter((g) => g.start < (hour + 3600) * 1000 && g.end > hour * 1000).length }))
    const index = new Map(hours.map((h, i) => [h, i]))
    rows.forEach((r) => { const b = buckets[index.get(r.hour) ?? -1]; if (!b) return; const k = keyOf(r.node); b.values[k] = (b.values[k] || 0) + val(r); b.total += val(r) })
    const topDomains = [...domains].map(([domain, d]) => ({ domain, value: val(d), nodes: [...d.nodes] })).filter((d) => d.value > 0).sort((a, b) => b.value - a.value)
    return { total, series, keyOf, buckets, topDomains, domainCount: domains.size, nodeCount: nodeTotals.size }
  }, [traffic, f.direction]) // API data is the authoritative server state.

  const { total, series, keyOf, buckets } = view
  const dayValue = value(total)
  const peak = buckets.reduce<HourBucket | undefined>((p, b) => b.total > (p?.total || 0) ? b : p, undefined)
  const defaultHour = currentHour ?? peak?.hour ?? hours[0]
  const selectedHour = f.hour !== null && hours.includes(f.hour) ? f.hour : defaultHour
  const selectedRows = rows.filter((r) => r.hour === selectedHour)
  const grouped = new Map<string, Row[]>()
  selectedRows.forEach((r) => grouped.set(r.node, [...(grouped.get(r.node) || []), r]))
  const groups = [...grouped.entries()].map(([node, rs]) => ({ node, rows: rs.sort((a, b) => value(b) - value(a)), total: rs.reduce((s, r) => s + value(r), 0) })).sort((a, b) => b.total - a.total)
  const hourTotal = groups.reduce((s, g) => s + g.total, 0)
  const colorOf = (node: string) => series.find((s) => s.key === keyOf(node))?.color || otherColor
  const global = value({ upload: traffic?.global_upload || 0, download: traffic?.global_download || 0 })
  const filtered = f.node !== 'all' || !!f.search
  const loading = trafficQuery.isPending
  const refreshing = trafficQuery.isFetching || statusQuery.isFetching
  const refetch = () => { void statusQuery.refetch(); void trafficQuery.refetch() }
  const pickDomain = (domain: string) => { const next = f.search === domain ? '' : domain; setSearch(next); f.setSearch(next) }
  const sparkValues = buckets.filter((b) => currentHour === undefined || b.hour <= currentHour).map((b) => b.total)
  const hourIndex = selectedHour === undefined ? -1 : hours.indexOf(selectedHour)

  return <div className="shell">
    <header className="topbar"><div className="topbar-inner">
      <div className="brand"><img src="/favicon.svg" alt="" width={30} height={30} /><div><h1>Mihomo Visz</h1><p>流量去向</p></div></div>
      <div className="topbar-actions">
        <StatusPill status={status} failed={!!statusQuery.error} />
        <Tooltip><TooltipTrigger asChild><button type="button" className="icon-btn" aria-label="刷新数据" disabled={refreshing} onMouseDown={noFocus} onClick={refetch}><RefreshCw size={15} className={cn(refreshing && 'animate-spin')} /></button></TooltipTrigger><TooltipContent>刷新（每 5 秒自动刷新）</TooltipContent></Tooltip>
      </div>
    </div></header>

    <main className="page">
      {status?.error && <div role="alert" className="banner"><CircleAlert size={16} /><span>{status.error}。正在自动重连；历史数据仍可查询。</span></div>}
      {status?.storage_error && <div role="alert" className="banner"><CircleAlert size={16} /><span>{status.storage_error}</span></div>}
      {(trafficQuery.error || statusQuery.error) && <div role="alert" className="banner"><CircleAlert size={16} /><span>{(trafficQuery.error || statusQuery.error)?.message}</span><button type="button" className="banner-action" onClick={refetch}>重试</button></div>}

      <section className="toolbar" aria-label="流量筛选">
        <DateStrip date={f.date} today={status?.today || ''} days={7} onChange={f.setDate} />
        <div className="toolbar-filters">
          <Select value={f.node} onValueChange={f.setNode}><SelectTrigger aria-label="节点" className="node-select"><Network size={14} /><SelectValue /></SelectTrigger><SelectContent position="popper" align="start">
            <SelectItem value="all">全部节点</SelectItem>{traffic?.nodes.map((n) => <SelectItem key={n} value={n}><i className="swatch" style={{ background: colorOf(n) }} />{n}</SelectItem>)}
          </SelectContent></Select>
          <Segmented label="统计流量" value={f.direction} onChange={f.setDirection} options={[{ value: 'total', label: '总量' }, { value: 'download', label: <><ArrowDown size={13} />下载</>, title: '仅下载' }, { value: 'upload', label: <><ArrowUp size={13} />上传</>, title: '仅上传' }]} />
          <Segmented label="域名归类" value={f.group} onChange={f.setGroup} options={[{ value: 'domain', label: '主域名' }, { value: 'host', label: '完整域名' }]} />
          <div className="search-box"><Search size={14} /><input aria-label="搜索域名" placeholder="搜索域名" value={search} onChange={(e) => setSearch(e.target.value)} />{search && <button type="button" aria-label="清除搜索" onMouseDown={noFocus} onClick={() => { setSearch(''); f.setSearch('') }}><X size={13} /></button>}</div>
        </div>
      </section>

      <section className="overview" aria-label="流量概览">
        <div className="card hero">
          <div className="hero-top"><span className="label">已采集{directionName}<Hint text="当前筛选条件下已观察到的业务连接增量。GB = 10⁹ 字节。" /></span><span className="eyebrow">{f.date ? longDate(f.date) : '—'}</span></div>
          <div className="hero-main">{loading ? <Skeleton className="h-14 w-56" /> : <Big value={dayValue} className="hero-value" />}<Sparkline values={sparkValues} /></div>
          <div className="split" role="img" aria-label={`上传 ${bytes(total.upload)}，下载 ${bytes(total.download)}`}>
            <div className="split-bar">{total.upload + total.download > 0 && <><span className="up" style={{ flexGrow: total.upload }} /><span className="down" style={{ flexGrow: total.download }} /></>}</div>
            <div className="split-legend">
              <span><i className="up" /><ArrowUp size={12} />上传 <b>{bytes(total.upload)}</b><em>{percent(total.upload, total.upload + total.download, 0)}</em></span>
              <span><i className="down" /><ArrowDown size={12} />下载 <b>{bytes(total.download)}</b><em>{percent(total.download, total.upload + total.download, 0)}</em></span>
            </div>
          </div>
        </div>
        <button type="button" className="card tile tile-action" disabled={!peak} onMouseDown={noFocus} onClick={() => peak && f.setHour(peak.hour)}>
          <span className="label"><Flame size={14} />峰值时段<ArrowUpRight size={14} className="tile-go" /></span>
          <span className="tile-value">{peak ? hourLabel(peak.hour, zone) : '—'}</span>
          <span className="tile-foot">{peak ? <>{bytes(peak.total)} · 占当日 {percent(peak.total, dayValue, 0)}</> : '暂无流量'}</span>
        </button>
        <div className="card tile">
          <span className="label"><Telescope size={14} />当日全局观测<Hint text="由内核全局累计值做差，跟随流量方向，不跟随节点与域名筛选。可能包含未被连接快照捕获的流量；采集缺口不补算。" /></span>
          <Big value={global} className="tile-value" />
          <div className="meter" aria-label={`已采集占全局 ${percent(dayValue, global, 0)}`}><span style={{ width: `${global ? Math.min(100, dayValue / global * 100) : 0}%` }} /></div>
          <span className="tile-foot">{filtered ? '筛选结果' : '已采集'}占全局 {percent(dayValue, global, 0)}</span>
        </div>
        <div className="card tile">
          <span className="label"><Globe size={14} />覆盖范围</span>
          <span className="tile-value tile-count"><b>{view.nodeCount}</b><small>个节点</small><b>{view.domainCount}</b><small>{f.group === 'host' ? '个主机' : '个域名'}</small></span>
          <span className="tile-foot">{traffic?.gaps.length ? <Tooltip><TooltipTrigger asChild><button type="button" className="gap-note" onMouseDown={noFocus}><CircleAlert size={12} />{traffic.gaps.length} 段采集缺口</button></TooltipTrigger><TooltipContent className="max-w-96 leading-relaxed">{traffic.gaps.slice(-6).map((g, i) => <p key={i}>{new Date(g.start).toLocaleTimeString('zh-CN', { timeZone: zone, hourCycle: 'h23' })}–{new Date(g.end).toLocaleTimeString('zh-CN', { timeZone: zone, hourCycle: 'h23' })}　{g.reason}</p>)}</TooltipContent></Tooltip> : traffic ? '当日采集连续' : '—'}</span>
        </div>
      </section>

      <section className="card chart-card">
        <div className="card-head">
          <div><h2>每小时{directionName}</h2><p>按节点堆叠 · 点击柱子查看该时段明细</p></div>
          <div className="card-actions"><span className="tag">采样统计</span><Hint text="定期读取活跃连接快照，图表在每批落盘后更新。短连接和关闭前最后一段流量可能漏采；跨小时增量按时间比例估算。节点取业务连接报告的出站，不保证包含 dialer-proxy 的机场入口。" /><a className="ghost-btn" href={`/api/export?${params}`} download><Download size={14} />导出</a></div>
        </div>
        {loading ? <Skeleton className="h-72 w-full" /> : !rows.length ? <Empty icon={<SearchX size={22} />} title="暂无匹配流量" text={filtered ? '尝试调整节点或域名筛选。' : '从采集启动后开始记录，有流量经过时自动更新。'} /> :
          <div className="chart-body">
            <HourlyChart buckets={buckets} series={series} selected={selectedHour} current={currentHour} zone={zone} directionName={directionName} highlight={highlight} onSelect={f.setHour} />
            <NodeLegend series={series} total={dayValue} highlight={highlight} onHighlight={setHighlight} />
          </div>}
      </section>

      <div className="lower">
        <section className="card detail" aria-label="时段明细">
          <div className="card-head detail-head">
            <div><p className="eyebrow">时段明细</p><h2 className="detail-hour">{selectedHour !== undefined ? hourLabel(selectedHour, zone) : '—'}{selectedHour === currentHour && selectedHour !== undefined && <span className="now-tag">进行中</span>}</h2></div>
            <div className="detail-summary">
              <div className="detail-total"><b>{bytes(hourTotal)}</b><span>占当日 {percent(hourTotal, dayValue, 0)}</span></div>
              <div className="stepper">
                <button type="button" className="icon-btn" aria-label="上一小时" disabled={hourIndex <= 0} onMouseDown={noFocus} onClick={() => f.setHour(hours[hourIndex - 1])}><ChevronLeft size={16} /></button>
                <button type="button" className="icon-btn" aria-label="下一小时" disabled={hourIndex < 0 || hourIndex >= hours.length - 1} onMouseDown={noFocus} onClick={() => f.setHour(hours[hourIndex + 1])}><ChevronRight size={16} /></button>
              </div>
            </div>
          </div>
          {loading ? <Skeleton className="h-48 w-full" /> : !groups.length ? <Empty icon={<Telescope size={22} />} title="这个小时暂无记录" text="选择其他小时，或等待新的流量记录。" /> :
            <div className="node-list">{groups.map((g) => <NodeGroup key={g.node} group={g} color={colorOf(g.node)} hourTotal={hourTotal} value={value} directionName={directionName} hostMode={f.group === 'host'} />)}</div>}
        </section>
        <TopDomains items={view.topDomains} total={dayValue} colorOf={colorOf} active={f.search} onPick={pickDomain} loading={loading} />
      </div>

      <footer>
        <span>{status?.last_sample ? `最后采集 ${new Intl.DateTimeFormat('zh-CN', { timeZone: zone, hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }).format(status.last_sample)}` : '尚未收到快照'}</span>
        <span>每 {((status?.interval_ms || 1000) / 1000).toLocaleString('zh-CN')} 秒采样</span>
        <span>每 {((status?.flush_interval_ms || 30000) / 1000).toLocaleString('zh-CN')} 秒落盘</span>
        <span>保留 {status?.retention_days || 7} 天</span>
        <span>{zone}</span>
        {!!status?.unknown_domain && <span>{status.unknown_domain} 个活跃连接未识别域名</span>}
      </footer>
    </main>
  </div>
}

function NodeLegend({ series, total, highlight, onHighlight }: { series: Series[]; total: number; highlight: string | null; onHighlight: (key: string | null) => void }) {
  return <ol className="legend" aria-label="节点排行" onMouseLeave={() => onHighlight(null)}>
    {[...series].sort((a, b) => b.total - a.total).map((s) => <li key={s.key} className={cn(highlight && highlight !== s.key && 'dim')} onMouseEnter={() => onHighlight(s.key)}>
      <div className="legend-row"><i style={{ background: s.color }} /><span className="legend-name" title={s.name}>{s.name}</span><b>{bytes(s.total)}</b></div>
      <div className="legend-bar"><span style={{ width: `${total ? s.total / total * 100 : 0}%`, background: s.color }} /></div>
    </li>)}
  </ol>
}

function NodeGroup({ group, color, hourTotal, value, directionName, hostMode }: { group: { node: string; rows: Row[]; total: number }; color: string; hourTotal: number; value: (r: Amount) => number; directionName: string; hostMode: boolean }) {
  const top = Math.max(...group.rows.map(value), 1)
  return <div className="node-card">
    <div className="node-heading">
      <span className="node-title"><i style={{ background: color }} />{group.node}</span>
      <span className="node-share">{percent(group.total, hourTotal, 0)}</span>
      <strong>{bytes(group.total)}</strong>
    </div>
    <div className="table-wrap"><table>
      <thead><tr><th>{hostMode ? '完整域名 / IP' : '主域名'}</th><th className="num updown">上传</th><th className="num updown">下载</th><th className="num">{directionName}</th><th className="num share-col">占比</th></tr></thead>
      <tbody>{group.rows.map((r) => <tr key={r.domain}>
        <td className={cn('domain-cell', r.domain.startsWith('未识别') && 'unknown')}>{r.domain}<span className="row-sub"><ArrowUp size={10} />{bytes(r.upload)}<ArrowDown size={10} />{bytes(r.download)}</span><span className="row-bar" style={{ width: `${value(r) / top * 100}%`, background: color }} /></td>
        <td className="num muted updown">{bytes(r.upload)}</td><td className="num muted updown">{bytes(r.download)}</td>
        <td className="num strong">{bytes(value(r))}</td><td className="num share-col">{percent(value(r), group.total)}</td>
      </tr>)}</tbody>
    </table></div>
  </div>
}

function TopDomains({ items, total, colorOf, active, onPick, loading }: { items: { domain: string; value: number; nodes: string[] }[]; total: number; colorOf: (node: string) => string; active: string; onPick: (domain: string) => void; loading: boolean }) {
  const top = items.slice(0, 12)
  return <section className="card top-domains" aria-label="当日热门域名">
    <div className="card-head"><div><p className="eyebrow">当日排行</p><h2>热门域名</h2></div>{items.length > top.length && <span className="tag">共 {items.length} 个</span>}</div>
    {loading ? <Skeleton className="h-64 w-full" /> : !top.length ? <Empty icon={<Globe size={22} />} title="暂无域名" text="当日还没有可统计的域名流量。" /> :
      <ol>{top.map((d, i) => <li key={d.domain}><button type="button" className={cn('rank-row', active === d.domain && 'active')} aria-pressed={active === d.domain} title={active === d.domain ? '取消筛选' : '只看这个域名'} onMouseDown={noFocus} onClick={() => onPick(d.domain)}>
        <span className="rank">{i + 1}</span>
        <span className="rank-main"><span className="rank-name"><span className={cn(d.domain.startsWith('未识别') && 'unknown')}>{d.domain}</span><span className="rank-nodes">{d.nodes.slice(0, 4).map((n) => <i key={n} title={n} style={{ background: colorOf(n) }} />)}</span></span><span className="rank-bar"><span style={{ width: `${d.value / top[0].value * 100}%` }} /></span></span>
        <span className="rank-value"><b>{bytes(d.value)}</b><em>{percent(d.value, total, 0)}</em></span>
      </button></li>)}</ol>}
  </section>
}
