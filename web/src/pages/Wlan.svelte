<script lang="ts">
  import Icon from '../lib/components/Icon.svelte';
  import DeviceIcon from '../lib/components/DeviceIcon.svelte';
  import EmptyState from '../lib/components/EmptyState.svelte';
  import SignalBars from '../lib/components/SignalBars.svelte';
  import { api } from '../lib/api';
  import { clock, store } from '../lib/state.svelte';
  import { link } from '../lib/router.svelte';
  import { relTime, t } from '../lib/i18n.svelte';
  import { apName, bandLabel, clientName, hintText, hintTodo, rateText, signalLevel, signalText } from '../lib/wlan';
  import type { WlanAccessPoint, WlanClient, WlanHint, WlanView } from '../lib/types';

  let view = $state<WlanView | null>(null);
  let failed = $state(false);

  async function load() {
    try {
      view = await api.wlan();
      failed = false;
    } catch {
      failed = true;
    }
  }
  $effect(() => {
    void load();
    const timer = setInterval(load, 60_000); // the router is asked every 5 minutes
    return () => clearInterval(timer);
  });

  type Item = { h: WlanHint; c: WlanClient | null; ap: WlanAccessPoint };
  // All hints, repeater problems first: they explain the hints of the devices behind them.
  const hints = $derived.by(() => {
    const out: Item[] = [];
    for (const ap of view?.aps ?? []) {
      for (const h of ap.hints) out.push({ h, c: null, ap });
    }
    for (const ap of view?.aps ?? []) {
      for (const c of ap.clients) for (const h of c.hints) if (h !== 'weak_repeater') out.push({ h, c, ap });
    }
    return out;
  });
  const sortedClients = (ap: WlanAccessPoint) =>
    [...ap.clients].sort((a, b) => (a.signalDbm || -200) - (b.signalDbm || -200) || (a.signalPct ?? 0) - (b.signalPct ?? 0));
</script>

<div class="wrap">
  <header class="page-head">
    <div>
      <a class="crumb" href="/devices" use:link><Icon name="caret-left" size={14} /> {t('nav.devices')}</a>
      <h1>{t('wlan.title')}</h1>
      <p class="lead">{t('wlan.lead')}</p>
    </div>
    <div class="head-side">
      <a class="btn" href="/wlan/map" use:link><Icon name="map-trifold" size={16} /> {t('wlan.mapLink')}</a>
      {#if view?.at}<span class="faint small">{t('wlan.asOf', { t: relTime(view.at, clock.now) })}</span>{/if}
    </div>
  </header>

  {#if !view}
    {#if failed}<p class="notice danger">{t('common.error')}</p>{:else}<div class="skeleton" style:height="160px"></div>{/if}
  {:else if view.fritzbox === 'disabled'}
    <EmptyState text={t('wlan.none.text')}>
      <a class="btn primary" href="/settings/fritzbox" use:link>{t('wlan.none.link')}</a>
    </EmptyState>
  {:else}
    {#if view.status === 'auth'}<p class="notice warn">{t('wlan.auth')}</p>
    {:else if view.status === 'error' || view.fritzbox === 'error'}<p class="notice warn">{t('wlan.error')}</p>{/if}
    {#if view.source === 'wlanlist'}<p class="notice info">{t('wlan.fallback')}</p>{/if}

    {#if !view.at}
      {#if view.status !== 'auth' && view.status !== 'error' && view.fritzbox !== 'error'}<EmptyState text={t('wlan.pending')} />{/if}
    {:else}
      <section class="hints">
        <h2>{t('wlan.hints')}</h2>
        {#if hints.length === 0}
          <p class="all-good"><Icon name="check-circle" size={20} /> {t('wlan.noHints')}</p>
        {:else}
          <ul>
            {#each hints as it, i (i)}
              {@const strong = it.h === 'very_weak' || it.h === 'backhaul_weak'}
              <li>
                <span class="dot {strong ? 'danger' : 'warn'}" aria-hidden="true"></span>
                <div>
                  <p>{hintText(it.h, it.c, it.ap, store.devices)}</p>
                  {#if hints.findIndex((x) => hintTodo(x.h) === hintTodo(it.h)) === i}<p class="todo">{hintTodo(it.h)}</p>{/if}
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      {#each view.aps as ap (ap.mac)}
        <section class="ap">
          <header class="ap-head">
            <div>
              <h2>
                {#if ap.deviceId && store.devices.get(ap.deviceId)}
                  <a href="/devices/{ap.deviceId}" use:link>{apName(ap, store.devices)}</a>
                {:else}{apName(ap, store.devices)}{/if}
              </h2>
              <p class="meta">
                <span class="badge accent">{ap.master ? t('wlan.master') : t('wlan.repeater')}</span>
                {#if ap.model}<span class="faint sep">{ap.model}</span>{/if}
                {#if ap.bands?.length}<span class="faint sep">{ap.bands.map(bandLabel).join(' / ')}</span>{/if}
              </p>
              {#if ap.backhaul}
                <p class="meta" class:warn={ap.hints.includes('backhaul_weak')}>
                  <span class="sep">{ap.backhaul.type === 'lan' ? t('wlan.backhaul.lan') : t('wlan.backhaul.wlan', { band: bandLabel(ap.backhaul.band) })}</span>
                  {#if ap.backhaul.signalDbm}<span class="sep num">{t('wlan.dbm', { v: ap.backhaul.signalDbm })}</span>{/if}
                  {#if ap.backhaul.rate}<span class="sep num">{t('wlan.rate', { v: ap.backhaul.rate })}</span>{/if}
                </p>
              {/if}
            </div>
            <span class="count faint">{ap.clients.length === 1 ? t('wlan.devices.one') : t('wlan.devices', { n: ap.clients.length })}</span>
          </header>
          {#if ap.clients.length === 0}
            <p class="faint small">{t('wlan.noDevices')}</p>
          {:else}
            <table>
              <thead>
                <tr><th>{t('wlan.col.device')}</th><th>{t('wlan.col.band')}</th><th>{t('wlan.col.signal')}</th><th>{t('wlan.col.rate')}</th></tr>
              </thead>
              <tbody>
                {#each sortedClients(ap) as c (c.mac)}
                  {@const dev = c.deviceId ? store.devices.get(c.deviceId) : undefined}
                  {@const lvl = signalLevel(c)}
                  <tr class:flag={c.hints.length > 0}>
                    <td class="name">
                      {#if dev}
                        <a href="/devices/{dev.id}" use:link><DeviceIcon type={dev.type} size={22} /> {clientName(c, store.devices)}</a>
                      {:else}<span class="muted">{clientName(c, store.devices)}</span>{/if}
                    </td>
                    <td class="num" data-label={t('wlan.col.band')}>{bandLabel(c.band)}</td>
                    <td class="signal {lvl}" data-label={t('wlan.col.signal')}>
                      <SignalBars level={lvl} />
                      <span class="num val">{signalText(c)}</span>
                      <span class="sr-only">{t(`wlan.signal.${lvl}`)}</span>
                    </td>
                    <td class="num rate" data-label={t('wlan.col.rate')}>{rateText(c)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </section>
      {/each}
    {/if}
  {/if}
</div>

<style>
  .head-side {
    display: grid;
    justify-items: end;
    gap: 6px;
  }
  .small {
    font-size: 13px;
  }
  .hints {
    display: grid;
    gap: 10px;
    margin-bottom: 16px;
  }
  .hints ul {
    list-style: none;
    margin: 0;
    padding: 0 16px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .hints li {
    display: grid;
    grid-template-columns: 8px minmax(0, 1fr);
    gap: 12px;
    padding: 11px 0;
  }
  .hints li + li {
    border-top: 1px solid var(--border);
  }
  .hints li .dot {
    margin-top: 7px;
  }
  .hints li p:first-child {
    font-weight: 500;
  }
  .todo {
    color: var(--text-2);
    font-size: 14px;
  }
  .all-good {
    display: flex;
    align-items: center;
    gap: 10px;
    color: var(--ok);
    font-weight: 500;
  }
  .ap {
    margin-top: 16px;
    padding: 16px 16px 6px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .ap-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 16px;
    margin-bottom: 10px;
  }
  .ap-head h2 {
    font-size: 16px;
  }
  .ap-head h2 a {
    color: var(--text);
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    margin-top: 4px;
    font-size: 14px;
  }
  .meta .sep + .sep::before {
    content: '·';
    margin-right: 6px;
    color: var(--text-2);
  }
  .meta.warn {
    color: var(--warn);
    font-weight: 500;
  }
  .count {
    font-size: 13px;
    white-space: nowrap;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    table-layout: fixed;
    font-size: 14px;
  }
  th:nth-child(1) {
    width: 42%;
  }
  th:nth-child(2) {
    width: 13%;
  }
  th:nth-child(3) {
    width: 18%;
  }
  th {
    text-align: left;
    font-size: 12px;
    font-weight: 500;
    color: var(--text-2);
    padding: 6px 10px 6px 0;
    border-bottom: 1px solid var(--border);
  }
  td {
    padding: 8px 10px 8px 0;
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
  }
  tbody tr:last-child td {
    border-bottom: 0;
  }
  td.name a {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    color: var(--text);
    font-weight: 500;
  }
  td.signal {
    white-space: nowrap;
  }
  td.signal :global(.bars) {
    margin-right: 6px;
    vertical-align: -2px;
  }
  td.rate {
    white-space: nowrap;
  }
  td.weak .val {
    color: var(--warn);
  }
  td.very_weak .val {
    color: var(--danger);
  }
  @media (max-width: 640px) {
    thead {
      display: none;
    }
    table,
    tbody,
    tr {
      display: block;
      table-layout: auto;
    }
    tr {
      display: grid;
      grid-template-columns: repeat(3, auto);
      justify-content: start;
      gap: 4px 16px;
      padding: 10px 0;
      border-bottom: 1px solid var(--border);
    }
    tbody tr:last-child {
      border-bottom: 0;
    }
    td {
      padding: 0;
      border: 0;
    }
    td.name {
      grid-column: 1 / -1;
    }
    /* Without the header row each value names its column. */
    td[data-label]::before {
      content: attr(data-label) ' ';
      display: block;
      font-family: var(--font);
      font-size: 11px;
      color: var(--text-2);
    }
  }
</style>
