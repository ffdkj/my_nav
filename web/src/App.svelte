<script lang="ts">
  import { Moon, Settings, Sun } from '@lucide/svelte'
  import CarryLayer from '$lib/components/CarryLayer.svelte'
  import ContextMenu, { type MenuTarget } from '$lib/components/ContextMenu.svelte'
  import FolderDialog from '$lib/components/FolderDialog.svelte'
  import FolderModal from '$lib/components/FolderModal.svelte'
  import Grid from '$lib/components/Grid.svelte'
  import LinkDialog from '$lib/components/LinkDialog.svelte'
  import PageDots from '$lib/components/PageDots.svelte'
  import PageManager from '$lib/components/PageManager.svelte'
  import SearchBar from '$lib/components/SearchBar.svelte'
  import SettingsDialog from '$lib/components/SettingsDialog.svelte'
  import WallpaperLayer from '$lib/components/WallpaperLayer.svelte'
  import Toasts from '$lib/components/Toasts.svelte'
  import { api } from '$lib/api'
  import { tileShape } from '$lib/shape'
  import { board } from '$lib/store/board.svelte'
  import { pwa } from '$lib/store/pwa.svelte'
  import { theme } from '$lib/store/theme.svelte'
  import { ui } from '$lib/store/ui.svelte'
  import type { IconChoice, Item, Link } from '$lib/types'

  let dialogOpen = $state(false)
  let editing = $state<Link | null>(null)
  let openFolder = $state<Item | null>(null)
  let editFolder = $state<Item | null>(null)
  let managerOpen = $state(false)
  let settingsOpen = $state(false)
  let menu = $state<MenuTarget | null>(null)

  /** 换页动画里"出场"的那份旧网格快照的宿主 */
  let ghostHost = $state<HTMLElement | null>(null)
  /** 参与平移的网格容器（也是换页前要克隆的对象） */
  let stage = $state<HTMLElement | null>(null)

  let started = false
  $effect(() => {
    if (!started) {
      started = true
      theme.init()
      void board.boot()
      pwa.init()
    }
  })

  // 主题：settings.theme 是事实源，init() 里的 localStorage 只是首屏缓存。
  // 这个 effect 读 theme.dark，所以「跟随系统」时系统换了深浅色也会自动重跑。
  $effect(() => {
    theme.syncFromServer(board.settings['theme'])
    theme.apply()
  })

  // 图块形状：设置 → CSS 变量（`--radius-tile` / `--radius-tile-lg`）。
  // 内联在 <html> 上，覆盖 app.css 里 @theme 给的默认值；图块/文件夹/拖拽幽灵
  // 全部读这两个变量，所以这里改一处就够。
  $effect(() => {
    const s = tileShape(board.settings['tile_shape'])
    const style = document.documentElement.style
    style.setProperty('--radius-tile', s.tile)
    style.setProperty('--radius-tile-lg', s.big)
  })

  // 换页前抓一份当前网格的 DOM 快照（board.selectPage 在数据换掉之前调用它）。
  // 克隆的是舞台**里面**那层，避免把 data-testid="page-stage" 也复制一份出来。
  $effect(() => {
    board.snapshotGrid = () => {
      const el = stage?.firstElementChild as HTMLElement | null | undefined
      if (!el) return null
      const clone = el.cloneNode(true) as HTMLElement
      // 快照只是画面：不能接收焦点、不该进无障碍树（克隆里带着 <a href>）
      clone.setAttribute('inert', '')
      clone.setAttribute('aria-hidden', 'true')
      return clone
    }
    return () => {
      board.snapshotGrid = null
    }
  })

  // 动画状态机：transition 变化 → 塞入快照 → 等两帧 → 让 CSS 过渡跑起来
  $effect(() => {
    const t = board.transition
    if (!t || !ghostHost) {
      ghostHost?.replaceChildren()
      return
    }
    ghostHost.replaceChildren(...(t.ghost ? [t.ghost] : []))
  })

  /**
   * 出场快照：从 0 平移到屏幕外（方向与入场相反）。
   * 用 translate3d 而不是 translateX：强制提到合成层，避免动画期间重新栅格化。
   */
  const ghostStyle = $derived.by(() => {
    const t = board.transition
    if (!t) return 'display: none'
    const pct = board.animating ? -t.dir * 100 : 0
    return `transform: translate3d(${pct}%, 0, 0); transition: transform var(--page-slide-ms) var(--page-slide-ease); will-change: transform;`
  })

  /** 入场网格：从屏幕外平移到 0 */
  const stageStyle = $derived.by(() => {
    const t = board.transition
    if (!t) return ''
    const pct = board.animating ? 0 : t.dir * 100
    return `transform: translate3d(${pct}%, 0, 0); transition: transform var(--page-slide-ms) var(--page-slide-ease); will-change: transform;`
  })

  // 动画期间在 <html> 上挂标记：app.css 靠它临时关掉玻璃面的 backdrop-filter
  // （移动层上每帧重算模糊是掉帧主因）。animating 一翻真就挂上，收尾时自动摘掉。
  $effect(() => {
    document.documentElement.dataset.pageSliding = board.animating ? '1' : ''
  })

  // ---------- 翻页 ----------

  function flip(dir: -1 | 1): boolean {
    const p = board.adjacentPage(dir)
    if (!p) return false
    void board.selectPage(p.id)
    return true
  }

  // 滚轮：累积超过阈值翻一页，400ms 内不重复触发
  let wheelAcc = 0
  let wheelLockUntil = 0
  function onwheel(e: WheelEvent) {
    if (board.carry || board.pages.length < 2) return
    const now = Date.now()
    if (now < wheelLockUntil) return
    wheelAcc += e.deltaY
    if (Math.abs(wheelAcc) >= 120) {
      if (flip(wheelAcc > 0 ? 1 : -1)) {
        wheelAcc = 0
        wheelLockUntil = now + 400
      }
    }
  }

  // 手势横滑：只在非图块区域起手，横向位移 >60 且纵向 <40 才算翻页
  let swipeStart: { x: number; y: number } | null = null
  function onpointerdown(e: PointerEvent) {
    if ((e.target as HTMLElement | null)?.closest('[data-tile]')) return
    swipeStart = { x: e.clientX, y: e.clientY }
  }
  function onpointerup(e: PointerEvent) {
    if (!swipeStart || board.carry) {
      swipeStart = null
      return
    }
    const dx = e.clientX - swipeStart.x
    const dy = e.clientY - swipeStart.y
    swipeStart = null
    if (Math.abs(dx) > 60 && Math.abs(dy) < 40) flip(dx < 0 ? 1 : -1)
  }

  // 拖拽到屏幕左右边缘 → 翻页并进入跨页 carry
  function handleEdge(dir: -1 | 1, itemId: string) {
    const p = board.adjacentPage(dir)
    if (!p) {
      ui.info(dir === 1 ? '已经是最后一页' : '已经是第一页')
      return
    }
    board.beginCarry(itemId, p.id)
  }

  // ---------- 图标操作 ----------

  function openAdd() {
    editing = null
    dialogOpen = true
  }

  function openEdit(item: Item) {
    if (item.kind === 'folder') {
      editFolder = item
      return
    }
    editing = board.linkOf(item) ?? null
    dialogOpen = true
  }

  async function submitDialog(url: string, title: string, icon: IconChoice | null) {
    dialogOpen = false
    if (editing) {
      const target = editing
      const id = target.id
      board.links[id] = { ...target, url, title: title || target.title }
      try {
        await api.patch(`/api/links/${id}`, { url, title: title || target.title })
      } catch (err) {
        ui.error('保存失败：' + (err instanceof Error ? err.message : String(err)))
        await board.reload()
        return
      }
      await applyIcon(id, icon)
    } else {
      // 新增：先建链接（服务端会同步抓一次标准 favicon），
      // 再把用户在对话框里选中的那张覆盖上去（Q11 决策 a）
      const id = await board.addLink(url, title)
      if (id) await applyIcon(id, icon)
    }
  }

  /** 把对话框里选的图标落到某个已存在的链接上（null = 保持现状） */
  async function applyIcon(linkId: string, icon: IconChoice | null) {
    if (!icon) return
    switch (icon.kind) {
      case 'candidate':
        await board.pickIcon(linkId, icon.candidate)
        break
      case 'monogram':
        await board.applyMonogram(linkId, icon.text, icon.color, icon.fontSize)
        break
      case 'upload':
        await board.uploadIcon(linkId, icon.file)
        break
    }
  }

  function confirmDelete(item: Item) {
    if (item.kind === 'folder') {
      editFolder = item
      return
    }
    const link = board.linkOf(item)
    const name = link?.title ?? '该图标'
    if (!window.confirm(`确定删除「${name}」吗？`)) return
    void board.removeItem(item.id)
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      menu = null
      if (board.carry) board.cancelCarry()
    }
  }
</script>

<svelte:window onkeydown={onkeydown} />

<main
  class="relative min-h-dvh"
  onwheel={onwheel}
  onpointerdown={onpointerdown}
  onpointerup={onpointerup}
>
  <WallpaperLayer />

  <div class="mx-auto flex min-h-dvh max-w-6xl flex-col gap-6 px-4 py-8">
    <header class="flex items-center gap-3">
      <h1 class="shrink-0 text-lg font-semibold tracking-tight">my_nav</h1>
      <div class="flex flex-1 justify-center">
        <SearchBar />
      </div>
      <button
        type="button"
        class="shrink-0 cursor-pointer rounded-full bg-glass p-2 ring-1 ring-glass-ring hover:bg-glass-hover"
        aria-label={theme.dark ? '切换到浅色主题' : '切换到深色主题'}
        data-testid="theme-toggle"
        onclick={() => theme.toggle()}
      >
        {#if theme.dark}<Moon size={18} />{:else}<Sun size={18} />{/if}
      </button>
      <button
        type="button"
        class="shrink-0 cursor-pointer rounded-full bg-glass p-2 ring-1 ring-glass-ring hover:bg-glass-hover"
        aria-label="打开设置"
        onclick={() => (settingsOpen = true)}
      >
        <Settings size={18} />
      </button>
    </header>

    {#if board.page}
      <p class="text-xs text-fg/40">
        {board.page.name}
        · {board.sequence.length} 个图标
        {#if board.status === 'saving'}· 保存中…{/if}
        {#if board.status === 'loading'}· 加载中…{/if}
        {#if board.carry}· 跨页移动中，松手放下{/if}
      </p>
    {/if}

    <!--
      换页舞台：只有这里的图标参与平移。
      搜索栏 / 主题按钮 / 设置入口 / 页码圆点都在这个 div 之外，永远不动。
      出场的那一份是换页前克隆的旧网格（见 board.snapshotGrid），
      它静态、不可交互，只是为了"旧页面滑出去"这一段视觉。
    -->
    <div class="relative flex-1">
      <div
        bind:this={ghostHost}
        class="pointer-events-none absolute inset-x-0 top-0"
        style={ghostStyle}
        aria-hidden="true"
        data-testid="page-ghost"
      ></div>

      <div bind:this={stage} style={stageStyle} data-testid="page-stage">
        {#if board.status === 'loading' && board.sequence.length === 0}
          <p class="py-20 text-center text-sm text-fg/50">加载中…</p>
        {:else}
          <Grid
            onadd={openAdd}
            onedit={openEdit}
            ondelete={confirmDelete}
            onopenfolder={(item) => (openFolder = item)}
            onmenu={(target) => (menu = target)}
            onedge={handleEdge}
          />
        {/if}
      </div>
    </div>

    <footer class="flex flex-col items-center gap-3 pt-4">
      {#if pwa.needRefresh}
        <div
          class="flex items-center gap-3 rounded-full bg-accent-500/95 px-4 py-1.5 text-xs text-white ring-1 ring-fg/25"
          role="status"
        >
          有新版本可用
          <button
            type="button"
            onclick={() => pwa.applyUpdate()}
            class="cursor-pointer rounded-full bg-fg/20 px-2 py-0.5 font-medium hover:bg-fg/30"
          >
            刷新
          </button>
          <button
            type="button"
            onclick={() => pwa.dismiss()}
            class="cursor-pointer rounded-full px-2 py-0.5 hover:bg-fg/20"
            aria-label="稍后再说"
          >
            稍后
          </button>
        </div>
      {/if}
      <PageDots onmanage={() => (managerOpen = true)} />
      <p class="text-xs text-fg/30">
        {#if board.lastError}
          上次操作失败：{board.lastError}
        {:else}
          M6 · 壁纸（上传/外链/轮换）· 设置中心 · 引擎管理
        {/if}
      </p>
    </footer>
  </div>
</main>

<LinkDialog
  open={dialogOpen}
  link={editing}
  onsubmit={submitDialog}
  onclose={() => (dialogOpen = false)}
/>

<FolderModal item={openFolder} onclose={() => (openFolder = null)} />
<FolderDialog item={editFolder} onclose={() => (editFolder = null)} />
<PageManager open={managerOpen} onclose={() => (managerOpen = false)} />
<SettingsDialog open={settingsOpen} onclose={() => (settingsOpen = false)} />
<ContextMenu
  target={menu}
  onclose={() => (menu = null)}
  onedit={openEdit}
  ondelete={(item) => {
    menu = null
    confirmDelete(item)
  }}
/>
<CarryLayer />

<Toasts />
