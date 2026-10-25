<script lang="ts">
  // The mark echoes the Netzring: the network as a ring around its router,
  // one device lighting up. On a tile for light surfaces, bare on ink.
  let { size = 28, tile = true }: { size?: number; tile?: boolean } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" class="logo" class:tile>
  {#if tile}<rect width="32" height="32" rx="7" fill="var(--ink)" />{/if}
  <circle cx="16" cy="16" r="9.5" fill="none" stroke="var(--on-ink)" stroke-width="2.4" />
  <circle cx="16" cy="16" r="2.6" fill="var(--on-ink)" />
  <circle cx="22.7" cy="9.3" r="3.6" fill="var(--amber)" stroke="var(--ink)" stroke-width="2" />
</svg>

<style>
  .logo {
    flex: none;
    display: block;
  }
</style>
