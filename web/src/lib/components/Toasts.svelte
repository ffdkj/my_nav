<script lang="ts">
  import { ui } from '$lib/store/ui.svelte'
</script>

<div class="pointer-events-none fixed inset-x-0 bottom-4 z-[100] flex flex-col items-center gap-2 px-4">
  {#each ui.toasts as toast (toast.id)}
    <button
      type="button"
      onclick={() => ui.dismiss(toast.id)}
      class="pointer-events-auto max-w-md cursor-pointer rounded-xl px-4 py-2 text-sm shadow-lg ring-1 backdrop-blur
             {toast.kind === 'error'
        ? 'bg-red-500/90 text-white ring-red-300/40'
        : toast.kind === 'success'
          ? 'bg-emerald-500/90 text-white ring-emerald-300/40'
          : 'bg-surface-700/95 text-white ring-white/20'}"
    >
      {toast.message}
    </button>
  {/each}
</div>
