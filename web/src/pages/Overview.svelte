<script lang="ts">
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import Info from '@lucide/svelte/icons/info';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Check from '@lucide/svelte/icons/check';
  import Globe from '@lucide/svelte/icons/globe';
  import DeviceRow from '../lib/components/DeviceRow.svelte';
  import DeviceIcon from '../lib/components/DeviceIcon.svelte';
  import ScanButton from '../lib/components/ScanButton.svelte';
  import Sparkline from '../lib/components/Sparkline.svelte';
  import EmptyState from '../lib/components/EmptyState.svelte';
  import Dialog from '../lib/components/Dialog.svelte';
  import { toast } from '../lib/components/toast.svelte';
  import { api } from '../lib/api';
  import { auth, clock, store } from '../lib/state.svelte';
  import { link } from '../lib/router.svelte';
  import { deviceTypes, displayName, typeLabel } from '../lib/devices';
  import { eventText } from '../lib/events';
  import { number, relTime, t } from '../lib/i18n.svelte';
  import type { DeviceType } from '../lib/types';

  const devices = $derived([...store.devices.values()]);
  const online = $derived(devices.filter((d) => d.online).length);
  const fresh = $derived(
    devices.filter((d) => d.new).sort((a, b) => b.firstSeen - a.firstSeen),
  );
  const ov = $derived(store.overview);
  const scanner = $derived(ov?.scanner);
  const active = $derived(scanner?.interfaces?.filter((i) => i.active) ?? []);
  const byType = $derived.by(() => {
    const m = new Map<DeviceType, number>();
    for (const d of devices) m.set(d.type, (m.get(d.type) ?? 0) + 1);
    return deviceTypes.filter((ty) => m.has(ty)).map((ty) => ({ type: ty, count: m.get(ty)! }));
  });

  let history = $state<{ t: number; ms: number }[]>([]);
  async function loadHistory() {
    try {
      history = (await api.internet()).history.slice(-240);
    } catch {
      /* not critical */
    }
  }
  $effect(() => {
    void loadHistory();
    const id = setInterval(loadHistory, 60_000);
    return () => clearInterval(id);
  });

  async function markKnown(id: number) {
    try {
      await api.patchDevice(id, { known: true });
    } catch {
      toast(t('common.error'), 'error');
    }
  }
  let confirmAll = $state(false);
  async function markAll() {
    confirmAll = false;
    try {
      const r = await api.acknowledgeAll();
      toast(t('settings.ackDone', { n: r.count }));
    } catch {
      toast(t('common.error'), 'error');
    }
  }

  // Alerts that silently go nowhere are worse than none: say so.
  let noChannel = $state(false);
  $effect(() => {
    if (!store.settings?.notifyNewDevice || scanner?.mode === 'demo') return;
    api.channels().then((r) => (noChannel = !r.channels.some((c) => c.enabled)), () => {});
  });

  // Sign-ins are recorded in the activity log but are noise on the overview.
  const feed = $derived(store.recent.filter((e) => e.kind !== 'auth_login').slice(0, 8));

  const inetState = $derived(ov?.internet.state ?? 'unknown');
  const inetOk = $derived(inetState === 'online');
  const secIssues = $derived((ov?.security.critical ?? 0) + (ov?.security.warning ?? 0));
  const deviceName = (id?: number) => (id ? store.devices.get(id) : undefined);
</script>

<Dialog bind:open={confirmAll} title={t('overview.markAllTitle', { n: fresh.length })}>
  <p>{t('overview.markAllText')}</p>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (confirmAll = false)}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" onclick={markAll}>{t('overview.markAllKnown')}</button>
  {/snippet}
</Dialog>

<div class="page">
  {#if noChannel}
    <div class="notice warn channel-notice" role="status">
      <TriangleAlert size={17} />
      <span>{t('settings.channelsMissing')}</span>
      <a class="btn small" href="/settings/notifications" use:link>{t('settings.channelsSetup')}</a>
    </div>
  {/if}
  {#if scanner?.mode === 'demo'}
    <div class="notice info"><Info size={17} /> {t('overview.demo')}</div>
  {:else if scanner?.mode === 'fallback'}
    <div class="notice warn"><TriangleAlert size={17} /> {t('overview.fallback')}</div>
  {/if}
  {#if scanner?.error}
    <div class="notice danger"><TriangleAlert size={17} /> {t('overview.scanError', { msg: scanner.error })}</div>
  {/if}
  {#each scanner?.interfaces?.filter((i) => i.active && (i.skipped === 'large' || i.skipped === 'too_large')) ?? [] as i (i.name)}
    <div class="notice warn"><TriangleAlert size={17} /> {t('overview.subnetSkipped', { p: i.prefix })}</div>
  {/each}
  {#if ov && !ov.baseline && scanner?.mode !== 'demo'}
    <div class="notice info"><Info size={17} /> {t('overview.learning')}</div>
  {/if}

  <header class="head">
    <div class="title">
      <h1>{ov?.networkName || store.settings?.networkName || t('overview.unnamed')}</h1>
      <p class="muted meta">
        {#each active as i (i.name)}
          <span class="mono">{i.prefix}</span><span class="sep">·</span><span>{i.name}</span><span class="sep">·</span>
        {/each}
        {#if scanner?.gateways?.length}
          <span>{t('overview.gateway')} <span class="mono">{scanner.gateways[0]}</span></span>
        {/if}
      </p>
    </div>
    <div class="scan">
      <ScanButton />
      {#if scanner?.lastScanAt}
        <span class="faint small">{t('overview.lastScan', { t: relTime(scanner.lastScanAt, clock.now) })}</span>
      {/if}
    </div>
  </header>

  <section class="tiles" aria-label="Status">
    <a class="tile card" href="/devices?filter=online" use:link>
      <span class="tile-label"><span class="dot on"></span>{t('overview.online')}</span>
      <span class="big num">{store.loading ? '–' : number(online)}</span>
      <span class="faint small">{t('overview.devicesTotal', { n: devices.length })}</span>
    </a>
    <a class="tile card" href="/devices?filter=offline" use:link>
      <span class="tile-label"><span class="dot"></span>{t('overview.offline')}</span>
      <span class="big num">{store.loading ? '–' : number(devices.length - online)}</span>
      <span class="faint small">{t('overview.devicesTotal', { n: devices.length })}</span>
    </a>
    <a class="tile card" href="/devices?filter=new" use:link>
      <span class="tile-label"><span class="dot new"></span>{t('overview.new')}</span>
      <span class="big num" class:warn={fresh.length > 0}>{store.loading ? '–' : number(fresh.length)}</span>
      <span class="faint small">{t('overview.newHint')}</span>
    </a>
    <a class="tile card" href="/settings/internet" use:link>
      <span class="tile-label"><Globe size={14} />{t('overview.internet')}</span>
      <span class="inet">
        <span class="inet-state" class:ok={inetOk} class:bad={inetState === 'offline' || inetState === 'lan_down'} class:warn={inetState === 'degraded'}>
          {t(`internet.${inetState}`)}
        </span>
        {#if inetOk && ov?.internet.latencyMs !== undefined && ov.internet.latencyMs >= 0}
          <span class="muted num">{t('internet.latency', { ms: ov.internet.latencyMs })}</span>
        {/if}
      </span>
      {#if history.length > 2}<Sparkline points={history} height={30} />{/if}
    </a>
  </section>

  <div class="grid">
    <div class="col">
    <section class="card attention" aria-labelledby="att">
      <div class="card-head">
        <h2 id="att">{t('overview.attention')}</h2>
        {#if fresh.length > 1}<button class="btn small" type="button" onclick={() => (confirmAll = true)}><Check size={15} /> {t('overview.markAllKnown')}</button>{/if}
      </div>
      <div class="list">
        {#if store.loading}
          {#each [1, 2, 3] as i (i)}<div class="skeleton sk-row"></div>{/each}
        {:else if fresh.length === 0}
          <EmptyState text={t('overview.attentionEmpty')} />
        {:else}
          {#each fresh.slice(0, 6) as d (d.id)}
            <DeviceRow device={d} href="/devices/{d.id}" compact>
              {#snippet actions()}
                <button class="btn small" type="button" onclick={() => markKnown(d.id)}><Check size={15} /> {t('overview.markKnown')}</button>
              {/snippet}
            </DeviceRow>
          {/each}
          {#if fresh.length > 6}
            <a class="more" href="/devices?filter=new" use:link>{t('common.showAll')} ({fresh.length}) <ChevronRight size={15} /></a>
          {/if}
        {/if}
      </div>
    </section>

    <section class="card" aria-labelledby="types">
      <div class="card-head"><h2 id="types">{t('overview.byType')}</h2></div>
      <div class="types">
        {#each byType as bt (bt.type)}
          <a class="type-chip" href="/devices?type={bt.type}" use:link>
            <DeviceIcon type={bt.type} size={30} />
            <span class="type-name">{typeLabel(bt.type)}</span>
            <span class="num muted">{bt.count}</span>
          </a>
        {/each}
      </div>
    </section>
    </div>

    <div class="col">
    <section class="card" aria-labelledby="act">
      <div class="card-head">
        <h2 id="act">{t('overview.activity')}</h2>
        <a class="small" href="/activity" use:link>{t('common.showAll')}</a>
      </div>
      <ul class="feed">
        {#each feed as e (e.id)}
          {@const dev = deviceName(e.deviceId)}
          <li>
            <span class="feed-dot {e.severity}"></span>
            <span class="feed-text">
              {#if dev}
                <a href="/devices/{dev.id}" use:link>{eventText(e, displayName(dev))}</a>
              {:else}
                {eventText(e)}
              {/if}
            </span>
            <time class="faint small" datetime={new Date(e.ts).toISOString()}>{relTime(e.ts, clock.now)}</time>
          </li>
        {:else}
          <li class="muted">{t('overview.activityEmpty')}</li>
        {/each}
      </ul>
    </section>

    

    <section class="card security" aria-labelledby="sec">
      <div class="card-head"><h2 id="sec">{t('overview.security')}</h2></div>
      <a class="sec-body" href="/security" use:link>
        {#if secIssues > 0}
          <span class="sec-icon bad"><ShieldAlert size={22} /></span>
          <span class="sec-text">
            <strong>
              {#if ov?.security.critical}{t('security.count.critical', { n: ov.security.critical })}{/if}
              {#if ov?.security.critical && ov?.security.warning} · {/if}
              {#if ov?.security.warning}{t('security.count.warning', { n: ov.security.warning })}{/if}
            </strong>
            <span class="faint small">{ov?.security.lastRunAt ? t('overview.securityLast', { t: relTime(ov.security.lastRunAt, clock.now) }) : t('overview.securityNever')}</span>
          </span>
        {:else}
          <span class="sec-icon ok"><ShieldCheck size={22} /></span>
          <span class="sec-text">
            <strong>{ov?.security.lastRunAt ? t('overview.securityOk') : t('overview.securityNever')}</strong>
            {#if ov?.security.lastRunAt}<span class="faint small">{t('overview.securityLast', { t: relTime(ov.security.lastRunAt, clock.now) })}</span>{/if}
          </span>
        {/if}
        <ChevronRight size={18} class="chev" />
      </a>
      {#if auth.state && !auth.state.https}
        <p class="hint http">{t('overview.http')}</p>
      {/if}
    </section>
    </div>
  </div>
</div>

<style>
  .page {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 20px;
    max-width: 1280px;
  }
  .head {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 16px;
    flex-wrap: wrap;
  }
  .title {
    display: grid;
    gap: 6px;
    min-width: 0;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 6px;
    font-size: 13.5px;
  }
  .sep {
    color: var(--text-3);
  }
  .scan {
    display: grid;
    justify-items: end;
    gap: 6px;
  }
  .small {
    font-size: 12.5px;
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 12px;
  }
  .tile {
    display: grid;
    align-content: start;
    gap: 4px;
    padding: 14px 16px;
    color: inherit;
    text-decoration: none;
    transition: border-color 120ms var(--ease);
  }
  .tile:hover {
    border-color: var(--border-strong);
    text-decoration: none;
  }
  .tile-label {
    display: flex;
    align-items: center;
    gap: 7px;
    color: var(--text-2);
    font-weight: 550;
    font-size: 13px;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--off);
  }
  .dot.on {
    background: var(--ok);
  }
  .dot.new {
    background: var(--warn);
  }
  .big {
    font-size: 30px;
    font-weight: 620;
    letter-spacing: -0.02em;
    line-height: 1.15;
  }
  .big.warn {
    color: var(--warn);
  }
  .inet {
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-height: 34px;
    padding-top: 6px;
  }
  .inet-state {
    font-size: 17px;
    font-weight: 600;
  }
  .inet-state.ok {
    color: var(--ok);
  }
  .inet-state.bad {
    color: var(--danger);
  }
  .inet-state.warn {
    color: var(--warn);
  }
  .grid {
    display: grid;
    grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
    gap: 16px;
    align-items: start;
  }
  .col {
    display: grid;
    gap: 16px;
    min-width: 0;
  }
  .list {
    display: grid;
    gap: 2px;
    padding: 0 8px 10px;
  }
  .sk-row {
    height: 48px;
    margin: 2px 6px;
  }
  .more {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 8px 8px 2px;
    font-size: 13px;
  }
  .feed {
    list-style: none;
    margin: 0;
    padding: 0 16px 14px;
    display: grid;
    gap: 11px;
  }
  .feed li {
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: 10px;
    align-items: baseline;
  }
  .feed-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--border-strong);
    transform: translateY(-1px);
  }
  .feed-dot.warning {
    background: var(--warn);
  }
  .feed-dot.critical {
    background: var(--danger);
  }
  .feed-text {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .feed-text a {
    color: inherit;
  }
  .channel-notice {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }
  .channel-notice span {
    flex: 1 1 260px;
  }
  .types {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
    gap: 6px;
    padding: 0 12px 14px;
  }
  /* On phones the type list becomes compact chips so activity stays close. */
  @media (max-width: 639px) {
    .types {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
    }
    .type-chip {
      gap: 6px;
      padding: 4px 10px 4px 4px;
      border: 1px solid var(--border);
      border-radius: 999px;
    }
    .type-name {
      flex: none;
    }
  }
  .type-chip {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 8px;
    border-radius: 8px;
    color: inherit;
    text-decoration: none;
  }
  .type-chip:hover {
    background: var(--surface-2);
    text-decoration: none;
  }
  .type-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sec-body {
    display: flex;
    align-items: center;
    gap: 12px;
    margin: 0 12px 12px;
    padding: 10px;
    border-radius: 8px;
    color: inherit;
    text-decoration: none;
  }
  .sec-body:hover {
    background: var(--surface-2);
    text-decoration: none;
  }
  .sec-icon {
    display: grid;
    place-items: center;
    width: 42px;
    height: 42px;
    border-radius: 50%;
    flex: none;
  }
  .sec-icon.ok {
    background: var(--ok-soft);
    color: var(--ok);
  }
  .sec-icon.bad {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .sec-text {
    display: grid;
    flex: 1;
  }
  .sec-body :global(.chev) {
    color: var(--text-3);
  }
  .http {
    padding: 0 16px 14px;
  }
  @media (max-width: 1100px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
  @media (max-width: 760px) {
    .tiles {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  @media (max-width: 639px) {
    .head {
      align-items: flex-start;
    }
    .scan {
      justify-items: start;
    }
    h1 {
      font-size: 23px;
    }
    .big {
      font-size: 26px;
    }
  }
</style>
