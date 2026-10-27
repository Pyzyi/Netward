<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import DeviceIcon from './DeviceIcon.svelte';
  import Heartbeat from './Heartbeat.svelte';
  import { displayName, subtitle } from '../devices';
  import { link } from '../router.svelte';
  import { clock, store } from '../state.svelte';
  import { relTime, t } from '../i18n.svelte';
  import type { Device } from '../types';

  // One line of the device register: glyph, name, address, the 24-hour
  // heartbeat and the current state. `wide` adds the heartbeat column.
  let {
    device,
    href,
    wide = true,
    compact = false,
    selected = false,
    actions,
  }: { device: Device; href: string; wide?: boolean; compact?: boolean; selected?: boolean; actions?: Snippet } = $props();
  const d = $derived(device);
  const name = $derived(displayName(d));
  const sub = $derived(subtitle(d));
</script>

<div class="row-item" class:compact class:selected class:narrow={!wide}>
  <a class="main" {href} use:link aria-current={selected ? 'true' : undefined}>
    <DeviceIcon type={d.type} online={d.online} fresh={d.new} size={compact ? 32 : 36} />
    <span class="names">
      <span class="name">
        <span class="text">{name}</span>
        {#if d.favorite}<Icon name="fill:star" size={13} class="fav" label={t('device.actions.favorite')} />{/if}
        {#if d.watch}<Icon name="bell" size={13} class="watch" label={t('device.actions.watch')} />{/if}
        {#if d.new}<span class="badge warn">{t('devices.newBadge')}</span>{/if}
        {#if d.software?.status === 'outdated'}<span class="badge danger" title={d.software.name}>{t('devices.outdatedBadge')}</span>{/if}
        {#if d.software?.status === 'update'}<span class="badge warn" title={d.software.name}>{t('devices.updateBadge')}</span>{/if}
        {#if d.isSelf}<span class="badge accent">{t('devices.thisServer')}</span>{/if}
        {#if d.isGateway}<span class="badge accent">{t('devices.router')}</span>{/if}
      </span>
      {#if sub}<span class="sub">{sub}</span>{/if}
    </span>
    <span class="ip mono">{d.ip}</span>
    <span class="beat"><Heartbeat strip={store.strips[d.id] ?? ''} /></span>
    <span class="seen" class:on={d.online}>
      {#if d.online}
        <!-- The green mark on the tile and the heartbeat already say it. -->
        <span class="on-text">{t('devices.onlineNow')}</span>
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
    border-bottom: 1px solid var(--border);
  }
  .row-item:last-child {
    border-bottom: 0;
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
    grid-template-columns: auto minmax(0, 1fr) 140px 192px 104px;
    align-items: center;
    gap: 16px;
    padding: 8px 16px;
    color: inherit;
    text-decoration: none;
  }
  .main:hover {
    text-decoration: none;
  }
  .narrow .main {
    grid-template-columns: auto minmax(0, 1fr) 120px 104px;
  }
  .narrow .beat {
    display: none;
  }
  .compact .main {
    grid-template-columns: auto minmax(0, 1fr) auto;
    padding: 8px 4px;
  }
  .compact .seen,
  .compact .beat,
  .compact .ip {
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
    font-weight: 500;
  }
  .text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .name :global(.fav) {
    color: var(--amber);
  }
  .name :global(.watch) {
    color: var(--text-2);
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
    font-weight: 500;
  }
  /* With the heartbeat visible, the word "Online" is redundant; without it, it is not. */
  .on-text {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
  }
  .seen .state,
  .seen .ago {
    display: block;
  }
  .seen .state {
    font-weight: 500;
    color: var(--text);
  }
  .seen .ago {
    font-size: 12px;
    color: var(--text-2);
  }
  .actions {
    display: flex;
    gap: 6px;
    padding-right: 8px;
  }
  @media (max-width: 900px) {
    .main {
      grid-template-columns: auto minmax(0, 1fr) 120px 104px;
    }
    .beat {
      display: none;
    }
    .on-text {
      position: static;
      width: auto;
      height: auto;
      clip: auto;
    }
  }
  @media (max-width: 639px) {
    /* IP and heartbeat are hidden here: glyph, name, state. */
    .main,
    .narrow .main,
    .compact .main {
      grid-template-columns: auto minmax(0, 1fr) auto;
      gap: 12px;
      padding: 9px 14px;
    }
    .ip {
      display: none;
    }
  }
</style>
