import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Activity, ArrowDownLeft, ArrowUpRight, ChevronLeft, ChevronRight, Download, HelpCircle, RefreshCw, Search, Wifi, WifiOff } from 'lucide-react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip as ChartTooltip, XAxis, YAxis } from 'recharts'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { get, trafficParams, type Row, type Status, type Traffic } from '@/lib/api'
import { useFilters, type Direction } from '@/lib/filters'
import { bytes, cn, hourLabel, timeLabel } from '@/lib/utils'

const colors = ['#119e89', '#6d84df', '#dca454', '#ba77b9', '#5aa4c0', '#a0ae66', '#9ca3af']
function Hint({ text }: { text: string }) {
  return <Tooltip><TooltipTrigger asChild><button className="hint" aria-label={text}><HelpCircle size={14} /></button></TooltipTrigger><TooltipContent className="max-w-80">{text}</TooltipContent></Tooltip>
}
function FilterSelect({ label, value, onChange, children }: { label: string; value: string; onChange: (value: string) => void; children: React.ReactNode }) {
  return <label className="filter"><span>{label}</span><Select value={value} onValueChange={onChange}><SelectTrigger aria-label={label} className="w-full"><SelectValue /></SelectTrigger><SelectContent>{children}</SelectContent></Select></label>
}
function Metric({ label, value, hint, icon }: { label: string; value: string; hint?: string; icon?: React.ReactNode }) {
  return <Card className="metric"><CardContent><div className="metric-label">{icon}{label}{hint && <Hint text={hint} />}</div><div className="metric-value">{value}</div></CardContent></Card>
}
function Empty({ title, text }: { title: string; text: string }) {
  return <div className="empty"><Activity size={26} strokeWidth={1.3} /><strong>{title}</strong><p>{text}</p></div>
}
export default function App() {
  const f = useFilters()
  const [search, setSearch] = useState(f.search)
  useEffect(() => { const timer = setTimeout(() => f.setSearch(search), 250); return () => clearTimeout(timer) }, [search, f.setSearch])
  const statusQuery = useQuery({ queryKey: ['status'], queryFn: ({ signal }) => get<Status>('/api/status', signal) })
  const status = statusQuery.data
  useEffect(() => { if (!f.date && status) f.setDate(status.today) }, [status, f.date, f.setDate])
  const params = trafficParams(f.date, f.node, f.search, f.group)
  const trafficQuery = useQuery({ queryKey: ['traffic', params], queryFn: ({ signal }) => get<Traffic>(`/api/traffic?${params}`, signal), enabled: !!f.date })
  const traffic = trafficQuery.data
  const zone = status?.timezone || 'Asia/Shanghai'
  const value = (r: { upload: number; download: number }) => f.direction === 'total' ? r.upload + r.download : r[f.direction]
  const rows = traffic?.rows || []
  const total = rows.reduce((s, r) => ({ upload: s.upload + r.upload, download: s.download + r.download }), { upload: 0, download: 0 })
  const hours = traffic?.hours || []
  const currentHour = hours.find((h) => h <= (status?.now || Date.now()) / 1000 && h + 3600 > (status?.now || Date.now()) / 1000)
  const defaultHour = currentHour ?? [...rows].sort((a, b) => value(b) - value(a))[0]?.hour ?? hours[0]
  const selectedHour = f.hour !== null && hours.includes(f.hour) ? f.hour : defaultHour
  const selectedRows = rows.filter((r) => r.hour === selectedHour)
  const grouped = new Map<string, Row[]>()
  selectedRows.forEach((r) => grouped.set(r.node, [...(grouped.get(r.node) || []), r]))
  const groups = [...grouped.entries()].map(([node, rs]) => ({ node, rows: rs.sort((a, b) => value(b) - value(a)), total: rs.reduce((s, r) => s + value(r), 0) })).sort((a, b) => b.total - a.total)
  const { series, chartData } = useMemo(() => {
    const directionValue = (r: Row) => f.direction === 'total' ? r.upload + r.download : r[f.direction]
    const ranking = new Map<string, number>()
    rows.forEach((r) => ranking.set(r.node, (ranking.get(r.node) || 0) + directionValue(r)))
    const names = [...ranking].sort((a, b) => b[1] - a[1]).map(([n]) => n)
    const top = names.slice(0, 6)
    const series = top.map((name, i) => ({ key: `node${i}`, name, color: colors[i] }))
    if (names.length > top.length) series.push({ key: 'other', name: '其余节点', color: colors[6] })
    const lookup = new Map(top.map((name, i) => [name, `node${i}`]))
    const buckets = new Map<number, Record<string, number>>()
    rows.forEach((r) => { const b = buckets.get(r.hour) || {}; const key = lookup.get(r.node) || 'other'; b[key] = (b[key] || 0) + directionValue(r); buckets.set(r.hour, b) })
    return { series, chartData: hours.map((timestamp) => ({ timestamp, label: timeLabel(timestamp, zone), ...buckets.get(timestamp) })) }
  }, [traffic, f.direction, zone]) // API data is the authoritative server state.
  function moveDate(delta: number) {
    const date = new Date(`${f.date}T12:00:00Z`)
    if (!Number.isNaN(date.valueOf())) { date.setUTCDate(date.getUTCDate() + delta); f.setDate(date.toISOString().slice(0, 10)) }
  }
  const refreshing = trafficQuery.isFetching || statusQuery.isFetching
  const directionName = { total: '总流量', upload: '上传', download: '下载' }[f.direction]
  return <main className="page">
    <header className="page-header"><div className="brand"><span className="brand-icon"><Activity size={22} /></span><h1>Mihomo <span>流量去向</span></h1></div><div className="header-actions">
      <Tooltip><TooltipTrigger asChild><Badge variant={status?.connected ? 'secondary' : 'outline'} className={cn('source-badge', status?.connected && 'online')}>{status?.connected ? <Wifi size={12} /> : <WifiOff size={12} />}{status?.connected ? '正在采集' : status ? '采集已断开' : '连接中'}</Badge></TooltipTrigger><TooltipContent>{status?.version || '等待内核版本'} · {status?.active || 0} 个业务连接 · 已排除 {status?.inner || 0} 个内部连接</TooltipContent></Tooltip>
      <Button variant="ghost" size="icon" aria-label="刷新数据" disabled={refreshing} onClick={() => { void statusQuery.refetch(); void trafficQuery.refetch() }}><RefreshCw className={cn('size-4', refreshing && 'animate-spin')} /></Button>
    </div></header>
    {status?.error && <div role="alert" className="error-banner">{status.error}。正在自动重连；历史数据仍可查询。</div>}
    {status?.storage_error && <div role="alert" className="error-banner">{status.storage_error}</div>}
    {(trafficQuery.error || statusQuery.error) && <div role="alert" className="error-banner">{(trafficQuery.error || statusQuery.error)?.message}<Button size="sm" variant="outline" onClick={() => { void statusQuery.refetch(); void trafficQuery.refetch() }}>重试</Button></div>}
    <section className="filters" aria-label="流量筛选">
      <label className="filter date-filter"><span>日期 <small>{zone}</small></span><div className="date-controls"><Button variant="outline" size="icon" aria-label="前一天" disabled={!f.date} onClick={() => moveDate(-1)}><ChevronLeft size={16} /></Button><Input aria-label="日期" type="date" value={f.date} onChange={(e) => { if (e.target.value) f.setDate(e.target.value) }} /><Button variant="outline" size="icon" aria-label="后一天" disabled={!f.date || f.date >= (status?.today || '9999-12-31')} onClick={() => moveDate(1)}><ChevronRight size={16} /></Button></div></label>
      <FilterSelect label="节点" value={f.node} onChange={f.setNode}><SelectItem value="all">全部节点</SelectItem>{traffic?.nodes.map((n) => <SelectItem key={n} value={n}>{n}</SelectItem>)}</FilterSelect>
      <FilterSelect label="统计流量" value={f.direction} onChange={(v) => f.setDirection(v as Direction)}><SelectItem value="total">上传 + 下载</SelectItem><SelectItem value="download">仅下载</SelectItem><SelectItem value="upload">仅上传</SelectItem></FilterSelect>
      <FilterSelect label="域名归类" value={f.group} onChange={(v) => f.setGroup(v as 'domain' | 'host')}><SelectItem value="domain">主域名</SelectItem><SelectItem value="host">完整域名 / IP</SelectItem></FilterSelect>
      <label className="filter search-filter"><span>搜索域名</span><div className="search-box"><Search size={15} /><Input aria-label="搜索域名" placeholder="例如 google.com" value={search} onChange={(e) => setSearch(e.target.value)} /></div></label>
    </section>
    <section className="metrics" aria-label="流量概览">
      <Metric label={`已采集${directionName}`} value={bytes(value(total))} hint="当前筛选条件下已观察到的业务连接增量。GB = 10⁹ 字节。" />
      <Metric label="上传" value={bytes(total.upload)} icon={<ArrowUpRight size={15} />} />
      <Metric label="下载" value={bytes(total.download)} icon={<ArrowDownLeft size={15} />} />
      <Metric label="当日全局观测" value={bytes(value({ upload: traffic?.global_upload || 0, download: traffic?.global_download || 0 }))} hint="由内核全局累计值做差，跟随流量方向，不跟随节点与域名筛选。可能包含未被连接快照捕获的流量；采集缺口不补算。" />
    </section>
    <Card className="chart-card"><CardHeader className="section-heading"><div><CardTitle>每小时用量</CardTitle><span className="section-caption">按节点分布</span></div><div className="chart-actions"><Badge variant="outline">采样统计</Badge><Hint text="定期读取活跃连接快照，图表在每批落盘后更新。短连接和关闭前最后一段流量可能漏采；跨小时增量按时间比例估算。节点取业务连接报告的出站，不保证包含 dialer-proxy 的机场入口。" /><Button variant="ghost" size="sm" asChild><a href={`/api/export?${params}`} download><Download size={14} />导出</a></Button></div></CardHeader>
      <CardContent>{trafficQuery.isPending && f.date ? <Skeleton className="h-64 w-full" /> : !rows.length ? <Empty title="暂无匹配流量" text={f.node !== 'all' || f.search ? '尝试调整节点或域名筛选。' : '从采集启动后开始记录，有流量经过时自动更新。'} /> : <>
        <div className="chart" role="img" aria-label="每小时各节点流量堆叠柱状图，可通过下方小时按钮选择时段"><ResponsiveContainer width="100%" height={260}><BarChart data={chartData} barCategoryGap="28%" margin={{ left: 0, right: 8, top: 12, bottom: 0 }}>
          <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="3 5" /><XAxis dataKey="label" tick={{ fill: 'var(--muted-foreground)', fontSize: 11 }} tickLine={false} axisLine={false} interval="preserveStartEnd" minTickGap={20} /><YAxis tickFormatter={bytes} width={72} tick={{ fill: 'var(--muted-foreground)', fontSize: 11 }} tickLine={false} axisLine={false} />
          <ChartTooltip cursor={{ fill: 'var(--accent)' }} content={({ active, payload, label }) => active && payload?.length ? <div className="chart-tooltip"><strong>{label} · {directionName}</strong>{payload.map((p) => <div key={String(p.dataKey)}><span style={{ color: p.color }}>{p.name}</span><b>{bytes(Number(p.value))}</b></div>)}<div className="tooltip-total">合计 <b>{bytes(payload.reduce((s, p) => s + Number(p.value || 0), 0))}</b></div></div> : null} />
          {series.map((s) => <Bar key={s.key} name={s.name} dataKey={s.key} stackId="traffic" fill={s.color} maxBarSize={36} isAnimationActive={false} onClick={(_, i) => { if (chartData[i]) f.setHour(chartData[i].timestamp) }} cursor="pointer" />)}
        </BarChart></ResponsiveContainer></div>
        <div className="legend">{series.map((s) => <span key={s.key}><i style={{ background: s.color }} />{s.name}</span>)}</div>
      </>}</CardContent></Card>
    <section className="details-section"><div className="details-heading"><div><h2>域名明细 <span>{selectedHour !== undefined ? hourLabel(selectedHour, zone) : ''}</span></h2></div><strong>{bytes(selectedRows.reduce((s, r) => s + value(r), 0))}</strong></div>
      <div className="hour-picker" aria-label="选择小时">{hours.map((h) => <button key={h} aria-pressed={h === selectedHour} className={cn('hour-button', h === selectedHour && 'selected')} onClick={() => f.setHour(h)}>{timeLabel(h, zone)}</button>)}</div>
      {trafficQuery.isPending ? <Skeleton className="h-48 w-full" /> : !groups.length ? <Card><CardContent><Empty title="这个小时暂无记录" text="选择其他小时，或等待新的流量记录。" /></CardContent></Card> : <div className="node-tables">{groups.map((g) => <Card key={g.node} className="node-card"><CardHeader className="node-heading"><CardTitle><span className="node-dot" style={{ background: series.find((s) => s.name === g.node)?.color || colors[6] }} />{g.node}</CardTitle><strong>{bytes(g.total)}</strong></CardHeader><CardContent><Table><TableHeader><TableRow><TableHead>{f.group === 'host' ? '完整域名 / IP' : '主域名'}</TableHead><TableHead className="text-right">上传</TableHead><TableHead className="text-right">下载</TableHead><TableHead className="text-right">{directionName}</TableHead><TableHead className="text-right">节点内占比</TableHead></TableRow></TableHeader><TableBody>{g.rows.map((r) => <TableRow key={r.domain}><TableCell className="domain-cell">{r.domain}</TableCell><TableCell className="text-right tabular-nums muted">{bytes(r.upload)}</TableCell><TableCell className="text-right tabular-nums muted">{bytes(r.download)}</TableCell><TableCell className="text-right tabular-nums font-medium">{bytes(value(r))}</TableCell><TableCell className="text-right tabular-nums">{g.total ? `${(value(r) / g.total * 100).toFixed(1)}%` : '—'}</TableCell></TableRow>)}</TableBody></Table></CardContent></Card>)}</div>}
    </section>
    <footer><span>{status?.last_sample ? `最后采集 ${new Intl.DateTimeFormat('zh-CN', { timeZone: zone, hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }).format(status.last_sample)}` : '尚未收到快照'} · 每 {((status?.interval_ms || 1000) / 1000).toLocaleString('zh-CN')} 秒采样 · 每 {((status?.flush_interval_ms || 30000) / 1000).toLocaleString('zh-CN')} 秒落盘 · 保留 {status?.retention_days || 7} 天</span><div>{!!traffic?.gaps.length && <Tooltip><TooltipTrigger asChild><button className="gap-note">当日 {traffic.gaps.length} 段采集缺口</button></TooltipTrigger><TooltipContent className="max-w-96">{traffic.gaps.slice(-6).map((g, i) => <p key={i}>{new Date(g.start).toLocaleTimeString('zh-CN', { timeZone: zone })}–{new Date(g.end).toLocaleTimeString('zh-CN', { timeZone: zone })}：{g.reason}</p>)}</TooltipContent></Tooltip>}{!!status?.unknown_domain && <span>{status.unknown_domain} 个活跃连接未识别域名</span>}</div></footer>
  </main>
}
