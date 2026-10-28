<script lang="ts">
  import type { SignalLevel } from '../wlan';
  // Four bars like a phone's Wi-Fi symbol; the level is also given as text
  // next to it, so the bars are decoration.
  let { level }: { level: SignalLevel } = $props();
  const filled = $derived({ good: 4, ok: 3, weak: 2, very_weak: 1, unknown: 0 }[level]);
</script>

<span class="bars {level}" aria-hidden="true">
  {#each [1, 2, 3, 4] as i (i)}<span class:on={i <= filled} style:height="{3 + i * 3}px"></span>{/each}
</span>

<style>
  .bars {
    display: inline-flex;
    align-items: flex-end;
    gap: 2px;
    height: 15px;
  }
  .bars span {
    width: 4px;
    background: var(--surface-3);
  }
  .bars span.on {
    background: var(--ok);
  }
  .ok span.on {
    background: var(--text-2);
  }
  .weak span.on {
    background: var(--amber);
  }
  .very_weak span.on {
    background: var(--danger);
  }
</style>
