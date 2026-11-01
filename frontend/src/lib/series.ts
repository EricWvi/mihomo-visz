export type Series = { key: string; name: string; color: string; total: number }

const slots = 8
export const otherColor = 'var(--series-other)'

/**
 * Color follows the node, not its rank: with up to eight nodes each keeps the slot of its
 * position in the day's node list, so filters never repaint the survivors. Beyond that the
 * seven busiest nodes keep slots and the rest fold into “其余节点”.
 */
export function buildSeries(nodes: string[], totals: Map<string, number>) {
  const names = [...totals.keys()].sort((a, b) => totals.get(b)! - totals.get(a)!)
  const fold = nodes.length > slots
  const shown = fold ? names.slice(0, slots - 1) : names
  const order = fold ? [...shown].sort((a, b) => nodes.indexOf(a) - nodes.indexOf(b)) : nodes
  const series: Series[] = shown.map((name) => {
    const index = order.indexOf(name)
    return { key: `n:${name}`, name, color: index >= 0 && index < slots ? `var(--series-${index + 1})` : otherColor, total: totals.get(name)! }
  })
  const rest = names.slice(shown.length)
  if (rest.length) series.push({ key: 'other', name: `其余 ${rest.length} 个节点`, color: otherColor, total: rest.reduce((s, n) => s + totals.get(n)!, 0) })
  const keyOf = (node: string) => shown.includes(node) ? `n:${node}` : 'other'
  return { series, keyOf }
}
