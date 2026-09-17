import type { Wallpaper } from '$lib/types'

/** 列表与常规显示用缩略图；全屏用原图（upload）或直链（url）。 */
export function wallpaperThumb(w: Wallpaper): string {
  if (w.kind === 'upload' && w.thumb_file) return `/wallpapers/thumb/${w.thumb_file}`
  return w.remote_url ?? ''
}

export function wallpaperFull(w: Wallpaper): string {
  if (w.kind === 'upload' && w.file) return `/wallpapers/orig/${w.file}`
  return w.remote_url ?? ''
}
