import type { Wallpaper } from '$lib/types'

/** 兜底底色的出厂默认值（与 001_init.sql 的种子一致） */
export const DEFAULT_FALLBACK_COLOR = '#0b1220'

/**
 * 无壁纸时的底色。
 * 仍是出厂默认值就返回**主题底色**，这样浅色主题下不会糊上一层深蓝；
 * 用户自己挑过颜色则尊重用户的选择。
 */
export function fallbackColor(stored: string | null | undefined): string {
  if (!stored || stored.toLowerCase() === DEFAULT_FALLBACK_COLOR) return 'var(--color-surface-900)'
  return stored
}

/** 列表与常规显示用缩略图；全屏用原图（upload）或直链（url）。 */
export function wallpaperThumb(w: Wallpaper): string {
  if (w.kind === 'upload' && w.thumb_file) return `/wallpapers/thumb/${w.thumb_file}`
  return w.remote_url ?? ''
}

export function wallpaperFull(w: Wallpaper): string {
  if (w.kind === 'upload' && w.file) return `/wallpapers/orig/${w.file}`
  return w.remote_url ?? ''
}
