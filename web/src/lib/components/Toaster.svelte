<script lang="ts">
  import CircleCheck from '@lucide/svelte/icons/circle-check';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Info from '@lucide/svelte/icons/info';
  import { toasts } from './toast.svelte';
</script>

<div class="toaster" role="status" aria-live="polite">
  {#each toasts.list as item (item.id)}
    <div class="toast {item.kind}">
      {#if item.kind === 'ok'}<CircleCheck size={17} />{:else if item.kind === 'error'}<CircleAlert size={17} />{:else}<Info size={17} />{/if}
      <span>{item.text}</span>
    </div>
  {/each}
</div>

<style>
  .toaster {
    position: fixed;
    left: 50%;
    bottom: calc(24px + env(safe-area-inset-bottom));
    transform: translateX(-50%);
    display: grid;
    gap: 8px;
    z-index: 100;
    pointer-events: none;
    width: max-content;
    max-width: calc(100vw - 32px);
  }
  @media (max-width: 639px) {
    .toaster {
      bottom: calc(76px + env(safe-area-inset-bottom));
    }
  }
  .toast {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 10px 14px;
    border-radius: 10px;
    background: var(--text);
    color: var(--bg);
    box-shadow: var(--shadow-pop);
    font-weight: 500;
    animation: rise 180ms var(--ease);
  }
  .toast.ok :global(svg) {
    color: color-mix(in srgb, var(--ok) 65%, var(--bg));
  }
  .toast.error :global(svg) {
    color: color-mix(in srgb, var(--danger) 70%, var(--bg));
  }
  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }
</style>
