<script lang="ts">
  import Star from '@lucide/svelte/icons/star';
  import Bell from '@lucide/svelte/icons/bell';
  import DeviceIcon from './DeviceIcon.svelte';
  import { displayName, subtitle } from '../devices';
  import { link } from '../router.svelte';
  import { clock } from '../state.svelte';
  import { relTime, t } from '../i18n.svelte';
  import type { Device } from '../types';
  import type { Snippet } from 'svelte';

  let { device, href, showMac = true, compact = false, selected = false, actions }: {
    device: Device; href: string; showMac?: boolean; compact?: boolean; selected?: boolean; actions?: Snippet;
  } = $props();
  const d = $derived(device);
  const name = $derived(displayName(d));
  const sub = $derived(subtitle(d));
</script>

<div class="row-item" class:compact class:selected class:no-mac={!showMac}>
  <a class="main" {href} use:link aria-current={selected ? 'true' : undefined}>
    <DeviceIcon type={d.type} online={d.online} size={compact ? 34 : 38} />
    <span class="names">
      <span class="name">
        <span class="text">{name}</span>
        {#if d.new}<span class="badge warn">{t('devices.newBadge')}</span>{/if}
        {#if d.isSelf}<span class="badge accent">{t('devices.thisServer')}</span>{/if}
        {#if d.isGateway}<span class="badge">{t('devices.router')}</span>{/if}
        {#if d.favorite}<Star size={13} class="fav" fill="currentColor" aria-label={t('device.actions.favorite')} />{/if}
        {#if d.watch}<Bell size={13} class="watch" aria-label={t('device.actions.watch')} />{/if}
      </span>
      {#if sub}<span class="sub">{sub}</span>{/if}
    </span>
    <span class="ip mono">{d.ip}</span>
    {#if showMac}<span class="mac mono faint">{d.mac}</span>{/if}
    <span class="seen" class:on={d.online}>
      {#if d.online}
        {t('devices.onlineNow')}
      {:else}
        <span class="state">{t('common.offline')}</span>
        <span class="ago">{relTime(d.lastSeen, clock.now)}</span>
      {/if}
    </span>
  </a>
  {#if actions}<div class="actions">{@render actions()}</div>{/if}
</div>

<style>
  .row-item {
    display: flex;
    align-items: center;
    gap: 8px;
    border-radius: 8px;
  }
  .row-item:hover {
    background: var(--surface-2);
  }
  .row-item.selected {
    background: var(--accent-soft);
  }
  .main {
    flex: 1;
    min-width: 0;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) 128px 150px 110px;
    align-items: center;
    gap: 14px;
    padding: 9px 10px;
    color: inherit;
    text-decoration: none;
  }
  .no-mac .main {
    grid-template-columns: auto minmax(0, 1fr) 128px 110px;
  }
  .compact .main {
    grid-template-columns: auto minmax(0, 1fr) auto;
    padding: 8px 6px;
  }
  .compact .seen,
  .compact .mac {
    display: none;
  }
  .names {
    display: grid;
    min-width: 0;
  }
  .name {
    display: flex;
    align-items: center;
    gap: 7px;
    min-width: 0;
    font-weight: 560;
  }
  .text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .name :global(.fav) {
    flex: none;
    color: #d39a00;
  }
  .name :global(.watch) {
    flex: none;
    color: var(--accent);
  }
  .sub {
    color: var(--text-2);
    font-size: 13px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ip {
    color: var(--text);
  }
  .seen {
    font-size: 13px;
    color: var(--text-2);
    text-align: right;
    white-space: nowrap;
  }
  .seen.on {
    color: var(--ok);
    font-weight: 550;
  }
  .seen .state,
  .seen .ago {
    display: block;
  }
  .seen .state {
    font-weight: 550;
    color: var(--text-2);
  }
  .seen .ago {
    font-size: 12px;
  }
  .actions {
    display: flex;
    gap: 6px;
    padding-right: 8px;
  }
  @media (max-width: 1279px) {
    .main {
      grid-template-columns: auto minmax(0, 1fr) 120px 100px;
    }
    .mac {
      display: none;
    }
  }
  @media (max-width: 639px) {
    /* IP and MAC are hidden here: icon, name, last seen. */
    .main,
    .no-mac .main,
    .compact .main {
      grid-template-columns: auto minmax(0, 1fr) auto;
      gap: 12px;
      padding: 9px 4px;
    }
    .ip {
      display: none;
    }
  }
</style>
