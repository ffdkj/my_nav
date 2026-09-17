<script lang="ts">
  import { board } from '$lib/store/board.svelte'
  import { ui } from '$lib/store/ui.svelte'
  import { wallpaperThumb } from '$lib/wallpaper'
  import { uuidv7 } from '$lib/id'
  import type { SearchEngine } from '$lib/types'

  interface Props {
    open: boolean
    onclose: () => void
  }
  let { open, onclose }: Props = $props()

  type Tab = 'look' | 'wallpapers' | 'search' | 'interaction'
  let tab = $state<Tab>('look')
  let busy = $state<string | null>(null)
  let urlDraft = $state('')

  const PALETTE = [
    '#EF4444', '#F97316', '#EAB308', '#22C55E', '#14B8A6',
    '#06B6D4', '#3B82F6', '#6366F1', '#8B5CF6', '#EC4899',
  ]

  // 引擎编辑表单
  let editingEngine = $state<SearchEngine | null>(null)
  let engineFormOpen = $state(false)
  let engineDraft = $state({ name: '', url_tpl: '', icon_text: '', icon_color: '#3B82F6' })

  $effect(() => {
    if (open) void board.loadWallpapers()
  })

  function startEdit(engine: SearchEngine | null) {
    editingEngine = engine
    engineFormOpen = true
    engineDraft = engine
      ? {
          name: engine.name,
          url_tpl: engine.url_tpl,
          icon_text: engine.icon_text,
          icon_color: engine.icon_color,
        }
      : { name: '', url_tpl: '', icon_text: '', icon_color: '#3B82F6' }
  }

  async function saveEngine() {
    const ok = await board.saveEngine(
      { id: editingEngine?.id ?? uuidv7(), ...engineDraft },
      editingEngine === null,
    )
    if (ok) {
      engineFormOpen = false
      editingEngine = null
    }
  }

  async function upload(event: Event) {
    const input = event.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    if (!file) return
    if (file.size > 10 * 1024 * 1024) {
      ui.error('壁纸不要超过 10MB')
      input.value = ''
      return
    }
    busy = '正在上传壁纸…'
    await board.addWallpaperFile(uuidv7(), file)
    busy = null
    input.value = ''
  }

  async function addURL() {
    const url = urlDraft.trim()
    if (!url) return
    busy = '正在登记外链…'
    const created = await board.addWallpaperURL(uuidv7(), url)
    busy = null
    if (created) urlDraft = ''
  }

  async function moveWallpaper(index: number, dir: -1 | 1) {
    const ids = board.wallpapers.map((w) => w.id)
    const target = index + dir
    if (target < 0 || target >= ids.length) return
    ;[ids[index], ids[target]] = [ids[target], ids[index]]
    await board.reorderWallpapers(ids)
  }

  async function setGlobal(id: string) {
    await board.saveSetting('wallpaper_id', id)
  }

  async function setForPage(id: string) {
    if (!board.page) return
    try {
      await board.saveSetting('wallpaper_mode', 'custom')
      await board.saveSetting('wallpaper_id', id)
      board.page.wallpaper_mode = 'custom'
      board.page.wallpaper_id = id
      ui.success('已设为当前页壁纸')
    } catch {
      /* saveSetting 已提示 */
    }
  }
</script>

{#if open}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) onclose()
    }}
  >
    <div
      class="flex max-h-[88vh] w-full max-w-2xl flex-col rounded-2xl bg-surface-800 ring-1 ring-white/15"
      role="dialog"
      aria-modal="true"
      aria-label="设置"
    >
      <header class="flex items-center justify-between border-b border-white/10 px-5 py-3">
        <h2 class="text-base font-semibold">设置</h2>
        <button
          type="button"
          onclick={onclose}
          class="cursor-pointer rounded-lg px-2 py-1 text-sm text-white/60 hover:bg-white/10 hover:text-white"
          aria-label="关闭设置"
        >
          &#10005;
        </button>
      </header>

      <div class="flex gap-1 border-b border-white/10 px-3 py-2" role="tablist" aria-label="设置分区">
        {#each [['look', '外观'], ['wallpapers', '壁纸'], ['search', '搜索'], ['interaction', '交互']] as [value, label] (value)}
          <button
            type="button"
            role="tab"
            aria-selected={tab === value}
            onclick={() => (tab = value as Tab)}
            class="cursor-pointer rounded-lg px-3 py-1.5 text-sm transition
                   {tab === value ? 'bg-accent-500 text-white' : 'text-white/70 hover:bg-white/10'}"
          >
            {label}
          </button>
        {/each}
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto px-5 py-4 text-sm">
        {#if tab === 'look'}
          <section class="flex flex-col gap-4">
            <label class="flex items-center justify-between gap-4">
              <span>主题</span>
              <select
                value={board.settings['theme'] ?? 'dark'}
                onchange={(e) => board.saveSetting('theme', e.currentTarget.value)}
                class="rounded-lg bg-white/10 px-3 py-1.5 ring-1 ring-white/15"
              >
                <option value="dark">深色</option>
                <option value="light">浅色</option>
                <option value="auto">跟随系统</option>
              </select>
            </label>

            <label class="flex items-center justify-between gap-4">
              <span>本页壁纸</span>
              <select
                value={board.page?.wallpaper_mode ?? 'global'}
                onchange={async (e) => {
                  const mode = e.currentTarget.value as 'global' | 'custom'
                  if (!board.page) return
                  board.page.wallpaper_mode = mode
                  await board.saveSetting('wallpaper_mode', mode)
                }}
                class="rounded-lg bg-white/10 px-3 py-1.5 ring-1 ring-white/15"
              >
                <option value="global">跟随全局</option>
                <option value="custom">本页单独指定</option>
              </select>
            </label>

            <label class="flex items-center justify-between gap-4">
              <span>轮换</span>
              <select
                value={board.settings['wallpaper_rotation'] ?? 'off'}
                onchange={(e) => board.saveSetting('wallpaper_rotation', e.currentTarget.value)}
                class="rounded-lg bg-white/10 px-3 py-1.5 ring-1 ring-white/15"
              >
                <option value="off">关闭</option>
                <option value="load">每次进入随机</option>
                <option value="interval">定时轮换</option>
              </select>
            </label>

            {#if (board.settings['wallpaper_rotation'] ?? 'off') === 'interval'}
              <label class="flex items-center justify-between gap-4">
                <span>轮换间隔（分钟）</span>
                <input
                  type="number"
                  min="1"
                  max="1440"
                  value={board.settings['wallpaper_interval_min'] ?? '30'}
                  onchange={(e) => board.saveSetting('wallpaper_interval_min', e.currentTarget.value)}
                  class="w-24 rounded-lg bg-white/10 px-3 py-1.5 ring-1 ring-white/15"
                />
              </label>
            {/if}

            <label class="flex items-center justify-between gap-4">
              <span>无壁纸时的底色</span>
              <input
                type="color"
                value={board.settings['wallpaper_fallback'] ?? '#0b1220'}
                onchange={(e) => board.saveSetting('wallpaper_fallback', e.currentTarget.value)}
                class="size-8 cursor-pointer rounded border-0 bg-transparent"
                aria-label="兜底底色"
              />
            </label>
          </section>
        {:else if tab === 'wallpapers'}
          <section class="flex flex-col gap-4">
            <div class="flex flex-wrap items-center gap-2">
              <label
                class="cursor-pointer rounded-lg bg-accent-500 px-3 py-1.5 text-sm font-medium text-white hover:brightness-110"
              >
                上传壁纸
                <input
                  type="file"
                  accept="image/png,image/jpeg,image/webp,image/gif"
                  onchange={upload}
                  class="hidden"
                  aria-label="上传壁纸"
                />
              </label>
              <input
                bind:value={urlDraft}
                placeholder="图床 URL（https://…）"
                class="min-w-0 flex-1 rounded-lg bg-white/10 px-3 py-1.5 ring-1 ring-white/15 outline-none focus:ring-2 focus:ring-accent-500"
                aria-label="图床 URL"
              />
              <button
                type="button"
                onclick={addURL}
                class="cursor-pointer rounded-lg bg-white/10 px-3 py-1.5 hover:bg-white/20"
              >
                添加
              </button>
            </div>
            <p class="text-xs text-white/40">
              上传会存到服务器并生成 1920 宽缩略图；外链只登记地址（省服务器流量），
              需要固化时点「下载到服务器」。
            </p>

            {#if board.wallpapers.length === 0}
              <p class="py-6 text-center text-white/40">还没有壁纸</p>
            {:else}
              <ul class="grid grid-cols-2 gap-3 sm:grid-cols-3">
                {#each board.wallpapers as w, i (w.id)}
                  <li class="overflow-hidden rounded-xl bg-white/5 ring-1 ring-white/10">
                    <img src={wallpaperThumb(w)} alt="" class="h-24 w-full object-cover" />
                    <div class="flex items-center justify-between gap-1 p-2 text-xs">
                      <span class="truncate text-white/60">
                        {w.kind === 'upload' ? '本地' : '外链'}
                        {#if w.w && w.h}· {w.w}×{w.h}{/if}
                      </span>
                      <div class="flex shrink-0 gap-1">
                        <button
                          type="button"
                          onclick={() => moveWallpaper(i, -1)}
                          class="cursor-pointer rounded px-1 hover:bg-white/10"
                          aria-label="上移壁纸"
                        >
                          &#8593;
                        </button>
                        <button
                          type="button"
                          onclick={() => moveWallpaper(i, 1)}
                          class="cursor-pointer rounded px-1 hover:bg-white/10"
                          aria-label="下移壁纸"
                        >
                          &#8595;
                        </button>
                        <button
                          type="button"
                          onclick={() => board.deleteWallpaper(w.id)}
                          class="cursor-pointer rounded px-1 text-red-300 hover:bg-red-500/20"
                          aria-label="删除壁纸"
                        >
                          &#10005;
                        </button>
                      </div>
                    </div>
                    <div class="flex flex-wrap gap-1 px-2 pb-2 text-xs">
                      <button
                        type="button"
                        onclick={() => setGlobal(w.id)}
                        class="cursor-pointer rounded bg-white/10 px-2 py-0.5 hover:bg-white/20"
                      >
                        设为全局
                      </button>
                      <button
                        type="button"
                        onclick={() => setForPage(w.id)}
                        class="cursor-pointer rounded bg-white/10 px-2 py-0.5 hover:bg-white/20"
                      >
                        设为本页
                      </button>
                      {#if w.kind === 'url'}
                        <button
                          type="button"
                          onclick={() => board.materializeWallpaper(w.id)}
                          class="cursor-pointer rounded bg-white/10 px-2 py-0.5 hover:bg-white/20"
                        >
                          下载到服务器
                        </button>
                      {/if}
                    </div>
                  </li>
                {/each}
              </ul>
            {/if}
          </section>
        {:else if tab === 'search'}
          <section class="flex flex-col gap-4">
            <label class="flex items-center justify-between gap-4">
              <span>默认搜索引擎</span>
              <select
                value={board.settings['default_engine_id'] ?? ''}
                onchange={(e) => board.saveSetting('default_engine_id', e.currentTarget.value)}
                class="rounded-lg bg-white/10 px-3 py-1.5 ring-1 ring-white/15"
                aria-label="默认搜索引擎"
              >
                {#each board.engines as e (e.id)}
                  <option value={e.id}>{e.name}</option>
                {/each}
              </select>
            </label>

            <div class="rounded-xl bg-white/5 p-3">
              <div class="mb-2 flex items-center justify-between">
                <p class="text-xs text-white/60">搜索引擎列表（内置的可以改，但不能删）</p>
                <button
                  type="button"
                  onclick={() => startEdit(null)}
                  class="cursor-pointer rounded bg-white/10 px-2 py-0.5 text-xs hover:bg-white/20"
                >
                  新增
                </button>
              </div>
              <ul class="flex flex-col gap-1">
                {#each board.engines as e (e.id)}
                  <li class="flex items-center gap-2 rounded-lg px-2 py-1 hover:bg-white/5">
                    <span
                      class="flex size-5 shrink-0 items-center justify-center rounded-full text-[10px] font-semibold text-white"
                      style="background:{e.icon_color}"
                    >
                      {e.icon_text}
                    </span>
                    <span class="min-w-0 flex-1 truncate">{e.name}</span>
                    <span class="hidden truncate text-xs text-white/30 sm:block">{e.url_tpl}</span>
                    <button
                      type="button"
                      onclick={() => startEdit(e)}
                      class="cursor-pointer rounded px-1.5 text-xs hover:bg-white/10"
                      aria-label="编辑 {e.name}"
                    >
                      编辑
                    </button>
                    <button
                      type="button"
                      disabled={e.is_builtin}
                      onclick={() => board.deleteEngine(e.id)}
                      class="cursor-pointer rounded px-1.5 text-xs text-red-300 hover:bg-red-500/20 disabled:opacity-30"
                      aria-label="删除 {e.name}"
                    >
                      删除
                    </button>
                  </li>
                {/each}
              </ul>
            </div>

            {#if engineFormOpen}
              <div class="rounded-xl bg-white/5 p-3">
                  <p class="mb-2 text-xs text-white/60">
                    {editingEngine ? `编辑「${editingEngine.name}」` : '新增搜索引擎'}
                  </p>
                  <div class="grid gap-2 sm:grid-cols-2">
                    <input
                      bind:value={engineDraft.name}
                      placeholder="名称"
                      class="rounded-lg bg-white/10 px-3 py-1.5 ring-1 ring-white/15"
                      aria-label="引擎名称"
                    />
                    <input
                      bind:value={engineDraft.icon_text}
                      maxlength="2"
                      placeholder="图标文字（1-2 字）"
                      class="rounded-lg bg-white/10 px-3 py-1.5 ring-1 ring-white/15"
                      aria-label="引擎图标文字"
                    />
                    <input
                      bind:value={engineDraft.url_tpl}
                      placeholder={'https://example.com/search?q={query}'}
                      class="rounded-lg bg-white/10 px-3 py-1.5 ring-1 ring-white/15 sm:col-span-2"
                      aria-label="引擎 URL 模板"
                    />
                    <div class="flex flex-wrap items-center gap-1.5 sm:col-span-2">
                      {#each PALETTE as c (c)}
                        <button
                          type="button"
                          onclick={() => (engineDraft.icon_color = c)}
                          class="size-5 cursor-pointer rounded-full ring-2
                                 {engineDraft.icon_color === c ? 'ring-white' : 'ring-transparent'}"
                          style="background:{c}"
                          aria-label="颜色 {c}"
                        ></button>
                      {/each}
                    </div>
                  </div>
                  <div class="mt-2 flex justify-end gap-2">
                    <button
                      type="button"
                      onclick={() => startEdit(null)}
                      class="cursor-pointer rounded-lg bg-white/10 px-3 py-1.5 text-xs hover:bg-white/20"
                    >
                      取消
                    </button>
                    <button
                      type="button"
                      onclick={saveEngine}
                      class="cursor-pointer rounded-lg bg-accent-500 px-3 py-1.5 text-xs font-medium text-white hover:brightness-110"
                    >
                      保存
                    </button>
                </div>
              </div>
            {/if}
          </section>
        {:else}
          <section class="flex flex-col gap-4">
            <label class="flex items-center justify-between gap-4">
              <span>
                合并悬停阈值
                <span class="text-xs text-white/40">（拖到另一块上停多久算合并）</span>
              </span>
              <span class="flex items-center gap-2">
                <input
                  type="range"
                  min="0"
                  max="1000"
                  step="50"
                  value={board.settings['merge_dwell_ms'] ?? '500'}
                  oninput={(e) => board.saveSetting('merge_dwell_ms', e.currentTarget.value)}
                  class="w-40 accent-[var(--color-accent-500)]"
                  aria-label="合并悬停阈值"
                />
                <span class="w-14 text-right text-xs text-white/60">
                  {board.settings['merge_dwell_ms'] ?? '500'}ms
                </span>
              </span>
            </label>

            <label class="flex items-center justify-between gap-4">
              <span>边缘翻页阈值 <span class="text-xs text-white/40">（拖到屏幕边缘多久翻页）</span></span>
              <span class="flex items-center gap-2">
                <input
                  type="range"
                  min="0"
                  max="600"
                  step="50"
                  value={board.settings['page_flip_edge_ms'] ?? '150'}
                  oninput={(e) => board.saveSetting('page_flip_edge_ms', e.currentTarget.value)}
                  class="w-40 accent-[var(--color-accent-500)]"
                  aria-label="边缘翻页阈值"
                />
                <span class="w-14 text-right text-xs text-white/60">
                  {board.settings['page_flip_edge_ms'] ?? '150'}ms
                </span>
              </span>
            </label>
          </section>
        {/if}
      </div>

      {#if busy}
        <p class="border-t border-white/10 px-5 py-2 text-xs text-white/60">{busy}</p>
      {/if}
    </div>
  </div>
{/if}
