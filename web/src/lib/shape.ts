/**
 * 图块形状预设（设置 → 外观 → 图块形状）。
 *
 * 值是**容器圆角**，由 App.svelte 写进 `--radius-tile` / `--radius-tile-lg`
 * 两个 CSS 变量（内联在 <html> 上，覆盖 app.css 里 @theme 的默认值），
 * 图块、文件夹、拖拽幽灵都读这两个变量，所以一处设置全局生效。
 *
 * 大文件块（2×2）单独一档：它的外框接近正方形，若也取 50% 会变成一个正圆，
 * 里面固定 3×3 的九个格子四角会被裁掉 —— 所以圆形预设下大块用 26%（超椭圆感），
 * 视觉上仍是"圆润的一档"，但不切内容。
 */
export interface TileShape {
  id: string
  label: string
  tile: string
  big: string
}

export const TILE_SHAPES: TileShape[] = [
  { id: 'rounded', label: '圆角方形', tile: '1rem', big: '1rem' },
  { id: 'circle', label: '圆形', tile: '50%', big: '26%' },
  { id: 'squircle', label: '超椭圆', tile: '22%', big: '22%' },
  { id: 'square', label: '直角', tile: '0px', big: '0px' },
]

export const DEFAULT_TILE_SHAPE = 'rounded'

export function tileShape(id: string | undefined): TileShape {
  return TILE_SHAPES.find((s) => s.id === id) ?? TILE_SHAPES[0]
}
