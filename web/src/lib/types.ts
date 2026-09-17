/**
 * 与后端 API 共享的契约类型（手写，单一来源）。
 * 对应 docs/spec.md §6 与 internal/nav/types.go —— 两边字段名必须一致。
 * 所有实体主键是客户端生成的 UUIDv7 文本。
 */

export type Id = string

export interface Page {
  id: Id
  slug: string
  name: string
  sort_order: number
  revision: number
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
  /** 1 = 1x1 小夹；2 = 2x2 大夹（占 4 格、内部 9 个图标直接可点） */
  size: 1 | 2
}

export interface Child {
  id: Id
  link_id: Id
  sort_order: number
}

/** 客户端侧的布局单元：顺序即布局，col/row 由打包器算出 */
export interface Item {
  id: Id
  kind: 'link' | 'folder'
  link_id?: Id
  folder_id?: Id
  size?: 1 | 2
  children: Child[]
}

/** 服务端返回的 board 里，item 带上了权威的 col/row */
export interface BoardItem extends Item {
  col: number
  row: number
}

export interface Board {
  page: Page
  revision: number
  items: BoardItem[]
  links: Link[]
  folders: Folder[]
}

export interface BoardPayload {
  revision: number
  items: Array<{
    id: Id
    kind: 'link' | 'folder'
    link_id?: Id
    folder_id?: Id
    size?: number
    col: number
    row: number
    children?: Child[]
  }>
  new_links?: Array<{ id: Id; url: string; title: string }>
  new_folders?: Array<{ id: Id; name?: string | null; size?: number }>
  deleted_link_ids?: Id[]
  deleted_folder_ids?: Id[]
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

export interface Wallpaper {
  id: Id
  kind: 'upload' | 'url'
  remote_url: string | null
  file: string | null
  thumb_file: string | null
  w: number | null
  h: number | null
  bytes: number | null
  sort_order: number
}

export type Settings = Record<string, string>

export interface Bootstrap {
  pages: Page[]
  settings: Settings
  engines: SearchEngine[]
}

/** 搜索用的扁平链接（带所属页面信息，供全局搜索） */
export interface LinkWithPage extends Link {
  page_id: Id
  page_slug: string
  page_name: string
}

export interface ApiError {
  error: { code: string; message: string; conflicts?: Array<{ col: number; row: number }> }
}
