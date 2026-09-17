/**
 * 与后端 API 共享的契约类型（手写，单一来源）。
 * 对应 docs/spec.md §6；后端 Go 结构体字段名需与此保持一致。
 * 所有实体主键为客户端生成的 UUIDv7 文本。
 */

export type Id = string

export interface Page {
  id: Id
  slug: string
  name: string
  sort_order: number
  wallpaper_mode: 'global' | 'custom'
  wallpaper_id: Id | null
}

export type IconSource = 'auto' | 'upload' | 'monogram'
export type IconStatus = 'pending' | 'ok' | 'miss' | 'error'

export interface Link {
  id: Id
  title: string
  url: string
  open_new_tab: boolean
  icon_source: IconSource
  icon_path: string | null
  icon_status: IconStatus
  mono_text: string | null
  mono_color: string
  mono_font_size: number
}

export interface Folder {
  id: Id
  name: string | null
  /** 1 = 1×1 小夹（外显 9 缩略图，点击开模态）；2 = 2×2 大夹（内部直接可点） */
  size: 1 | 2
}

/** 页面上的一个占位：要么是链接，要么是文件夹（2×2 时占 4 格） */
export interface BoardItem {
  id: Id
  kind: 'link' | 'folder'
  link_id?: Id
  folder_id?: Id
  col: number
  row: number
  /** kind === 'folder' 时的子项顺序（夹内顺序，与 col/row 无关） */
  children?: BoardChild[]
}

export interface BoardChild {
  id: Id
  link_id: Id
  sort_order: number
}

export interface Board {
  page: Page
  revision: number
  items: BoardItem[]
  links: Link[]
  folders: Folder[]
}

export interface SearchEngine {
  id: Id
  name: string
  /** 必须包含 {query} 占位符 */
  url_tpl: string
  icon_text: string
  icon_color: string
  sort_order: number
  is_builtin: boolean
}

export interface Settings {
  default_engine_id: Id | null
  merge_dwell_ms: number
  page_flip_edge_ms: number
  wallpaper_mode: 'global' | 'custom'
  wallpaper_id: Id | null
  wallpaper_rotation: 'off' | 'load' | 'interval'
  wallpaper_interval_min: number
  wallpaper_fallback: string
  search_open_new_tab: boolean
  theme: 'dark' | 'light' | 'auto'
}

export interface Bootstrap {
  pages: Page[]
  settings: Settings
  engines: SearchEngine[]
  revision: number
}

/** 统一错误体 */
export interface ApiError {
  error: { code: string; message: string }
}
