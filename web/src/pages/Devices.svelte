<script lang="ts">
  import Search from '@lucide/svelte/icons/search';
  import X from '@lucide/svelte/icons/x';
  import DeviceRow from '../lib/components/DeviceRow.svelte';
  import EmptyState from '../lib/components/EmptyState.svelte';
  import ScanButton from '../lib/components/ScanButton.svelte';
  import DeviceDetail from './DeviceDetail.svelte';
  import { store } from '../lib/state.svelte';
  import { navigate, router } from '../lib/router.svelte';
  import { deviceTypes, displayName, ipKey, matches, typeLabel } from '../lib/devices';
  import { t } from '../lib/i18n.svelte';
  import type { Device } from '../lib/types';

  type Filter = 'all' | 'online' | 'offline' | 'new' | 'unreviewed' | 'favorites';
  type Group = 'none' | 'type' | 'location';
  type Sort = 'status' | 'name' | 'ip' | 'lastSeen' | 'firstSeen';

  function pref<T extends string>(key: string, fallback: T, allowed: readonly T[]): T {
    try {
      const v = localStorage.getItem(`netzwart.${key}`) as T | null;
      if (v && allowed.includes(v)) return v;
    } catch {
      /* ignore */
    }
    return fallback;
  }
  function savePref(key: string, v: string) {
    try {
      localStorage.setItem(`netzwart.${key}`, v);
    } catch {
      /* ignore */
    }
  }

  const filters: Filter[] = ['all', 'online', 'offline', 'new', 'unreviewed', 'favorites'];
  let query = $state('');
  let filter = $state<Filter>((router.query.get('filter') as Filter) ?? 'all');
  let typeFilter = $state<string>(router.query.get('type') ?? '');
  let group = $state<Group>(pref('group', 'none', ['none', 'type', 'location'] as const));
  let sort = $state<Sort>(pref('sort', 'status', ['status', 'name', 'ip', 'lastSeen', 'firstSeen'] as const));
  let searchEl: HTMLInputElement | undefined = $state();

  $effect(() => {
    const f = router.query.get('filter') as Filter | null;
    if (f && filters.includes(f)) filter = f;
    typeFilter = router.query.get('type') ?? '';
  });
  $effect(() => savePref('group', group));
  $effect(() => savePref('sort', sort));

  const all = $derived([...store.devices.values()]);
  const counts = $derived({
    all: all.length,
    online: all.filter((d) => d.online).length,
    offline: all.filter((d) => !d.online).length,
    new: all.filter((d) => d.new).length,
    unreviewed: all.filter((d) => !d.known && !d.isSelf).length,
    favorites: all.filter((d) => d.favorite).length,
  });

  function pass(d: Device): boolean {
    switch (filter) {
      case 'online':
        return d.online;
      case 'offline':
        return !d.online;
      case 'new':
        return d.new;
      case 'unreviewed':
        return !d.known && !d.isSelf;
      case 'favorites':
        return d.favorite;
    }
    return true;
  }

  const collator = new Intl.Collator(undefined, { sensitivity: 'base', numeric: true });
  function compare(a: Device, b: Device): number {
    switch (sort) {
      case 'name':
        return collator.compare(displayName(a), displayName(b));
      case 'ip':
        return ipKey(a.ip) - ipKey(b.ip);
      case 'lastSeen':
        return b.lastSeen - a.lastSeen;
      case 'firstSeen':
        return b.firstSeen - a.firstSeen;
    }
    // status: gateway and favorites first, then online, then name
    const rank = (d: Device) => (d.isGateway ? 0 : d.favorite ? 1 : 2);
    return rank(a) - rank(b) || Number(b.online) - Number(a.online) || collator.compare(displayName(a), displayName(b));
  }

  const visible = $derived(
    all.filter((d) => pass(d) && (!typeFilter || d.type === typeFilter) && matches(d, query.trim())).sort(compare),
  );

  const groups = $derived.by(() => {
    if (group === 'none') return [{ key: '', label: '', items: visible }];
    const m = new Map<string, Device[]>();
    for (const d of visible) {
      const key = group === 'type' ? d.type : d.location || '';
      if (!m.has(key)) m.set(key, []);
      m.get(key)!.push(d);
    }
    const keys = [...m.keys()];
    if (group === 'type') keys.sort((a, b) => deviceTypes.indexOf(a as never) - deviceTypes.indexOf(b as never));
    else keys.sort((a, b) => (a === '' ? 1 : b === '' ? -1 : collator.compare(a, b)));
    return keys.map((k) => ({
      key: k,
      label: group === 'type' ? typeLabel(k) : k || t('devices.noLocation'),
      items: m.get(k)!,
    }));
  });

  function setFilter(f: Filter) {
    filter = f;
    const q = new URLSearchParams(router.query);
    if (f === 'all') q.delete('filter');
    else q.set('filter', f);
    navigate(`/devices${router.route.name === 'devices' && router.route.id ? `/${router.route.id}` : ''}?${q}`, { replace: true });
  }

  function reset() {
    query = '';
    typeFilter = '';
    setFilter('all');
  }

  function onKey(e: KeyboardEvent) {
    const target = e.target as HTMLElement;
    const typing = target.closest('input, textarea, select, [contenteditable]');
    if (e.key === '/' && !typing) {
      e.preventDefault();
      searchEl?.focus();
    }
  }

  const selected = $derived(router.route.name === 'devices' ? router.route.id : undefined);
  const qs = $derived(router.query.toString() ? `?${router.query}` : '');
</script>

<svelte:window onkeydown={onKey} />

<div class="layout" class:with-detail={selected !== undefined}>
  <div class="page">
    <header class="head">
      <div>
        <h1>{t('nav.devices')}</h1>
        <p class="muted">{t('devices.count', { n: counts.all })}</p>
      </div>
      <ScanButton />
    </header>

    <div class="toolbar">
      <div class="search">
        <Search size={16} class="search-icon" />
        <input
          bind:this={searchEl}
          class="input"
          type="search"
          placeholder={t('devices.search')}
          aria-label={t('devices.search')}
          bind:value={query}
          onkeydown={(e) => e.key === 'Escape' && (query = '')}
        />
        <kbd class="kbd" aria-hidden="true">/</kbd>
      </div>
      <div class="selects">
        <select class="select" bind:value={group} aria-label={t('devices.group')}>
          <option value="none">{t('devices.group.none')}</option>
          <option value="type">{t('devices.group.type')}</option>
          <option value="location">{t('devices.group.location')}</option>
        </select>
        <select class="select" bind:value={sort} aria-label={t('devices.sort')}>
          {#each ['status', 'name', 'ip', 'lastSeen', 'firstSeen'] as s (s)}
            <option value={s}>{t(`devices.sort.${s}`)}</option>
          {/each}
        </select>
      </div>
    </div>

    <div class="filters" role="group" aria-label="Filter">
      <div class="seg scroll">
        {#each filters as f (f)}
          {#if f === 'all' || counts[f] > 0 || filter === f}
            <button type="button" aria-pressed={filter === f} onclick={() => setFilter(f)}>
              {t(`devices.filter.${f}`)}<span class="count">{counts[f]}</span>
            </button>
          {/if}
        {/each}
      </div>
      {#if typeFilter}
        <button class="badge accent type-pill" type="button" onclick={() => navigate(`/devices${filter !== 'all' ? `?filter=${filter}` : ''}`, { replace: true })}>
          {typeLabel(typeFilter)} <X size={12} />
        </button>
      {/if}
    </div>

    <section class="card list" aria-label={t('nav.devices')}>
      <div class="list-head" aria-hidden="true">
        <span></span><span>{t('devices.col.name')}</span><span>{t('devices.col.ip')}</span><span class="mac-col">{t('devices.col.mac')}</span><span class="right">{t('devices.col.seen')}</span>
      </div>
      {#if store.loading}
        {#each [1, 2, 3, 4, 5, 6] as i (i)}<div class="skeleton sk"></div>{/each}
      {:else if all.length === 0}
        <EmptyState text={t('devices.noneYet')} />
      {:else if visible.length === 0}
        <EmptyState text={query ? t('devices.empty', { q: query }) : t('devices.emptyFilter')}>
          <button class="btn" type="button" onclick={reset}>{t('devices.resetFilter')}</button>
        </EmptyState>
      {:else}
        {#each groups as g (g.key)}
          {#if g.label}
            <h2 class="group-label">{g.label} <span class="faint num">{g.items.length}</span></h2>
          {/if}
          {#each g.items as d (d.id)}
            <DeviceRow device={d} href="/devices/{d.id}{qs}" selected={selected === d.id} showMac={selected === undefined} />
          {/each}
        {/each}
      {/if}
    </section>
  </div>

  {#if selected !== undefined}
    <DeviceDetail id={selected} onclose={() => navigate(`/devices${qs}`)} />
  {/if}
</div>

<style>
  .layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    min-width: 0;
    gap: 20px;
    max-width: 1440px;
  }
  .layout.with-detail {
    grid-template-columns: minmax(0, 1fr) 480px;
  }
  /* The detail panel needs room: drop the MAC column while it is open. */
  .layout.with-detail .mac-col {
    display: none;
  }
  .layout.with-detail .list-head {
    grid-template-columns: 38px minmax(0, 1fr) 128px 110px;
  }
  .page {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 14px;
    align-content: start;
    min-width: 0;
  }
  .head {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    gap: 12px;
    flex-wrap: wrap;
  }
  .toolbar {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
  }
  .search {
    position: relative;
    flex: 1 1 280px;
  }
  .search :global(.search-icon) {
    position: absolute;
    left: 11px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--text-3);
    pointer-events: none;
  }
  .search .input {
    padding-left: 34px;
    padding-right: 34px;
  }
  .kbd {
    position: absolute;
    right: 9px;
    top: 50%;
    transform: translateY(-50%);
    font-family: var(--mono);
    font-size: 11px;
    padding: 1px 6px;
    border: 1px solid var(--border-strong);
    border-radius: 4px;
    color: var(--text-3);
  }
  .selects {
    display: flex;
    gap: 10px;
  }
  .selects .select {
    width: auto;
  }
  .filters {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }
  .scroll {
    min-width: 0;
    max-width: 100%;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .type-pill {
    border: 0;
    cursor: pointer;
    height: 26px;
    padding: 0 10px;
    font-size: 12.5px;
  }
  .list {
    padding: 6px;
  }
  .list-head {
    display: grid;
    grid-template-columns: 38px minmax(0, 1fr) 128px 150px 110px;
    gap: 14px;
    padding: 6px 10px 8px;
    font-size: 12px;
    font-weight: 550;
    color: var(--text-3);
    border-bottom: 1px solid var(--border);
    margin-bottom: 4px;
  }
  .right {
    text-align: right;
  }
  .group-label {
    display: flex;
    gap: 8px;
    align-items: baseline;
    padding: 14px 10px 4px;
    font-size: 12.5px;
    font-weight: 600;
    color: var(--text-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .sk {
    height: 50px;
    margin: 4px;
  }
  @media (max-width: 1279px) {
    .list-head {
      grid-template-columns: 38px minmax(0, 1fr) 120px 100px;
    }
    .mac-col {
      display: none;
    }
    .layout.with-detail {
      grid-template-columns: minmax(0, 1fr) 420px;
    }
  }
  @media (max-width: 1100px) {
    .layout.with-detail {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  @media (max-width: 639px) {
    .list-head {
      display: none;
    }
    /* Hint that the filter chips continue to the right. */
    .scroll {
      mask-image: linear-gradient(to right, #000 82%, transparent);
      padding-right: 28px;
    }
    .kbd {
      display: none;
    }
    .selects {
      width: 100%;
    }
    .selects .select {
      flex: 1;
    }
    .list {
      padding: 4px;
    }
  }
</style>
