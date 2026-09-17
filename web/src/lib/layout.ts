/**
 * 布局打包器：把"有序序列"排布到固定列数的网格上。
 *
 * 这是本项目布局模型的核心简化（对应 docs/spec.md §3 决策 3 与 §10.2）：
 *   **顺序即布局** —— 数据里只保存一个有序数组，
 *   具体 (col,row) 由这个打包器在提交时按 12 列算出，在渲染时按当前显示列数算出。
 * 好处：
 *   - 永远不会有重叠或空洞，服务端的不变量校验天然通过
 *   - 窄屏自动折行是"换一个 cols 再打包一次"，不需要单独的响应式规则
 *   - 拖拽只需产出新的顺序（"顺延"语义天然成立）
 * 2×2 的大文件夹由 spanOf 返回 2，打包器会为它预留 2×2 的方块。
 */

export interface Placed<T> {
  item: T
  col: number
  row: number
  span: number
}

export function pack<T extends { id: string }>(
  items: readonly T[],
  cols: number,
  spanOf: (item: T) => number = () => 1,
): Placed<T>[] {
  const width = Math.max(1, cols)
  const occupied = new Set<string>()
  const out: Placed<T>[] = []

  let col = 0
  let row = 0

  for (const item of items) {
    const span = Math.min(Math.max(1, spanOf(item)), width)

    // 线性推进游标，直到找到一个 span×span 的空块
    for (;;) {
      if (col + span > width) {
        col = 0
        row++
        continue
      }
      let fits = true
      for (let dc = 0; dc < span && fits; dc++) {
        for (let dr = 0; dr < span; dr++) {
          if (occupied.has(`${col + dc},${row + dr}`)) {
            fits = false
            break
          }
        }
      }
      if (fits) break
      col++
    }

    for (let dc = 0; dc < span; dc++) {
      for (let dr = 0; dr < span; dr++) {
        occupied.add(`${col + dc},${row + dr}`)
      }
    }
    out.push({ item, col, row, span })
    col += span
  }

  return out
}

export function rowCount(placed: readonly Placed<unknown>[]): number {
  return placed.reduce((max, p) => Math.max(max, p.row + p.span), 0)
}
