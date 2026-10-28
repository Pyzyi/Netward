<script lang="ts">
  import type { Presence } from '../types';
  import { dateTime, time, t } from '../i18n.svelte';

  // since: when Netward first saw the device; before that nothing is known.
  let { data, since = 0 }: { data: Presence; since?: number } = $props();
  const unknownUntil = $derived(Math.min(data.to, Math.max(data.from, since)));

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
      // Step in wall-clock hours (setHours), so ticks stay on 00/06/12/18
      // across daylight-saving changes.
      const at = new Date(data.from);
      at.setMinutes(0, 0, 0);
      at.setHours(at.getHours() + 1);
      while (at.getHours() % 6 !== 0) at.setHours(at.getHours() + 1);
      for (; at.getTime() < data.to - 3 * hour; at.setHours(at.getHours() + 6)) out.push({ at: at.getTime(), label: time(at.getTime()) });
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
    {#if unknownUntil > data.from}
      <span class="unknown" style:left="0" style:width={width(data.from, unknownUntil)} title={t('device.presence.unknown')}></span>
    {/if}
    {#each data.gaps as g (g.start)}
      <span class="gap" style:left={pct(g.start)} style:width={width(g.start, g.end)} title={t('device.presence.gap')}></span>
    {/each}
  </div>
  <div class="ticks" aria-hidden="true">
    {#each ticks as tick (tick.at)}
      <span style:left={pct(tick.at)}>{tick.label}</span>
    {/each}
    <span class="now">{t('device.presence.now')}</span>
  </div>
  <ul class="legend">
    <li><span class="sw on-sw"></span>{t('device.status.online')}</li>
    <li><span class="sw off-sw"></span>{t('common.offline')}</li>
    {#if data.gaps.length}<li><span class="sw gap-sw"></span>{t('device.presence.gap')}</li>{/if}
    {#if unknownUntil > data.from}<li><span class="sw unknown-sw"></span>{t('device.presence.unknown')}</li>{/if}
  </ul>
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
  .gap,
  .unknown {
    position: absolute;
    top: 0;
    bottom: 0;
    min-width: 2px;
  }
  .on {
    background: var(--signal);
  }
  .gap,
  .gap-sw {
    background: repeating-linear-gradient(135deg, var(--control-border) 0 3px, transparent 3px 7px);
  }
  .unknown,
  .unknown-sw {
    background: radial-gradient(var(--border-strong) 1px, transparent 1.2px) 0 0 / 6px 6px, var(--surface);
  }
  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    margin: 4px 0 0;
    padding: 0;
    list-style: none;
    font-size: 13px;
    color: var(--text-2);
  }
  .legend li {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .sw {
    width: 14px;
    height: 10px;
    border-radius: 3px;
    border: 1px solid var(--border);
  }
  .on-sw {
    background: var(--signal);
  }
  .off-sw {
    background: var(--surface-3);
    border-color: var(--border-strong);
  }
  .ticks .now {
    right: 0;
    transform: none;
  }
  .ticks {
    position: relative;
    height: 16px;
    font-size: 11px;
    color: var(--text-3);
  }
  .ticks span {
    position: absolute;
    transform: translateX(-50%);
    white-space: nowrap;
  }

</style>
