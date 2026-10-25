<script lang="ts">
  import type { Presence } from '../types';
  import { dateTime, time, t } from '../i18n.svelte';

  let { data }: { data: Presence } = $props();

  const total = $derived(Math.max(1, data.to - data.from));
  const pct = (v: number) => `${(Math.max(0, Math.min(total, v - data.from)) / total) * 100}%`;
  const width = (a: number, b: number) =>
    `${(Math.max(0, Math.min(data.to, b) - Math.max(data.from, a)) / total) * 100}%`;

  const ticks = $derived.by(() => {
    const out: { at: number; label: string }[] = [];
    const span = data.to - data.from;
    const day = 86400000;
    if (span <= day + 60000) {
      const hour = 3600000;
      const first = new Date(data.from);
      first.setMinutes(0, 0, 0);
      let at = first.getTime() + hour;
      while (new Date(at).getHours() % 6 !== 0) at += hour;
      for (; at < data.to - hour; at += 6 * hour) out.push({ at, label: time(at) });
    } else {
      const fmt = new Intl.DateTimeFormat(undefined, span > 8 * day ? { day: 'numeric', month: 'short' } : { weekday: 'short' });
      const step = span > 8 * day ? 6 * day : day;
      const start = new Date(data.from);
      start.setHours(0, 0, 0, 0);
      for (let at = start.getTime() + day; at < data.to; at += step) out.push({ at, label: fmt.format(at) });
    }
    return out;
  });
</script>

<div class="timeline">
  <div class="bar" role="img" aria-label={t('device.presence.online', { pct: Math.round(data.onlinePct) })}>
    {#each data.spans as s (s.start)}
      <span class="on" style:left={pct(s.start)} style:width={width(s.start, s.end || data.to)} title="{dateTime(s.start)} – {s.end ? dateTime(s.end) : t('device.status.online')}"></span>
    {/each}
    {#each data.gaps as g (g.start)}
      <span class="gap" style:left={pct(g.start)} style:width={width(g.start, g.end)} title={t('device.presence.gap')}></span>
    {/each}
  </div>
  <div class="ticks" aria-hidden="true">
    {#each ticks as tick (tick.at)}
      <span style:left={pct(tick.at)}>{tick.label}</span>
    {/each}
  </div>
</div>

<style>
  .timeline {
    display: grid;
    gap: 6px;
  }
  .bar {
    position: relative;
    height: 28px;
    border-radius: 6px;
    background: var(--surface-2);
    overflow: hidden;
  }
  .on,
  .gap {
    position: absolute;
    top: 0;
    bottom: 0;
    min-width: 2px;
  }
  .on {
    background: var(--ok);
    opacity: 0.85;
  }
  .gap {
    background: repeating-linear-gradient(135deg, var(--border-strong) 0 4px, transparent 4px 8px);
  }
  .ticks {
    position: relative;
    height: 16px;
    font-size: 11.5px;
    color: var(--text-3);
  }
  .ticks span {
    position: absolute;
    transform: translateX(-50%);
    white-space: nowrap;
  }

</style>
