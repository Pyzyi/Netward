<script lang="ts">
  import Icon from '../lib/components/Icon.svelte';
  import EmptyState from '../lib/components/EmptyState.svelte';
  import { api } from '../lib/api';
  import { clock, store } from '../lib/state.svelte';
  import { link } from '../lib/router.svelte';
  import { collapse, eventGroup, eventText, groupKinds, type EventGroup, type EventRun } from '../lib/events';
  import { dayLabel, time, t } from '../lib/i18n.svelte';
  import { displayName } from '../lib/devices';
  import type { EventItem } from '../lib/types';

  type Filter = 'all' | EventGroup;
  const filters: Filter[] = ['all', 'devices', 'presence', 'internet', 'security', 'system'];
  let filter = $state<Filter>('all');
  let items = $state<EventItem[]>([]);
  let loading = $state(true);
  let more = $state(true);

  async function load(reset = false) {
    loading = true;
    try {
      const before = reset ? undefined : items.at(-1)?.id;
      const kinds = filter === 'all' ? undefined : groupKinds[filter];
      const page = await api.events({ before, kinds, limit: 60 });
      items = reset ? page : [...items, ...page];
      more = page.length === 60;
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void filter;
    void load(true);
  });

  // Live: prepend new events that match the filter.
  let seenTick = store.eventTick;
  $effect(() => {
    if (store.eventTick === seenTick) return;
    seenTick = store.eventTick;
    const latest = store.recent[0];
    if (latest && !items.some((i) => i.id === latest.id) && (filter === 'all' || eventGroup(latest.kind) === filter)) {
      items = [latest, ...items];
    }
  });

  const textOf = (e: EventItem) => {
    const dev = e.deviceId ? store.devices.get(e.deviceId) : undefined;
    return eventText(e, dev ? displayName(dev) : undefined);
  };
  // Per day, with consecutive repeats ("Anmeldung von 127.0.0.1" ×12) folded into one row.
  const grouped = $derived.by(() => {
    const days: { day: string; items: EventItem[] }[] = [];
    for (const e of items) {
      const day = dayLabel(e.ts, clock.now);
      if (days.at(-1)?.day !== day) days.push({ day, items: [] });
      days.at(-1)!.items.push(e);
    }
    return days.map((d) => ({ day: d.day, count: d.items.length, runs: collapse(d.items, textOf) as EventRun[] }));
  });

  const icons: Record<string, string> = {
    device_new: 'user-plus', device_online: 'sign-in', device_offline: 'sign-out', ip_changed: 'shuffle',
    ip_conflict: 'warning', internet_down: 'wifi-slash', internet_up: 'globe-simple', security_finding: 'shield-warning',
    device_flood: 'warning', device_removed: 'trash', device_merged: 'git-merge', scan_baseline: 'broadcast',
    auth_login: 'key', auth_failed: 'shield-warning', auth_setup: 'check-circle', auth_password: 'key',
    config_settings: 'gear-six', config_channel: 'gear-six', config_network: 'gear-six',
    gateway_changed: 'shield-warning', dhcp_server_new: 'broadcast', identity_changed: 'shield-warning', port_new: 'plus',
  };
</script>

<div class="wrap">
  <header class="page-head">
    <h1>{t('activity.title')}</h1>
    <div class="seg filters" role="group" aria-label="Filter">
      {#each filters as f (f)}
        <button type="button" aria-pressed={filter === f} onclick={() => (filter = f)}>{t(`activity.filter.${f}`)}</button>
      {/each}
    </div>
  </header>

  {#if loading && items.length === 0}
    {#each [1, 2, 3, 4, 5] as i (i)}<div class="skeleton sk"></div>{/each}
  {:else if items.length === 0}
    <EmptyState text={t('activity.empty')} />
  {:else}
    {#each grouped as g, gi (`${g.day}-${gi}`)}
      <section class="day">
        <h2 class="day-label">{g.day} <span class="count-label">{g.count}</span></h2>
        <ol>
          {#each g.runs as run (run.e.id)}
            {@const e = run.e}
            {@const dev = e.deviceId ? store.devices.get(e.deviceId) : undefined}
            <li class={e.severity}>
              <time class="num" datetime={new Date(e.ts).toISOString()}>
                {time(e.ts)}{#if run.count > 1 && time(run.firstTs) !== time(e.ts)}<span class="from">{t('activity.since', { t: time(run.firstTs) })}</span>{/if}
              </time>
              <span class="ev-icon"><Icon name={icons[e.kind] ?? 'gear-six'} size={16} /></span>
              <span class="text">
                {#if dev}<a href="/devices/{dev.id}" use:link>{textOf(e)}</a>{:else}{textOf(e)}{/if}
                {#if e.kind === 'ip_changed' && e.data.from}<span class="faint"> ({t('activity.before')} {e.data.from})</span>{/if}
                {#if run.count > 1}<span class="faint num"> · {run.count}×</span>{/if}
              </span>
            </li>
          {/each}
        </ol>
      </section>
    {/each}
    {#if more}
      <div class="more"><button class="btn" type="button" onclick={() => load()} disabled={loading}>{t('activity.loadMore')}</button></div>
    {/if}
  {/if}
</div>

<style>
  .filters {
    overflow-x: auto;
    scrollbar-width: none;
  }
  .sk {
    height: 40px;
    margin: 6px 0;
  }
  .day {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .day + .day {
    margin-top: 16px;
  }
  .day-label {
    display: flex;
    align-items: baseline;
    gap: 8px;
    padding: 12px 16px;
    border-bottom: 1px solid var(--border);
    font-size: 14px;
  }
  ol {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    display: grid;
    grid-template-columns: 48px 28px minmax(0, 1fr);
    align-items: center;
    gap: 12px;
    padding: 8px 16px;
  }
  li + li {
    border-top: 1px solid var(--border);
  }
  time {
    align-self: start;
    padding-top: 5px;
    color: var(--text-2);
    font-size: 13px;
    line-height: 1.3;
  }
  .from {
    display: block;
    font-size: 12px;
  }
  .ev-icon {
    align-self: start;
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border-radius: 7px;
    color: var(--text-2);
  }
  li.warning .ev-icon {
    background: var(--warn-soft);
    color: var(--warn);
  }
  li.critical .ev-icon {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .text {
    min-width: 0;
    padding-top: 3px;
    overflow-wrap: anywhere;
  }
  .text a {
    color: var(--text);
    text-decoration: none;
  }
  .text a:hover {
    text-decoration: underline;
  }
  .text .faint {
    font-size: 13px;
  }
  .more {
    display: flex;
    justify-content: center;
    padding: 20px 0 0;
  }
  @media (max-width: 640px) {
    li {
      grid-template-columns: 40px 28px minmax(0, 1fr);
      gap: 10px;
      padding: 8px 12px;
    }
  }
</style>
