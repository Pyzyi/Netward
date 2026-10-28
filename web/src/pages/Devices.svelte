<script lang="ts">
  import Icon from '../lib/components/Icon.svelte';
  import DeviceRow from '../lib/components/DeviceRow.svelte';
  import EmptyState from '../lib/components/EmptyState.svelte';
  import ScanButton from '../lib/components/ScanButton.svelte';
  import DeviceDetail from './DeviceDetail.svelte';
  import { store } from '../lib/state.svelte';
  import { link, navigate, router } from '../lib/router.svelte';
  import { deviceTypes, displayName, ipKey, matches, typeLabel } from '../lib/devices';
  import { t } from '../lib/i18n.svelte';
  import type { Device } from '../lib/types';

  type Filter = 'all' | 'online' | 'offline' | 'new' | 'unreviewed' | 'software' | 'favorites';
  type Group = 'none' | 'type' | 'location';
  type Sort = 'status' | 'name' | 'ip' | 'lastSeen' | 'firstSeen';

  function pref<T extends string>(key: string, fallback: T, allowed: readonly T[]): T {
    try {
      const v = localStorage.getItem(`netward.${key}`) as T | null;
      if (v && allowed.includes(v)) return v;
    } catch {
      /* ignore */
    }
    return fallback;
  }
  function savePref(key: string, v: string) {
    try {
      localStorage.setItem(`netward.${key}`, v);
    } catch {
      /* ignore */
    }
  }

  const filters: Filter[] = ['all', 'online', 'offline', 'new', 'unreviewed', 'software', 'favorites'];
  // "Updates nötig": no security updates any more, or an update is waiting.
  const needsUpdate = (d: Device) => d.software?.status === 'outdated' || d.software?.status === 'update';
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
    software: all.filter(needsUpdate).length,
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
      case 'software':
        return needsUpdate(d);
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
  <div class="register">
    <header class="page-head">
      <div>
        <h1>{t('nav.devices')} <span class="total num">{counts.all}</span></h1>
        <p class="lead">{t('devices.lead', { online: counts.online, offline: counts.offline })}</p>
      </div>
      <div class="head-actions">
        <a class="btn" href="/wlan" use:link><Icon name="wifi-high" size={16} /> {t('devices.wlanLink')}</a>
        <ScanButton />
      </div>
    </header>

    <div class="toolbar">
      <div class="search">
        <Icon name="magnifying-glass" size={17} class="search-icon" />
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
            <option value={s}>{t('devices.sort')}: {t(`devices.sort.${s}`)}</option>
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
        <button class="btn small type-pill" type="button" onclick={() => navigate(`/devices${filter !== 'all' ? `?filter=${filter}` : ''}`, { replace: true })}>
          {typeLabel(typeFilter)} <Icon name="x" size={12} />
        </button>
      {/if}
    </div>

    <section class="list" aria-label={t('nav.devices')}>
      <div class="list-head eyebrow" class:narrow={selected !== undefined} aria-hidden="true">
        <span></span><span>{t('devices.col.name')}</span><span>{t('devices.col.ip')}</span><span class="beat-col">{t('devices.col.beat')}</span><span class="right">{t('devices.col.seen')}</span>
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
            <h2 class="group-label">{g.label} <span class="num">{g.items.length}</span></h2>
          {/if}
          {#each g.items as d (d.id)}
            <DeviceRow device={d} href="/devices/{d.id}{qs}" selected={selected === d.id} wide={selected === undefined} />
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
    max-width: var(--page);
    margin: 0 auto;
    padding: 28px 32px 56px;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 20px;
    min-width: 0;
  }
  .layout.with-detail {
    grid-template-columns: minmax(0, 1fr) 470px;
  }
  .register {
    min-width: 0;
  }
  h1 {
    display: flex;
    align-items: baseline;
    gap: 12px;
  }
  .total {
    font-weight: 400;
    color: var(--text-2);
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
    color: var(--text-2);
    pointer-events: none;
  }
  .search .input {
    padding-left: 36px;
    padding-right: 34px;
    background: var(--surface);
  }
  .kbd {
    position: absolute;
    right: 9px;
    top: 50%;
    transform: translateY(-50%);
    font-family: var(--mono);
    font-size: 11px;
    line-height: 18px;
    padding: 0 6px;
    border: 1px solid var(--border-strong);
    border-radius: 4px;
    color: var(--text-2);
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
    margin: 12px 0 0;
  }
  .scroll {
    min-width: 0;
    max-width: 100%;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .type-pill {
    gap: 6px;
  }
  .list {
    margin-top: 12px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
  }
  .list-head {
    display: grid;
    grid-template-columns: 36px minmax(0, 1fr) 140px 192px 104px;
    gap: 16px;
    padding: 9px 16px 8px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .list-head.narrow {
    grid-template-columns: 36px minmax(0, 1fr) 120px 104px;
  }
  .list-head.narrow .beat-col {
    display: none;
  }
  .right {
    text-align: right;
  }
  .group-label {
    display: flex;
    gap: 8px;
    align-items: baseline;
    padding: 8px 16px;
    border-bottom: 1px solid var(--border);
    background: var(--surface-2);
    color: var(--text);
    font-size: 13px;
    font-weight: 600;
  }
  .group-label .num {
    font-weight: 400;
    color: var(--text-2);
  }
  .sk {
    height: 44px;
    margin: 8px 16px;
  }
  .list :global(.empty) {
    padding: 40px 16px;
  }
  @media (max-width: 1280px) {
    .layout.with-detail {
      grid-template-columns: minmax(0, 1fr) 420px;
    }
  }
  @media (max-width: 900px) {
    .list-head {
      grid-template-columns: 38px minmax(0, 1fr) 120px 104px;
    }
    .beat-col {
      display: none;
    }
  }
  @media (max-width: 1100px) {
    .layout.with-detail {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  @media (max-width: 639px) {
    .layout {
      padding: 20px 16px 40px;
    }
    .list-head {
      display: none;
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
    /* Hint that the filter tabs continue to the right. */
    .scroll {
      mask-image: linear-gradient(to right, #000 82%, transparent);
      padding-right: 28px;
    }
  }
  .head-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: 10px;
  }
</style>
