<script lang="ts">
  import { api } from '$lib/api'
  import { board } from '$lib/store/board.svelte'
  import { ui } from '$lib/store/ui.svelte'
  import type { Link } from '$lib/types'

  interface Props {
    open: boolean
    /** 传入表示编辑已有链接，null 表示新增 */
    link?: Link | null
    onsubmit: (url: string, title: string) => void
    onclose: () => void
  }
  let { open, link = null, onsubmit, onclose }: Props = $props()

  const PALETTE = [
    '#EF4444', '#F97316', '#EAB308', '#22C55E', '#14B8A6',
    '#06B6D4', '#3B82F6', '#6366F1', '#8B5CF6', '#EC4899',
  ]

  let url = $state('')
  let title = $state('')
  let error = $state<string | null>(null)
  let busy = $state<string | null>(null)

  // 图标编辑（仅对已存在的链接有意义）
  let tab = $state<'auto' | 'upload' | 'monogram'>('auto')
  let monoText = $state('')
  let monoColor = $state('#3B82F6')
  let monoSize = $state(30)

  $effect(() => {
    if (!open) return
    url = link?.url ?? ''
    title = link?.title ?? ''
    error = null
    busy = null
    monoText = link?.mono_text ?? ''
    monoColor = link?.mono_color ?? '#3B82F6'
    monoSize = link?.mono_font_size ?? 30
    tab = link?.icon_source === 'monogram' ? 'monogram' : link?.icon_source === 'upload' ? 'upload' : 'auto'
    queueMicrotask(() => document.getElementById('link-url-input')?.focus())
  })

  function validate(raw: string): string | null {
    const trimmed = raw.trim()
    if (!trimmed) return '请填写网址'
    try {
      const parsed = new URL(trimmed)
      if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return '只支持 http / https 网址'
    } catch {
      return '网址格式不正确（需要带 http:// 或 https://）'
    }
    return null
  }

  function submit(e: SubmitEvent) {
    e.preventDefault()
    const problem = validate(url)
    if (problem) {
      error = problem
      return
    }
    onsubmit(url.trim(), title.trim())
  }

  // ---------- 图标操作 ----------

  function applyLink(updated: Link) {
    if (!link) return
    board.links[link.id] = updated
  }

  async function refetch() {
    if (!link) return
    busy = '正在重新抓取…'
    try {
      applyLink(await api.post<Link>(`/api/links/${link.id}/icon/refetch`))
      ui.success('已重新抓取')
    } catch (err) {
      ui.error('抓取失败：' + (err instanceof Error ? err.message : String(err)))
    } finally {
      busy = null
    }
  }

  async function reset() {
    if (!link) return
    busy = '正在重置…'
    try {
      applyLink(await api.post<Link>(`/api/links/${link.id}/icon/reset`))
      ui.success('已重置为标准 favicon')
    } catch (err) {
      ui.error('重置失败：' + (err instanceof Error ? err.message : String(err)))
    } finally {
      busy = null
    }
  }

  async function applyMonogram() {
    if (!link) return
    busy = '正在保存纯色图标…'
    try {
      applyLink(
        await api.post<Link>(`/api/links/${link.id}/icon/monogram`, {
          text: monoText,
          color: monoColor,
          font_size: monoSize,
        }),
      )
      ui.success('已切换为纯色文字图标')
    } catch (err) {
      ui.error('保存失败：' + (err instanceof Error ? err.message : String(err)))
    } finally {
      busy = null
    }
  }

  async function upload(event: Event) {
    const input = event.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    if (!file || !link) return
    if (file.size > 512 * 1024) {
      error = '图标文件不要超过 512KB'
      input.value = ''
      return
    }
    busy = '正在上传…'
    try {
      const form = new FormData()
      form.append('file', file)
      applyLink(await api.upload<Link>(`/api/links/${link.id}/icon/upload`, form))
      ui.success('已上传本地图标')
    } catch (err) {
      ui.error('上传失败：' + (err instanceof Error ? err.message : String(err)))
    } finally {
      busy = null
      input.value = ''
    }
  }

  const previewLink = $derived(link ? { ...board.links[link.id], mono_color: monoColor, mono_text: monoText || null } : null)
</script>

{#snippet iconPreview()}
  {#if link?.icon_status === 'ok' && link.icon_path && tab === 'auto'}
    <img src="/icons/{link.icon_path}" alt="" class="size-14 object-contain" />
  {:else}
    <span
      class="flex size-14 items-center justify-center rounded-full text-lg font-semibold text-white"
      style="background:{monoColor}"
    >
      {(monoText || previewLink?.title || '?').slice(0, 1).toUpperCase()}
    </span>
  {/if}
{/snippet}

{#if open}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) onclose()
    }}
  >
    <form
      onsubmit={submit}
      class="max-h-[90vh] w-full max-w-md overflow-y-auto rounded-2xl bg-surface-800 p-6 ring-1 ring-fg/15"
      aria-label={link ? '编辑图标' : '添加图标'}
    >
      <h2 class="mb-4 text-base font-semibold">{link ? '编辑图标' : '添加图标'}</h2>

      <div class="mb-3 flex items-center gap-4">
        {@render iconPreview()}
        <div class="min-w-0 flex-1">
          <p class="truncate text-sm">{title || link?.title || '未命名'}</p>
          <p class="truncate text-xs text-fg/40">{url || link?.url}</p>
        </div>
      </div>

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

      {#if link}
        <fieldset class="mb-4 rounded-xl bg-fg/5 p-3">
          <legend class="px-1 text-xs text-fg/60">图标</legend>

          <div class="mb-3 flex gap-1" role="tablist" aria-label="图标来源">
            {#each [['auto', '自动抓取'], ['upload', '本地图标'], ['monogram', '纯色文字']] as [value, label] (value)}
              <button
                type="button"
                role="tab"
                aria-selected={tab === value}
                onclick={() => (tab = value as typeof tab)}
                class="flex-1 cursor-pointer rounded-lg px-2 py-1.5 text-xs ring-1 transition
                       {tab === value
                  ? 'bg-accent-500 text-white ring-accent-500'
                  : 'bg-fg/5 text-fg/70 ring-fg/15 hover:bg-fg/10'}"
              >
                {label}
              </button>
            {/each}
          </div>

          {#if tab === 'auto'}
            <div class="flex gap-2">
              <button
                type="button"
                onclick={refetch}
                class="cursor-pointer rounded-lg bg-fg/10 px-3 py-1.5 text-xs hover:bg-fg/20"
              >
                重新抓取
              </button>
              <button
                type="button"
                onclick={reset}
                class="cursor-pointer rounded-lg bg-fg/10 px-3 py-1.5 text-xs hover:bg-fg/20"
              >
                重置为标准 favicon
              </button>
            </div>
            <p class="mt-2 text-xs text-fg/40">
              当前状态：{link.icon_status === 'ok'
                ? '已抓取'
                : link.icon_status === 'miss'
                  ? '该站没有可用图标（正在用纯色兜底）'
                  : link.icon_status === 'error'
                    ? '抓取出错'
                    : '待抓取'}
            </p>
          {:else if tab === 'upload'}
            <input
              type="file"
              accept="image/png,image/jpeg,image/webp,image/svg+xml"
              onchange={upload}
              class="block w-full cursor-pointer text-xs text-fg/70 file:mr-3 file:cursor-pointer file:rounded-lg file:border-0 file:bg-fg/10 file:px-3 file:py-1.5 file:text-fg/80"
              aria-label="上传本地图标"
            />
            <p class="mt-2 text-xs text-fg/40">PNG / JPG / WebP / SVG，≤512KB（服务端会缩到 256px）</p>
          {:else}
            <div class="flex flex-col gap-2">
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
              <button
                type="button"
                onclick={applyMonogram}
                class="self-start cursor-pointer rounded-lg bg-accent-500 px-3 py-1.5 text-xs font-medium text-white hover:brightness-110"
              >
                应用纯色文字
              </button>
            </div>
          {/if}

          <div class="mt-3 border-t border-fg/10 pt-2">
            <a
              href={link.url}
              target="_blank"
              rel="noreferrer noopener"
              class="text-xs text-accent-500 hover:underline"
            >
              打开原站 ↗
            </a>
          </div>
        </fieldset>
      {/if}

      {#if busy}
        <p class="mb-3 text-sm text-fg/60">{busy}</p>
      {/if}
      {#if error}
        <p class="mb-3 text-sm text-red-600 dark:text-red-400">{error}</p>
      {/if}

      <div class="flex justify-end gap-2">
        <button
          type="button"
          onclick={onclose}
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
