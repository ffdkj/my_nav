<script lang="ts">
  import type { Link } from '$lib/types'

  interface Props {
    open: boolean
    /** 传入表示编辑已有链接，null 表示新增 */
    link?: Link | null
    onsubmit: (url: string, title: string) => void
    onclose: () => void
  }
  let { open, link = null, onsubmit, onclose }: Props = $props()

  let url = $state('')
  let title = $state('')
  let error = $state<string | null>(null)

  // 打开时把已有值填进表单
  $effect(() => {
    if (open) {
      url = link?.url ?? ''
      title = link?.title ?? ''
      error = null
      // 自动聚焦 URL 输入框
      queueMicrotask(() => document.getElementById('link-url-input')?.focus())
    }
  })

  function submit(e: SubmitEvent) {
    e.preventDefault()
    const trimmed = url.trim()
    if (!trimmed) {
      error = '请填写网址'
      return
    }
    try {
      const parsed = new URL(trimmed)
      if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
        error = '只支持 http / https 网址'
        return
      }
    } catch {
      error = '网址格式不正确（需要带 http:// 或 https://）'
      return
    }
    onsubmit(trimmed, title.trim())
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
    <form
      onsubmit={submit}
      class="w-full max-w-md rounded-2xl bg-surface-800 p-6 ring-1 ring-white/15"
      aria-label={link ? '编辑图标' : '添加图标'}
    >
      <h2 class="mb-4 text-base font-semibold">{link ? '编辑图标' : '添加图标'}</h2>

      <label class="mb-3 block text-sm">
        <span class="mb-1 block text-white/70">网站地址</span>
        <input
          id="link-url-input"
          bind:value={url}
          placeholder="https://example.com"
          class="w-full rounded-lg bg-white/10 px-3 py-2 text-sm ring-1 ring-white/15 outline-none focus:ring-2 focus:ring-accent-500"
          autocomplete="off"
        />
      </label>

      <label class="mb-4 block text-sm">
        <span class="mb-1 block text-white/70">网站名称（留空则自动取域名）</span>
        <input
          bind:value={title}
          placeholder="例如 GitHub"
          class="w-full rounded-lg bg-white/10 px-3 py-2 text-sm ring-1 ring-white/15 outline-none focus:ring-2 focus:ring-accent-500"
          autocomplete="off"
        />
      </label>

      {#if error}
        <p class="mb-3 text-sm text-red-400">{error}</p>
      {/if}

      <div class="flex justify-end gap-2">
        <button
          type="button"
          onclick={onclose}
          class="cursor-pointer rounded-lg bg-white/10 px-4 py-2 text-sm hover:bg-white/20"
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
