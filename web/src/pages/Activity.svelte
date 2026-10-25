<script lang="ts">
  import UserPlus from '@lucide/svelte/icons/user-plus';
  import LogIn from '@lucide/svelte/icons/log-in';
  import LogOut from '@lucide/svelte/icons/log-out';
  import Shuffle from '@lucide/svelte/icons/shuffle';
  import Globe from '@lucide/svelte/icons/globe';
  import GlobeLock from '@lucide/svelte/icons/globe-lock';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import Settings from '@lucide/svelte/icons/settings';
  import Trash from '@lucide/svelte/icons/trash-2';
  import Radar from '@lucide/svelte/icons/radar';
  import KeyRound from '@lucide/svelte/icons/key-round';
  import EmptyState from '../lib/components/EmptyState.svelte';
  import { api } from '../lib/api';
  import { clock, store } from '../lib/state.svelte';
  import { link } from '../lib/router.svelte';
  import { eventGroup, eventText, groupKinds, type EventGroup } from '../lib/events';
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

  const grouped = $derived.by(() => {
    const out: { day: string; items: EventItem[] }[] = [];
    for (const e of items) {
      const day = dayLabel(e.ts, clock.now);
      if (out.at(-1)?.day !== day) out.push({ day, items: [] });
      out.at(-1)!.items.push(e);
    }
    return out;
  });

  const icons: Record<string, typeof UserPlus> = {
    device_new: UserPlus, device_online: LogIn, device_offline: LogOut, ip_changed: Shuffle, ip_conflict: ShieldAlert,
    internet_down: GlobeLock, internet_up: Globe, security_finding: ShieldAlert, device_flood: ShieldAlert,
    device_removed: Trash, device_merged: Shuffle, scan_baseline: Radar, auth_login: KeyRound, auth_failed: ShieldAlert,
    auth_setup: KeyRound, auth_password: KeyRound, config_settings: Settings, config_channel: Settings,
  };
</script>

<div class="page">
  <header>
    <h1>{t('activity.title')}</h1>
  </header>
  <div class="seg scroll" role="group" aria-label="Filter">
    {#each filters as f (f)}
      <button type="button" aria-pressed={filter === f} onclick={() => (filter = f)}>{t(`activity.filter.${f}`)}</button>
    {/each}
  </div>

  <section class="card feed">
    {#if loading && items.length === 0}
      {#each [1, 2, 3, 4, 5] as i (i)}<div class="skeleton sk"></div>{/each}
    {:else if items.length === 0}
      <EmptyState text={t('activity.empty')} />
    {:else}
      {#each grouped as g, gi (`${g.day}-${gi}`)}
        <h2 class="day">{g.day}</h2>
        <ul>
          {#each g.items as e (e.id)}
            {@const Icon = icons[e.kind] ?? Settings}
            {@const dev = e.deviceId ? store.devices.get(e.deviceId) : undefined}
            <li>
              <span class="ev-icon {e.severity}"><Icon size={16} /></span>
              <span class="text">
                {#if dev}<a href="/devices/{dev.id}" use:link>{eventText(e, displayName(dev))}</a>{:else}{eventText(e)}{/if}
                {#if e.kind === 'ip_changed' && e.data.from}<span class="faint mono small"> ({e.data.from} → {e.data.to})</span>{/if}
              </span>
              <time class="faint num" datetime={new Date(e.ts).toISOString()}>{time(e.ts)}</time>
            </li>
          {/each}
        </ul>
      {/each}
      {#if more}
        <div class="more"><button class="btn" type="button" onclick={() => load()} disabled={loading}>{t('activity.loadMore')}</button></div>
      {/if}
    {/if}
  </section>
</div>

<style>
  .page {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 16px;
    max-width: 920px;
  }
  .scroll {
    min-width: 0;
    max-width: 100%;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .feed {
    padding: 4px 0 8px;
  }
  .day {
    position: sticky;
    top: 0;
    padding: 14px 20px 6px;
    font-size: 12.5px;
    font-weight: 600;
    color: var(--text-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    background: var(--surface);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0 12px;
  }
  li {
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: 12px;
    align-items: center;
    padding: 8px;
    border-radius: 8px;
  }
  li:hover {
    background: var(--surface-2);
  }
  .ev-icon {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border-radius: 50%;
    background: var(--surface-2);
    color: var(--text-2);
  }
  .ev-icon.warning {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .ev-icon.critical {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .text {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .text a {
    color: inherit;
  }
  .small {
    font-size: 12.5px;
  }
  .sk {
    height: 40px;
    margin: 8px 16px;
  }
  .more {
    display: flex;
    justify-content: center;
    padding: 12px;
  }
</style>
