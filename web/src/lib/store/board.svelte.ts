import type { Board, BoardItem, Link } from '$lib/types'

/**
 * 当前页 board 的客户端状态。
 * Svelte 5 约定：共享可变状态放在 .svelte.ts 里的 class 单例；
 * items 必须是 **普通对象**（svelte-dnd-action issue #644：带 $state 字段的 class 实例在拖拽中会复制错误）。
 */
class BoardStore {
  pageId = $state<string | null>(null)
  revision = $state(0)
  items = $state<BoardItem[]>([])
  links = $state<Link[]>([])
  loading = $state(false)
  error = $state<string | null>(null)

  /** 已占用格：2×2 文件夹占 4 格 */
  occupied = $derived.by(() => {
    const set = new Set<string>()
    for (const it of this.items) {
      const span = it.kind === 'folder' && this.folderSize(it.folder_id) === 2 ? 2 : 1
      for (let dc = 0; dc < span; dc++) {
        for (let dr = 0; dr < span; dr++) {
          set.add(`${it.col + dc},${it.row + dr}`)
        }
      }
    }
    return set
  })

  folders = $state<{ id: string; size: 1 | 2 }[]>([])

  folderSize(folderId?: string): 1 | 2 {
    if (!folderId) return 1
    return this.folders.find((f) => f.id === folderId)?.size ?? 1
  }

  link(linkId?: string): Link | undefined {
    if (!linkId) return undefined
    return this.links.find((l) => l.id === linkId)
  }

  apply(board: Board) {
    this.pageId = board.page.id
    this.revision = board.revision
    this.items = board.items
    this.links = board.links
    this.folders = board.folders.map((f) => ({ id: f.id, size: f.size }))
  }

  /** 螺旋搜索最近空位（用于被挤占项顺延；实现于 M1/M2） */
  nearestFree(col: number, row: number, span: 1 | 2, cols = 12): { col: number; row: number } {
    for (let radius = 0; radius < 64; radius++) {
      for (let dc = -radius; dc <= radius; dc++) {
        for (let dr = -radius; dr <= radius; dr++) {
          if (Math.max(Math.abs(dc), Math.abs(dr)) !== radius) continue
          const c = col + dc
          const r = row + dr
          if (c < 0 || r < 0 || c + span > cols) continue
          let ok = true
          for (let x = 0; x < span && ok; x++) {
            for (let y = 0; y < span; y++) {
              if (this.occupied.has(`${c + x},${r + y}`) && !(dc === 0 && dr === 0)) {
                ok = false
                break
              }
            }
          }
          if (ok) return { col: c, row: r }
        }
      }
    }
    return { col, row }
  }
}

export const board = new BoardStore()
