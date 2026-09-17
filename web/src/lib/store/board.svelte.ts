import { api } from '$lib/api'
import { uuidv7 } from '$lib/id'
import { pack } from '$lib/layout'
import { ui } from '$lib/store/ui.svelte'
import type {
  Board,
  BoardItem,
  BoardPayload,
  Bootstrap,
  Folder,
  Item,
  Link,
  Page,
  SearchEngine,
  Settings,
} from '$lib/types'

/** 逻辑列数（与服务端 internal/nav.GridCols 必须一致） */
export const GRID_COLS = 12

const PALETTE = [
  '#3B82F6', '#8B5CF6', '#EC4899', '#F97316', '#EAB308',
  '#22C55E', '#14B8A6', '#06B6D4', '#6366F1', '#EF4444',
]

export function hostOf(rawURL: string): string {
  try {
    return new URL(rawURL).hostname.replace(/^www\./, '')
  } catch {
    return rawURL
  }
}

export function monogramColor(rawURL: string): string {
  const host = hostOf(rawURL)
  let h = 0
  for (let i = 0; i < host.length; i++) h = (h * 31 + host.charCodeAt(i)) >>> 0
  return PALETTE[h % PALETTE.length]
}

export function initialOf(link: Link): string {
  const source = link.mono_text || link.title || hostOf(link.url)
  return source.slice(0, 1).toUpperCase()
}

class BoardStore {
  pages = $state<Page[]>([])
  page = $state<Page | null>(null)
  revision = $state(0)
  /** 顺序即布局：数组顺序由打包器换算成 (col,row) */
  sequence = $state<Item[]>([])
  links = $state<Record<string, Link>>({})
  folders = $state<Record<string, Folder>>({})
  settings = $state<Settings>({})
  engines = $state<SearchEngine[]>([])
  status = $state<'idle' | 'loading' | 'saving'>('loading')
  lastError = $state<string | null>(null)
  booted = $state(false)

  #newLinks = new Set<string>()
  #newFolders = new Set<string>()
  #deletedLinks = new Set<string>()
  #deletedFolders = new Set<string>()

  async boot() {
    this.status = 'loading'
    try {
      const data = await api.get<Bootstrap>('/api/bootstrap')
      this.pages = data.pages
      this.settings = data.settings
      this.engines = data.engines

      const wanted = this.#pageFromHash() ?? this.pages[0]
      if (wanted) await this.selectPage(wanted.id)
      this.booted = true
    } catch (err) {
      this.lastError = err instanceof Error ? err.message : String(err)
      ui.error('初始化失败：' + this.lastError)
    } finally {
      this.status = 'idle'
    }
  }

  #pageFromHash(): Page | undefined {
    const m = /^#\/p\/([^/?]+)/.exec(location.hash)
    if (!m) return undefined
    const key = decodeURIComponent(m[1])
    return this.pages.find((p) => p.slug === key || p.id === key)
  }

  async selectPage(id: string) {
    this.status = 'loading'
    try {
      const board = await api.get<Board>(`/api/pages/${id}/board`)
      this.applyBoard(board)
      const target = `#/p/${board.page.slug}`
      if (location.hash !== target) history.replaceState(null, '', target)
    } catch (err) {
      this.lastError = err instanceof Error ? err.message : String(err)
      ui.error('打开页面失败：' + this.lastError)
    } finally {
      this.status = 'idle'
    }
  }

  /** 服务端是唯一事实源：把 board 转成"有序序列"（按 row,col 排序） */
  applyBoard(board: Board) {
    this.page = board.page
    this.revision = board.revision
    const sorted: BoardItem[] = [...board.items].sort((a, b) => a.row - b.row || a.col - b.col)
    this.sequence = sorted.map((it) => ({
      id: it.id,
      kind: it.kind,
      link_id: it.link_id,
      folder_id: it.folder_id,
      size: it.size ?? 1,
      children: it.children ?? [],
    }))
    this.links = Object.fromEntries(board.links.map((l) => [l.id, l]))
    this.folders = Object.fromEntries(board.folders.map((f) => [f.id, f]))
    this.#newLinks.clear()
    this.#newFolders.clear()
    this.#deletedLinks.clear()
    this.#deletedFolders.clear()
  }

  async reload() {
    if (!this.page) return
    const board = await api.get<Board>(`/api/pages/${this.page.id}/board`)
    this.applyBoard(board)
  }

  // ---------- 变更（全部走 commit：乐观应用 → PUT → 失败则回读服务器状态） ----------

  async addLink(url: string, title: string) {
    const trimmed = url.trim()
    if (!trimmed) return
    const linkId = uuidv7()
    const itemId = uuidv7()
    this.links[linkId] = {
      id: linkId,
      title: title.trim() || hostOf(trimmed),
      url: trimmed,
      open_new_tab: true,
      icon_source: 'auto',
      icon_path: null,
      icon_status: 'pending', // M5 接入抓取后由服务端更新
      mono_text: null,
      mono_color: monogramColor(trimmed),
      mono_font_size: 30,
    }
    this.#newLinks.add(linkId)
    this.sequence = [...this.sequence, { id: itemId, kind: 'link', link_id: linkId, size: 1, children: [] }]
    await this.commit()
  }

  async removeItem(itemId: string) {
    const item = this.sequence.find((i) => i.id === itemId)
    if (!item) return
    this.sequence = this.sequence.filter((i) => i.id !== itemId)

    if (item.link_id) {
      if (this.#newLinks.has(item.link_id)) this.#newLinks.delete(item.link_id)
      else this.#deletedLinks.add(item.link_id)
    }
    if (item.folder_id) {
      if (this.#newFolders.has(item.folder_id)) this.#newFolders.delete(item.folder_id)
      else this.#deletedFolders.add(item.folder_id)
      // 夹内链接随夹一起消失
      for (const child of item.children) {
        if (this.#newLinks.has(child.link_id)) this.#newLinks.delete(child.link_id)
        else this.#deletedLinks.add(child.link_id)
      }
    }
    await this.commit()
  }

  async reorder(orderedIds: string[]) {
    const byId = new Map(this.sequence.map((i) => [i.id, i]))
    const next = orderedIds.map((id) => byId.get(id)).filter((x): x is Item => Boolean(x))
    if (next.length !== this.sequence.length) return // 顺序不完整时忽略
    this.sequence = next
    await this.commit()
  }

  private async commit() {
    if (!this.page) return
    const placed = pack(this.sequence, GRID_COLS, (i) => (i.kind === 'folder' ? (i.size ?? 1) : 1))
    const payload: BoardPayload = {
      revision: this.revision,
      items: placed.map((p) => ({
        id: p.item.id,
        kind: p.item.kind,
        link_id: p.item.link_id,
        folder_id: p.item.folder_id,
        size: p.item.kind === 'folder' ? p.span : undefined,
        col: p.col,
        row: p.row,
        children: p.item.children,
      })),
      new_links: [...this.#newLinks].map((id) => ({
        id,
        url: this.links[id]?.url ?? '',
        title: this.links[id]?.title ?? '',
      })),
      new_folders: [...this.#newFolders].map((id) => ({
        id,
        name: this.folders[id]?.name ?? null,
        size: this.folders[id]?.size ?? 1,
      })),
      deleted_link_ids: [...this.#deletedLinks],
      deleted_folder_ids: [...this.#deletedFolders],
    }

    this.status = 'saving'
    try {
      const board = await api.put<Board>(`/api/pages/${this.page.id}/board`, payload)
      this.applyBoard(board)
      this.lastError = null
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      this.lastError = message
      ui.error('保存失败，已回到服务器状态：' + message)
      // 回读是唯一永远正确的回滚方式（服务端才有权威 revision）
      try {
        await this.reload()
      } catch {
        /* 回读也失败时保留本地状态，toast 已经提示 */
      }
    } finally {
      this.status = 'idle'
    }
  }

  // ---------- 查询辅助 ----------

  linkOf(item: Item | undefined): Link | undefined {
    return item?.link_id ? this.links[item.link_id] : undefined
  }

  setSetting(key: string, value: string) {
    this.settings[key] = value
  }

  get defaultEngine(): SearchEngine | undefined {
    const id = this.settings['default_engine_id']
    return this.engines.find((e) => e.id === id) ?? this.engines[0]
  }
}

export const board = new BoardStore()
