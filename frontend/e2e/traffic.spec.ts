import { test, expect } from '@playwright/test'

const start = Date.parse('2026-11-01T00:00:00+08:00') / 1000
const rows = [
  { hour: start + 12 * 3600, node: '深港专线1', domain: 'google.com', upload: 0, download: 1e9 },
  { hour: start + 12 * 3600, node: '深港专线1', domain: 'apple.com', upload: 0, download: 1e9 },
  { hour: start + 13 * 3600, node: '日本节点', domain: 'github.com', upload: 1e8, download: 4e8 },
]

test.beforeEach(async ({ page }) => {
  await page.route('**/api/status', (route) => route.fulfill({ json: {
    connected: true, last_flush: start * 1000, flush_interval_ms: 30000, retention_days: 7, storage_error: '', version: 'test', last_sample: (start + 12.5 * 3600) * 1000, started_at: start * 1000,
    active: 3, inner: 1, unknown_domain: 0, samples: 5, reconnects: 0, error: '', interval_ms: 1000,
    timezone: 'Asia/Shanghai', today: '2026-11-01', now: (start + 12.5 * 3600) * 1000,
  } }))
  await page.route('**/api/traffic?*', (route) => {
    const p = new URL(route.request().url()).searchParams
    const filtered = rows.filter((r) => (!p.get('node') || p.get('node') === r.node) && (!p.get('domain') || r.domain.includes(p.get('domain')!)))
    return route.fulfill({ json: {
      rows: p.get('date') === '2026-11-01' ? filtered : [], nodes: ['深港专线1', '日本节点'], gaps: [],
      hours: Array.from({ length: 24 }, (_, i) => start + i * 3600), date: p.get('date'), timezone: 'Asia/Shanghai',
      global_upload: 1e8, global_download: 24e8, all_upload: 1e8, all_download: 24e8,
    } })
  })
})

test('小时、节点与域名筛选保持合计一致', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText('正在采集', { exact: true })).toBeVisible()
  const detail = page.locator('.node-card')
  await expect(detail).toHaveCount(1)
  await expect(detail.getByText('深港专线1', { exact: true })).toBeVisible()
  await expect(detail.getByText('2 GB', { exact: true })).toBeVisible()
  await expect(detail.getByRole('cell', { name: 'google.com', exact: true })).toBeVisible()
  await page.getByRole('button', { name: /^13:00，/ }).click()
  await expect(detail.getByText('日本节点', { exact: true })).toBeVisible()
  await expect(detail.locator('.node-heading').getByText('500 MB', { exact: true })).toBeVisible()
  await page.getByRole('combobox', { name: '节点', exact: true }).click()
  await page.getByRole('option', { name: '深港专线1', exact: true }).click()
  await expect(page.getByText('这个小时暂无记录', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: /^12:00，/ }).click()
  await page.getByRole('textbox', { name: '搜索域名' }).fill('google.com')
  await expect(detail.getByRole('cell', { name: 'apple.com', exact: true })).toHaveCount(0)
  await expect(detail.locator('.node-heading').getByText('1 GB', { exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: '导出' })).toHaveAttribute('href', /node=.*domain=google.com/)
  await page.getByRole('radiogroup', { name: '统计流量' }).getByRole('radio', { name: '上传' }).click()
  await expect(detail.locator('.node-heading').getByText('0 B', { exact: true })).toBeVisible()
})

test('空日期与深浅主题的手机布局', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  await expect(page.locator('.node-card')).toHaveCount(1)
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  await page.getByRole('textbox', { name: '搜索域名' }).fill('nothing.example')
  await expect(page.getByText('暂无匹配流量', { exact: true })).toBeVisible()
  await page.getByRole('textbox', { name: '搜索域名' }).fill('')
  await page.getByRole('button', { name: '前一天' }).click()
  await expect(page.getByText('暂无匹配流量', { exact: true })).toBeVisible()
  expect(errors).toEqual([])
})

test('热门域名点选即筛选，再点取消', async ({ page }) => {
  await page.goto('/')
  const ranking = page.getByRole('region', { name: '当日热门域名' })
  await expect(ranking.getByRole('button')).toHaveCount(3)
  await ranking.getByRole('button', { name: /github\.com/ }).click()
  await expect(page.getByRole('textbox', { name: '搜索域名' })).toHaveValue('github.com')
  await expect(ranking.getByRole('button')).toHaveCount(1)
  await ranking.getByRole('button', { name: /github\.com/ }).click()
  await expect(page.getByRole('textbox', { name: '搜索域名' })).toHaveValue('')
  await expect(ranking.getByRole('button')).toHaveCount(3)
})
