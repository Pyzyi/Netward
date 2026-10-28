<script lang="ts">
  import { icons } from '../icons';

  // Phosphor icons on a 256 grid. Decorative unless a label is given.
  let { name, size = 18, label = '', class: cls = '' }: { name: string; size?: number; label?: string; class?: string } = $props();
  const parts = $derived(icons[name] ?? icons.question);
</script>

<svg
  class="icon {cls}"
  width={size}
  height={size}
  viewBox="0 0 256 256"
  fill="currentColor"
  role={label ? 'img' : undefined}
  aria-label={label || undefined}
  aria-hidden={label ? undefined : 'true'}
  focusable="false"
>
  {#each parts as [d, opacity, stroke], i (i)}
    {#if stroke}
      <path {d} fill="none" stroke="currentColor" stroke-width="16" stroke-linecap="round" stroke-linejoin="round" />
    {:else}
      <path {d} opacity={opacity} />
    {/if}
  {/each}
</svg>

<style>
  .icon {
    flex: none;
    display: inline-block;
    vertical-align: middle;
  }
</style>
