/**
 * 主题（白天/黑夜）。
 *
 * 事实源是服务端设置 `settings.theme`（`light | dark | auto`），
 * localStorage 只作为**首屏缓存**——否则每次打开都要等 `/api/bootstrap`
 * 回来才知道该用哪套颜色，浅色用户会先看到一帧深色（闪白/闪黑）。
 * `index.html` 里那段内联脚本读的就是这份缓存。
 */
import { board } from '$lib/store/board.svelte'

export type ThemeMode = 'light' | 'dark' | 'auto'

/** 与 index.html 内联脚本共用的 key，改这里要同步改那边 */
export const THEME_STORAGE_KEY = 'my-nav-theme'

/** 与 app.css 里 :root 的 --c-surface-900 对应，用于 <meta name="theme-color"> */
const THEME_COLOR: Record<'light' | 'dark', string> = {
  light: '#eef1f8',
  dark: '#070b14',
}

export function isThemeMode(v: unknown): v is ThemeMode {
  return v === 'light' || v === 'dark' || v === 'auto'
}

class ThemeStore {
  mode = $state<ThemeMode>('dark')
  #systemDark = $state(false)
  #media: MediaQueryList | null = null

  /** 当前实际生效的是不是深色（auto 时看系统） */
  get dark(): boolean {
    return this.mode === 'dark' || (this.mode === 'auto' && this.#systemDark)
  }

  get modeLabel(): string {
    return this.mode === 'light' ? '浅色' : this.mode === 'dark' ? '深色' : '跟随系统'
  }

  /** 幂等：初始化一次，注册系统主题变化监听 */
  init() {
    if (this.#media) return
    this.#media = window.matchMedia('(prefers-color-scheme: dark)')
    this.#systemDark = this.#media.matches
    this.#media.addEventListener('change', (e) => {
      this.#systemDark = e.matches
    })
    const cached = localStorage.getItem(THEME_STORAGE_KEY)
    if (isThemeMode(cached)) this.mode = cached
    this.apply()
  }

  /** 服务端设置是事实源：拿到之后覆盖首屏缓存（只有真的不同才动，避免来回抖） */
  syncFromServer(value: string | undefined) {
    if (!isThemeMode(value) || value === this.mode) return
    this.mode = value
    this.#cache()
  }

  set(mode: ThemeMode) {
    if (mode === this.mode) return
    this.mode = mode
    this.#cache()
    void board.saveSetting('theme', mode)
  }

  /** 在 浅色 ⇄ 深色 之间切换；auto 时按当前解析结果取反 */
  toggle() {
    this.set(this.dark ? 'light' : 'dark')
  }

  /**
   * 把当前主题写到 <html>：`.dark` 类驱动 app.css 的色板与 Tailwind 的 dark: 变体。
   * 由 App.svelte 的 $effect 调用，因此 mode / 系统主题变化都会自动重跑。
   */
  apply() {
    const dark = this.dark
    const root = document.documentElement
    root.classList.toggle('dark', dark)
    root.dataset.theme = dark ? 'dark' : 'light'
    root.dataset.themeMode = this.mode
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute('content', THEME_COLOR[dark ? 'dark' : 'light'])
  }

  #cache() {
    try {
      localStorage.setItem(THEME_STORAGE_KEY, this.mode)
    } catch {
      /* 隐私模式下写不了：不影响功能，只是少了首屏缓存 */
    }
  }
}

export const theme = new ThemeStore()
