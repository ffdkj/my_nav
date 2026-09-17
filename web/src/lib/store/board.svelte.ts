import { api } from '$lib/api'
import { uuidv7 } from '$lib/id'
import { pack } from '$lib/layout'
import { ui } from '$lib/store/ui.svelte'
import type {
  Board,
  BoardItem,
  BoardPayload,
  Bootstrap,
  Child,
  Folder,
  Item,
  Link,
  Page,
  SearchEngine,
  Settings,
} from '$lib/types'

/** 逻辑列数（与服务端 internal/nav.GridCols 必须一致） */
export const GRID_COLS = 12
/** 单个文件夹容量（与服务端 internal/nav.MaxFolderItems 一致） */
export const MAX_FOLDER_ITEMS = 9

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
  /** 名字/尺寸被改过的既有文件夹（new_folders 是 upsert，见 nav/service.go） */
  #touchedFolders = new Set<string>()

  // ---------- 载入 ----------

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
    this.#touchedFolders.clear()
  }

  async reload() {
    if (!this.page) return
    const board = await api.get<Board>(`/api/pages/${this.page.id}/board`)
    this.applyBoard(board)
  }

  // ---------- 基础变更 ----------

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
    const item = this.itemById(itemId)
    if (!item) return
    this.#markDeleted(item)
    this.sequence = this.sequence.filter((i) => i.id !== itemId)
    await this.commit()
  }

  #markDeleted(item: Item) {
    if (item.link_id) {
      if (this.#newLinks.has(item.link_id)) this.#newLinks.delete(item.link_id)
      else this.#deletedLinks.add(item.link_id)
    }
    if (item.folder_id) {
      if (this.#newFolders.has(item.folder_id)) this.#newFolders.delete(item.folder_id)
      else this.#deletedFolders.add(item.folder_id)
      this.#touchedFolders.delete(item.folder_id)
    }
    for (const child of item.children) {
      if (this.#newLinks.has(child.link_id)) this.#newLinks.delete(child.link_id)
      else this.#deletedLinks.add(child.link_id)
    }
  }

  async reorder(orderedIds: string[]) {
    const byId = new Map(this.sequence.map((i) => [i.id, i]))
    const next = orderedIds.map((id) => byId.get(id)).filter((x): x is Item => Boolean(x))
    if (next.length !== this.sequence.length) return
    this.sequence = next
    await this.commit()
  }

  // ---------- 文件夹 ----------

  itemById(id: string): Item | undefined {
    return this.sequence.find((i) => i.id === id)
  }

  folderOf(item: Item | undefined): Folder | undefined {
    return item?.folder_id ? this.folders[item.folder_id] : undefined
  }

  childLinks(item: Item): Link[] {
    return item.children
      .slice()
      .sort((a, b) => a.sort_order - b.sort_order)
      .map((c) => this.links[c.link_id])
      .filter((l): l is Link => Boolean(l))
  }

  /**
   * 合并：把 source 拖到 target 上（悬停达阈值后松手）。
   * - link + link  → 新建文件夹，target 在前、source 在后（与 iOS 一致）
   * - link + folder→ 链接进入该夹
   * - folder + link→ 该链接进入这个夹（夹位置不变）
   * - folder+folder→ 拒绝（不支持嵌套）
   */
  async mergeItems(sourceId: string, targetId: string): Promise<boolean> {
    if (sourceId === targetId) return false
    const seq = [...this.sequence]
    const si = seq.findIndex((i) => i.id === sourceId)
    const ti = seq.findIndex((i) => i.id === targetId)
    if (si < 0 || ti < 0) return false

    const source = seq[si]
    const target = seq[ti]

    if (source.kind === 'folder' && target.kind === 'folder') {
      ui.error('不支持文件夹嵌套（夹里不能放夹）')
      return false
    }

    const incoming =
      source.kind === 'folder' ? source.children.map((c) => c.link_id) : [source.link_id as string]
    const existing = target.kind === 'folder' ? target.children.length : 1
    if (existing + incoming.length > MAX_FOLDER_ITEMS) {
      ui.error(`文件夹最多放 ${MAX_FOLDER_ITEMS} 个图标，已放回原位`)
      return false
    }

    // 目标不是文件夹：就地为它建一个，把目标的链接先收进去
    let dest = target
    if (target.kind === 'link') {
      const folderId = uuidv7()
      this.folders[folderId] = { id: folderId, name: null, size: 1 }
      this.#newFolders.add(folderId)
      dest = {
        id: uuidv7(),
        kind: 'folder',
        folder_id: folderId,
        size: 1,
        children: [{ id: uuidv7(), link_id: target.link_id as string, sort_order: 0 }],
      }
      seq[ti] = dest
    }

    const base = dest.children.length
    dest.children = [
      ...dest.children,
      ...incoming.map((link_id, k) => ({ id: uuidv7(), link_id, sort_order: base + k })),
    ]

    // source 从序列移除；若是文件夹，其实体要删掉（链接已转移，placement 由整板替换重写）
    seq.splice(si, 1)
    if (source.kind === 'folder' && source.folder_id) {
      if (this.#newFolders.has(source.folder_id)) this.#newFolders.delete(source.folder_id)
      else this.#deletedFolders.add(source.folder_id)
      this.#touchedFolders.delete(source.folder_id)
    }

    this.sequence = seq
    await this.commit()
    return true
  }

  /** 把夹内某个链接拖回主网格（追加到页面末尾） */
  async removeFromFolder(folderItemId: string, childId: string) {
    const folder = this.itemById(folderItemId)
    if (!folder) return
    const child = folder.children.find((c) => c.id === childId)
    if (!child) return

    folder.children = folder.children.filter((c) => c.id !== childId)
    this.sequence = [
      ...this.sequence,
      { id: uuidv7(), kind: 'link', link_id: child.link_id, size: 1, children: [] },
    ]
    await this.commit()
  }

  /** 把页面上的链接放进文件夹（供模态/拖入使用） */
  async addToFolder(folderItemId: string, linkItemId: string) {
    const folder = this.itemById(folderItemId)
    const item = this.itemById(linkItemId)
    if (!folder || !item || !item.link_id) return
    if (folder.children.length >= MAX_FOLDER_ITEMS) {
      ui.error(`文件夹最多放 ${MAX_FOLDER_ITEMS} 个图标`)
      return
    }
    folder.children = [
      ...folder.children,
      { id: uuidv7(), link_id: item.link_id, sort_order: folder.children.length },
    ]
    this.sequence = this.sequence.filter((i) => i.id !== linkItemId)
    await this.commit()
  }

  async reorderFolderChildren(folderItemId: string, orderedChildIds: string[]) {
    const folder = this.itemById(folderItemId)
    if (!folder) return
    const byId = new Map(folder.children.map((c) => [c.id, c]))
    const next = orderedChildIds.map((id) => byId.get(id)).filter((c): c is Child => Boolean(c))
    if (next.length !== folder.children.length) return
    folder.children = next.map((c, i) => ({ ...c, sort_order: i }))
    await this.commit()
  }

  async setFolderSize(folderItemId: string, size: 1 | 2) {
    const item = this.itemById(folderItemId)
    if (!item?.folder_id) return
    item.size = size
    const folder = this.folders[item.folder_id]
    if (folder) folder.size = size
    if (!this.#newFolders.has(item.folder_id)) this.#touchedFolders.add(item.folder_id)
    await this.commit()
  }

  async renameFolder(folderItemId: string, name: string) {
    const item = this.itemById(folderItemId)
    if (!item?.folder_id) return
    const folder = this.folders[item.folder_id]
    if (folder) folder.name = name || null
    if (!this.#newFolders.has(item.folder_id)) this.#touchedFolders.add(item.folder_id)
    await this.commit()
  }

  // ---------- 页面管理 ----------

  async refreshPages() {
    const data = await api.get<{ pages: Page[] }>('/api/pages/')
    this.pages = data.pages
  }

  async #uniqueSlug(name: string): Promise<string> {
    const base =
      (name || 'page')
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-+|-+$/g, '') || 'page'
    const taken = new Set(this.pages.map((p) => p.slug))
    let slug = base
    let n = 2
    while (taken.has(slug)) slug = `${base}-${n++}`
    return slug
  }

  async createPage(name: string): Promise<Page | null> {
    const id = uuidv7()
    const slug = await this.#uniqueSlug(name)
    try {
      const page = await api.post<Page>('/api/pages/', { id, slug, name: name || '新页面' })
      await this.refreshPages()
      await this.selectPage(page.id)
      return page
    } catch (err) {
      ui.error('新建页面失败：' + (err instanceof Error ? err.message : String(err)))
      return null
    }
  }

  async renamePage(id: string, name: string) {
    try {
      await api.patch(`/api/pages/${id}`, { name })
      const local = this.pages.find((p) => p.id === id)
      if (local) local.name = name
      if (this.page?.id === id) this.page.name = name
    } catch (err) {
      ui.error('重命名失败：' + (err instanceof Error ? err.message : String(err)))
    }
  }

  async deletePage(id: string) {
    try {
      await api.del(`/api/pages/${id}`)
      const wasCurrent = this.page?.id === id
      await this.refreshPages()
      if (wasCurrent && this.pages[0]) await this.selectPage(this.pages[0].id)
    } catch (err) {
      ui.error('删除页面失败：' + (err instanceof Error ? err.message : String(err)))
    }
  }

  async reorderPages(orderedIds: string[]) {
    try {
      for (let i = 0; i < orderedIds.length; i++) {
        await api.patch(`/api/pages/${orderedIds[i]}`, { sort_order: i })
      }
      await this.refreshPages()
    } catch (err) {
      ui.error('页面排序失败：' + (err instanceof Error ? err.message : String(err)))
    }
  }

  adjacentPage(dir: -1 | 1): Page | undefined {
    if (!this.page) return undefined
    const i = this.pages.findIndex((p) => p.id === this.page?.id)
    if (i < 0) return undefined
    return this.pages[i + dir]
  }

  get pageIndex(): number {
    if (!this.page) return 0
    return Math.max(0, this.pages.findIndex((p) => p.id === this.page?.id))
  }

  // ---------- 跨页拖拽 carry（Q28b 完整版）----------

  /**
   * 拖到屏幕边缘翻页时进入 carry 模式：
   *  1) 记下被搬的项（数据仍在源页，尚未落库）
   *  2) 合成一次 mouseup 让拖拽库体面收场（它监听的是 window 的 mouseup）
   *  3) 切到目标页，由漂浮层接管指针跟随，松手时走 /api/board/move 原子落库
   * 之所以不让库继续跨页拖：它的 zone 绑定在当前页的 DOM 上，
   * 换页会让它缓存的元素全部失效。
   */
  carry = $state<{ itemId: string; item: Item; fromPageId: string } | null>(null)

  /** 网格几何（由 Grid 测量后写入）：漂浮层据此把指针位置换算成落点下标 */
  gridMetrics = $state({ left: 0, top: 0, tile: 96, gap: 16, cols: GRID_COLS })

  beginCarry(itemId: string, toPageId: string): boolean {
    const item = this.itemById(itemId)
    if (!item || !this.page) return false
    this.carry = {
      itemId,
      item: { ...item, children: [...item.children] },
      fromPageId: this.page.id,
    }
    window.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
    window.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, pointerType: 'mouse' }))
    void this.selectPage(toPageId)
    return true
  }

  cancelCarry() {
    this.carry = null
  }

  /** 在目标页上按指针位置落位（index 由漂浮层算好） */
  async finishCarry(index: number) {
    const carry = this.carry
    const target = this.page
    if (!carry || !target) return
    this.carry = null

    const seq = [...this.sequence]
    const at = Math.max(0, Math.min(index, seq.length))
    seq.splice(at, 0, carry.item)
    const items = this.#layoutPayload(seq)
    if (!items) return

    try {
      const board = await api.post<Board>('/api/board/move', {
        from_page_id: carry.fromPageId,
        to_page_id: target.id,
        item_id: carry.itemId,
        items,
      })
      this.applyBoard(board)
      await this.refreshPages()
      ui.success('已移动到「' + board.page.name + '」')
    } catch (err) {
      ui.error('移动失败：' + (err instanceof Error ? err.message : String(err)))
      await this.reload()
    }
  }

  /** 非拖拽路径：上下文菜单里的「移动到…」（追加到目标页末尾） */
  async moveItemToPage(itemId: string, toPageId: string) {
    const item = this.itemById(itemId)
    if (!item || !this.page || toPageId === this.page.id) return
    try {
      const targetBoard = await api.get<Board>(`/api/pages/${toPageId}/board`)
      const seq = [...targetBoard.items]
        .sort((a, b) => a.row - b.row || a.col - b.col)
        .map((it): Item => ({
          id: it.id,
          kind: it.kind,
          link_id: it.link_id,
          folder_id: it.folder_id,
          size: (it.size ?? 1) as 1 | 2,
          children: it.children ?? [],
        }))
      seq.push(item)
      const items = this.#layoutPayload(seq)
      if (!items) return
      const board = await api.post<Board>('/api/board/move', {
        from_page_id: this.page.id,
        to_page_id: toPageId,
        item_id: itemId,
        items,
      })
      await this.refreshPages()
      // 当前停留在源页：重新读源页（revision 已被服务端 +1）
      await this.reload()
      ui.success('已移动到「' + board.page.name + '」')
    } catch (err) {
      ui.error('移动失败：' + (err instanceof Error ? err.message : String(err)))
    }
  }

  /** 把一条序列打包成服务端要的 items 数组（含 col/row/size/children） */
  #layoutPayload(seq: Item[]) {
    const placed = pack(seq, GRID_COLS, (i) => (i.kind === 'folder' ? (i.size ?? 1) : 1))
    return placed.map((p) => ({
      id: p.item.id,
      kind: p.item.kind,
      link_id: p.item.link_id,
      folder_id: p.item.folder_id,
      size: p.item.kind === 'folder' ? p.span : undefined,
      col: p.col,
      row: p.row,
      children: p.item.children,
    }))
  }

  // ---------- 提交 ----------

  private async commit() {
    if (!this.page) return
    const placed = pack(this.sequence, GRID_COLS, (i) => (i.kind === 'folder' ? (i.size ?? 1) : 1))
    const folderIds = new Set<string>([...this.#newFolders, ...this.#touchedFolders])
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
      new_folders: [...folderIds].map((id) => ({
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

  /** 合并悬停阈值（毫秒），来自服务端设置，默认 500 */
  get mergeDwellMs(): number {
    const raw = Number(this.settings['merge_dwell_ms'] ?? '500')
    return Number.isFinite(raw) && raw >= 0 ? raw : 500
  }

  /** 拖到屏幕边缘后翻页的悬停阈值（毫秒），来自服务端设置，默认 150 */
  get pageFlipEdgeMs(): number {
    const raw = Number(this.settings['page_flip_edge_ms'] ?? '150')
    return Number.isFinite(raw) && raw >= 0 ? raw : 150
  }

  get defaultEngine(): SearchEngine | undefined {
    const id = this.settings['default_engine_id']
    return this.engines.find((e) => e.id === id) ?? this.engines[0]
  }
}

export const board = new BoardStore()
