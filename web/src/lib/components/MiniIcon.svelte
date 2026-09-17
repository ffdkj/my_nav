<script lang="ts">
  import { initialOf } from '$lib/store/board.svelte'
  import type { Link } from '$lib/types'

  interface Props {
    link: Link
    /** 'preview' = 夹外缩略（不可点）；'live' = 大夹内直接可点 */
    mode?: 'preview' | 'live'
  }
  let { link, mode = 'preview' }: Props = $props()

  const host = $derived.by(() => {
    try {
      return new URL(link.url).hostname.replace(/^www\./, '')
    } catch {
      return link.url
    }
  })
  const label = $derived(link.title || host)
</script>

{#snippet face()}
  {#if link.icon_status === 'ok' && link.icon_path}
    <img src="/icons/{link.icon_path}" alt="" class="size-[70%] object-contain" loading="lazy" />
  {:else}
    <span
      class="flex size-[80%] items-center justify-center rounded-full text-[0.6em] font-semibold text-white"
      style="background:{link.mono_color}"
    >
      {initialOf(link)}
    </span>
  {/if}
{/snippet}

{#if mode === 'live'}
  <a
    href={link.url}
    target={link.open_new_tab ? '_blank' : undefined}
    rel="noreferrer noopener"
    draggable="false"
    style="-webkit-user-drag:none"
    class="flex aspect-square items-center justify-center rounded-lg bg-white/5 transition hover:bg-white/20
           focus-visible:ring-2 focus-visible:ring-accent-500 focus-visible:outline-none"
    aria-label={label}
    title={label}
  >
    {@render face()}
  </a>
{:else}
  <span class="flex aspect-square items-center justify-center rounded-lg bg-white/5">
    {@render face()}
  </span>
{/if}
