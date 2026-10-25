<script lang="ts">
  import { t } from '../i18n.svelte';

  // The last 24 hours of a device as 48 half-hour beats: a full bar when it
  // was online, a low tick when Netzwart saw it offline, nothing when unknown.
  let { strip = '', width = 144, height = 18 }: { strip?: string; width?: number; height?: number } = $props();

  const slots = 48;
  const step = $derived(width / slots);
  const bars = $derived([...strip.padStart(slots, '-').slice(-slots)]);
  const hours = $derived(bars.filter((b) => b === '1').length / 2);
</script>

<svg class="beat" {width} {height} viewBox="0 0 {width} {height}" role="img" aria-label={t('device.heartbeat', { h: hours })}>
  {#each bars as b, i (i)}
    {#if b === '1'}
      <rect class="on" x={i * step} y="0" width={Math.max(1, step - 1)} {height} rx="0.5" />
    {:else if b === '0'}
      <rect class="off" x={i * step} y={height - 2} width={Math.max(1, step - 1)} height="2" />
    {/if}
  {/each}
</svg>

<style>
  .beat {
    display: block;
    flex: none;
  }
  .on {
    fill: var(--signal);
  }
  .off {
    fill: var(--off);
  }
</style>
