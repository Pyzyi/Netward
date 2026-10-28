<script lang="ts">
  import type { Snippet } from 'svelte';
  let { text, children }: { text: string; children?: Snippet } = $props();
</script>

<div class="empty">
  <p>{text}</p>
  {#if children}<div class="actions">{@render children()}</div>{/if}
</div>

<style>
  .empty {
    display: grid;
    justify-items: center;
    gap: 12px;
    padding: 32px 16px;
    text-align: center;
    color: var(--text-2);
  }
  p {
    max-width: 42ch;
  }
</style>
