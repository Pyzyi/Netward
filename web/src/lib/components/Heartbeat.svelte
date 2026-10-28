<script lang="ts">
  import { store } from '../state.svelte';
  import { number, t, time } from '../i18n.svelte';

  // The last 24 hours of a device in 48 half-hour slots, drawn as runs:
  // a solid bar while it was online, a thin baseline while Netzwart saw it
  // offline, nothing where it was not observed. Each run has a tooltip.
  let { strip = '', width = 192, height = 12 }: { strip?: string; width?: number; height?: number } = $props();

  const slots = 48;
  const slotMs = 30 * 60 * 1000;
  const step = $derived(width / slots);
  const cells = $derived(strip.padStart(slots, '-').slice(-slots));

  type Run = { kind: string; start: number; end: number };
  const runs = $derived.by(() => {
    const out: Run[] = [];
    for (let i = 0; i < slots; i++) {
      const k = cells[i];
      const last = out.at(-1);
      if (last && last.kind === k) last.end = i + 1;
      else out.push({ kind: k, start: i, end: i + 1 });
    }
    return out.filter((r) => r.kind !== '-');
  });
  const hours = $derived([...cells].filter((c) => c === '1').length / 2);
  const always = $derived(runs.length === 1 && runs[0].kind === '1' && runs[0].start === 0);
  const label = $derived(always ? t('device.heartbeatAlways') : t('device.heartbeat', { h: hours, hs: number(hours) }));
  const bar = $derived(Math.round(height * 0.66));
  const span = (r: Run) => {
    if (!store.stripFrom) return '';
    const a = store.stripFrom + r.start * slotMs;
    const b = Math.min(Date.now(), store.stripFrom + r.end * slotMs);
    return `${time(a)}–${time(b)}`;
  };
</script>

<svg class="beat" {width} {height} viewBox="0 0 {width} {height}" role="img" aria-label={label}>
  {#each [12, 24, 36] as q (q)}
    <rect class="tick" x={q * step - 0.5} y="0" width="1" {height} />
  {/each}
  {#each runs as r (r.start)}
    {#if r.kind === '1'}
      <rect class="on" x={r.start * step} y={(height - bar) / 2} width={Math.max(2, (r.end - r.start) * step - 1)} height={bar} rx="1.5">
        <title>{always ? t('device.heartbeatAlways') : `${span(r)} ${t('device.status.online').toLowerCase()}`}</title>
      </rect>
    {:else}
      <rect class="off" x={r.start * step} y={height / 2 - 0.75} width={Math.max(2, (r.end - r.start) * step - 1)} height="1.5">
        <title>{span(r)} {t('common.offline').toLowerCase()}</title>
      </rect>
    {/if}
  {/each}
  <rect class="now" x={width - 1} y="0" width="1" {height} />
</svg>

<style>
  .beat {
    display: block;
    flex: none;
    overflow: visible;
  }
  .on {
    fill: var(--signal);
    opacity: 0.85;
  }
  .tick {
    fill: var(--border);
  }
  .off {
    fill: var(--control-border);
  }
  .now {
    fill: var(--text-2);
    opacity: 0.35;
  }
</style>
