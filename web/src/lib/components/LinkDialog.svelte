<script lang="ts">
  import { api } from '$lib/api'
  import { board } from '$lib/store/board.svelte'
  import type { IconCandidate, IconCandidates, IconChoice, Link } from '$lib/types'

  interface Props {
    open: boolean
    /** 传入表示编辑已有链接，null 表示新增 */
    link?: Link | null
    onsubmit: (url: string, title: string, icon: IconChoice | null) => void
    onclose: () => void
  }
  let { open, link = null, onsubmit, onclose }: Props = $props()

  const PALETTE = [
    '#EF4444', '#F97316', '#EAB308', '#22C55E', '#14B8A6',
    '#06B6D4', '#3B82F6', '#6366F1', '#8B5CF6', '#EC4899',
  ]

  /** 输入停止多久后自动查候选（Q6/Q11 决策：不是每敲一个字就查） */
  const DEBOUNCE_MS = 600

  let url = $state('')
  let title = $state('')
  let error = $state<string | null>(null)
  let busy = $state<string | null>(null)

  // ---------- 候选 ----------
  let candidates = $state<IconCandidate[]>([])
  let loading = $state(false)
  let hint = $state<string | null>(null)
  /** 已经查过的 URL：避免同一个地址反复查（服务端另有 60s 缓存兜底） */
  let queriedFor = $state('')
  /** 竞态序号：只接受最后一次请求的结果（用户改网址后旧响应必须丢弃） */
  let seq = 0

  // ---------- 选择 ----------
  type Choice =
    | { kind: 'keep' }
    | { kind: 'monogram' }
    | { kind: 'upload' }
    | { kind: 'candidate'; index: number }
  let choice = $state<Choice>({ kind: 'keep' })

  // 纯色文字
  let monoText = $state('')
  let monoColor = $state('#3B82F6')
  let monoSize = $state(30)

  // 本地图标
  let fileInput = $state<HTMLInputElement | null>(null)
  let uploadFile = $state<File | null>(null)
  let uploadPreview = $state<string | null>(null)

  const host = $derived.by(() => {
    try {
      return new URL(url).hostname.replace(/^www\./, '')
    } catch {
      return ''
    }
  })
  const letter = $derived((monoText || title || host || '?').slice(0, 1).toUpperCase())

  function isURL(raw: string): boolean {
    try {
      const parsed = new URL(raw.trim())
      return parsed.protocol === 'http:' || parsed.protocol === 'https:'
    } catch {
      return false
    }
  }

  // 打开时重置：编辑态把现有的纯色参数带进来
  $effect(() => {
    if (!open) return
    url = link?.url ?? ''
    title = link?.title ?? ''
    error = null
    busy = null
    candidates = []
    queriedFor = ''
    hint = null
    loading = false
    choice = { kind: 'keep' }
    uploadFile = null
    uploadPreview = null
    monoText = link?.mono_text ?? ''
    monoColor = link?.mono_color ?? '#3B82F6'
    monoSize = link?.mono_font_size ?? 30
    queueMicrotask(() => document.getElementById('link-url-input')?.focus())
  })

  // 输入停止 DEBOUNCE_MS 后查一次候选（新增链接时链接还不存在，所以按 URL 查）
  $effect(() => {
    if (!open) return
    const target = url.trim()
    if (!isURL(target) || target === queriedFor) return
    const timer = setTimeout(() => void loadCandidates(target), DEBOUNCE_MS)
    return () => clearTimeout(timer)
  })

  async function loadCandidates(target: string) {
    const mine = ++seq
    loading = true
    hint = null
    try {
      const res = await api.post<IconCandidates>('/api/icons/candidates', { url: target })
      if (mine !== seq) return
      candidates = res.candidates
      queriedFor = target
      if (res.candidates.length === 0) hint = '这个站没有找到可用图标，先用纯色兜底'
    } catch (err) {
      if (mine !== seq) return
      queriedFor = target // 失败也记下来，免得用户每敲一个字就打一次对方站点
      hint = '查询图标失败：' + (err instanceof Error ? err.message : String(err))
    } finally {
      if (mine === seq) loading = false
    }
  }

  /** 重新查一次（不走 queriedFor 去重，直接查） */
  async function requery() {
    const target = url.trim()
    if (!isURL(target)) {
      error = '先填一个正确的网址再查询'
      return
    }
    candidates = []
    await loadCandidates(target)
  }

  // ---------- 编辑态的即时操作 ----------

  async function refetch() {
    if (!link) return
    busy = '正在重新抓取…'
    await board.refetchIcon(link.id)
    choice = { kind: 'keep' }
    busy = null
    await requery()
  }

  async function reset() {
    if (!link) return
    busy = '正在重置…'
    await board.resetIcon(link.id)
    choice = { kind: 'keep' }
    busy = null
    await requery()
  }

  // ---------- 本地上传 ----------

  function pickFile(event: Event) {
    const input = event.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    if (!file) return
    if (file.size > 512 * 1024) {
      error = '图标文件不要超过 512KB'
      input.value = ''
      return
    }
    error = null
    if (uploadPreview) URL.revokeObjectURL(uploadPreview)
    uploadFile = file
    uploadPreview = URL.createObjectURL(file)
    choice = { kind: 'upload' }
    input.value = ''
  }

  function close() {
    if (uploadPreview) {
      URL.revokeObjectURL(uploadPreview)
      uploadPreview = null
    }
    onclose()
  }

  // ---------- 提交 ----------

  function chosenIcon(): IconChoice | null {
    switch (choice.kind) {
      case 'candidate': {
        const candidate = candidates[choice.index]
        return candidate ? { kind: 'candidate', candidate } : null
      }
      case 'monogram':
        return { kind: 'monogram', text: monoText, color: monoColor, fontSize: monoSize }
      case 'upload':
        return uploadFile ? { kind: 'upload', file: uploadFile } : null
      default:
        return null
    }
  }

  function submit(e: SubmitEvent) {
    e.preventDefault()
    const trimmed = url.trim()
    if (!trimmed) {
      error = '请填写网址'
      return
    }
    if (!isURL(trimmed)) {
      error = '网址格式不正确（需要带 http:// 或 https://）'
      return
    }
    onsubmit(trimmed, title.trim(), chosenIcon())
  }

  function meta(c: IconCandidate): string {
    if (c.mime === 'image/svg+xml') return '矢量'
    if (c.width > 0 && c.height > 0) return `${c.width}×${c.height}`
    return c.mime === 'image/x-icon' ? 'ICO' : '位图'
  }

  function cardClass(selected: boolean): string {
    return `flex cursor-pointer flex-col items-center gap-1 rounded-lg p-1 ring-1 transition ${
      selected ? 'bg-accent-500/15 ring-2 ring-accent-500' : 'ring-fg/10 hover:bg-fg/10'
    }`
  }
</script>

{#if open}
  <div
    class="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) close()
    }}
  >
    <form
      onsubmit={submit}
      class="max-h-[90vh] w-full max-w-md overflow-y-auto rounded-2xl bg-surface-800 p-6 ring-1 ring-fg/15"
      aria-label={link ? '编辑图标' : '添加图标'}
    >
      <h2 class="mb-4 text-base font-semibold">{link ? '编辑图标' : '添加图标'}</h2>

      <label class="mb-3 block text-sm">
        <span class="mb-1 block text-fg/70">网站地址</span>
        <input
          id="link-url-input"
          bind:value={url}
          placeholder="https://example.com"
          class="w-full rounded-lg bg-fg/10 px-3 py-2 text-sm ring-1 ring-fg/15 outline-none focus:ring-2 focus:ring-accent-500"
          autocomplete="off"
        />
      </label>

      <label class="mb-4 block text-sm">
        <span class="mb-1 block text-fg/70">网站名称（留空则自动取域名）</span>
        <input
          bind:value={title}
          placeholder="例如 GitHub"
          class="w-full rounded-lg bg-fg/10 px-3 py-2 text-sm ring-1 ring-fg/15 outline-none focus:ring-2 focus:ring-accent-500"
          autocomplete="off"
        />
      </label>

      <fieldset class="mb-4 rounded-xl bg-fg/5 p-3">
        <legend class="px-1 text-xs text-fg/60">选择图标</legend>

        <div class="grid grid-cols-4 gap-2" data-testid="icon-cards">
          <!-- 纯色文字 -->
          <button
            type="button"
            onclick={() => (choice = { kind: 'monogram' })}
            class={cardClass(choice.kind === 'monogram')}
            aria-pressed={choice.kind === 'monogram'}
            data-testid="icon-card-monogram"
          >
            <span
              class="flex aspect-square w-full items-center justify-center rounded-md text-2xl font-semibold text-white"
              style="background:{monoColor}"
            >
              {letter}
            </span>
            <span class="text-[10px] text-fg/60">纯色图标</span>
          </button>

          <!-- 候选缩略图（服务端已经存好了字节，直接引 /icons/…） -->
          {#each candidates as c, i (c.icon_path)}
            <button
              type="button"
              onclick={() => (choice = { kind: 'candidate', index: i })}
              class={cardClass(choice.kind === 'candidate' && choice.index === i)}
              aria-pressed={choice.kind === 'candidate' && choice.index === i}
              title="{c.source} · {c.width}×{c.height} · {Math.round(c.bytes / 1024)}KB{c.alpha
                ? ' · 透明底'
                : ''}"
              data-testid="icon-card-candidate"
            >
              <span class="relative flex aspect-square w-full items-center justify-center overflow-hidden rounded-md bg-fg/5">
                <img src="/icons/{c.icon_path}" alt="" class="size-full object-contain p-0.5" />
                {#if c.alpha}
                  <span class="absolute top-0.5 right-0.5 rounded bg-black/60 px-1 text-[8px] text-white">
                    透明
                  </span>
                {/if}
              </span>
              <span class="text-[10px] text-fg/60">图标{String(i + 1).padStart(2, '0')}</span>
              <span class="text-[9px] leading-none text-fg/40">{meta(c)}</span>
            </button>
          {/each}

          <!-- 查询中的占位卡 -->
          {#if loading}
            {#each [0, 1, 2] as i (i)}
              <span class="flex flex-col items-center gap-1 p-1" data-testid="icon-card-skeleton">
                <span class="aspect-square w-full animate-pulse rounded-md bg-fg/10"></span>
                <span class="text-[10px] text-fg/30">查询中…</span>
              </span>
            {/each}
          {/if}

          <!-- 本地图标 -->
          <button
            type="button"
            onclick={() => fileInput?.click()}
            class={cardClass(choice.kind === 'upload')}
            aria-pressed={choice.kind === 'upload'}
            data-testid="icon-card-upload"
          >
            <span class="flex aspect-square w-full items-center justify-center overflow-hidden rounded-md bg-fg/5">
              {#if uploadPreview}
                <img src={uploadPreview} alt="" class="size-full object-contain p-0.5" />
              {:else}
                <span class="text-lg text-fg/40">＋</span>
              {/if}
            </span>
            <span class="text-[10px] text-fg/60">本地图标</span>
          </button>
        </div>

        <input
          bind:this={fileInput}
          type="file"
          accept="image/png,image/jpeg,image/webp,image/svg+xml"
          onchange={pickFile}
          class="hidden"
          aria-label="上传本地图标"
        />

        <div class="mt-2 flex items-start justify-between gap-2">
          <p class="text-xs text-fg/40">
            {#if hint}
              {hint}
            {:else if candidates.length}
              来自站点声明与 favicon 服务，共 {candidates.length} 个；上传 PNG/JPG/WebP/SVG ≤512KB
            {:else if link?.icon_picked_url}
              当前是手选的图标
            {:else}
              输入网址后自动查询；也可以上传本地图片或用纯色文字
            {/if}
          </p>
          <button
            type="button"
            onclick={requery}
            class="shrink-0 cursor-pointer rounded-lg bg-fg/10 px-2 py-1 text-xs hover:bg-fg/20"
            data-testid="icon-requery"
          >
            重新查询
          </button>
        </div>

        {#if choice.kind === 'monogram'}
          <div class="mt-3 flex flex-col gap-2 border-t border-fg/10 pt-3">
            <label class="text-xs text-fg/70">
              文字（留空取域名首字母）
              <input
                bind:value={monoText}
                maxlength="2"
                class="mt-1 w-full rounded-lg bg-fg/10 px-3 py-1.5 text-sm ring-1 ring-fg/15 outline-none focus:ring-2 focus:ring-accent-500"
              />
            </label>
            <label class="text-xs text-fg/70">
              字号 <span class="text-fg/40">{monoSize}</span>
              <input
                type="range"
                min="12"
                max="64"
                bind:value={monoSize}
                class="mt-1 w-full accent-[var(--color-accent-500)]"
              />
            </label>
            <div class="flex flex-wrap items-center gap-1.5">
              {#each PALETTE as c (c)}
                <button
                  type="button"
                  onclick={() => (monoColor = c)}
                  class="size-5 cursor-pointer rounded-full ring-2 transition
                         {monoColor.toLowerCase() === c.toLowerCase() ? 'ring-fg' : 'ring-transparent'}"
                  style="background:{c}"
                  aria-label="颜色 {c}"
                ></button>
              {/each}
              <input
                type="color"
                bind:value={monoColor}
                class="size-6 cursor-pointer rounded border-0 bg-transparent"
                aria-label="自定义颜色"
              />
            </div>
          </div>
        {/if}

        {#if link}
          <div class="mt-3 flex items-center justify-between gap-2 border-t border-fg/10 pt-2">
            <a href={link.url} target="_blank" rel="noreferrer noopener" class="text-xs text-accent-500 hover:underline">
              打开原站 ↗
            </a>
            <span class="flex gap-2">
              <button
                type="button"
                onclick={refetch}
                class="cursor-pointer rounded-lg bg-fg/10 px-2 py-1 text-xs hover:bg-fg/20"
              >
                重新抓取
              </button>
              <button
                type="button"
                onclick={reset}
                class="cursor-pointer rounded-lg bg-fg/10 px-2 py-1 text-xs hover:bg-fg/20"
              >
                重置为标准 favicon
              </button>
            </span>
          </div>
        {/if}
      </fieldset>

      {#if busy}
        <p class="mb-3 text-sm text-fg/60">{busy}</p>
      {/if}
      {#if error}
        <p class="mb-3 text-sm text-red-600 dark:text-red-400">{error}</p>
      {/if}

      <div class="flex justify-end gap-2">
        <button
          type="button"
          onclick={close}
          class="cursor-pointer rounded-lg bg-fg/10 px-4 py-2 text-sm hover:bg-fg/20"
        >
          取消
        </button>
        <button
          type="submit"
          class="cursor-pointer rounded-lg bg-accent-500 px-4 py-2 text-sm font-medium text-white hover:brightness-110"
        >
          确定
        </button>
      </div>
    </form>
  </div>
{/if}
