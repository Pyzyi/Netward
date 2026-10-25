<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import Logo from './Logo.svelte';
  import { link, router } from '../router.svelte';
  import { auth, clock, logout, store } from '../state.svelte';
  import { relTime, t } from '../i18n.svelte';

  let { children }: { children: Snippet } = $props();

  const nav = [
    { name: 'overview', href: '/', label: 'nav.overview', icon: 'squares-four' },
    { name: 'devices', href: '/devices', label: 'nav.devices', icon: 'devices' },
    { name: 'activity', href: '/activity', label: 'nav.activity', icon: 'pulse' },
    { name: 'security', href: '/security', label: 'nav.security', icon: 'shield-check' },
    { name: 'settings', href: '/settings', label: 'nav.settings', icon: 'gear-six' },
  ] as const;

  const newCount = $derived([...store.devices.values()].filter((d) => d.new).length);
  const secCount = $derived((store.overview?.security.critical ?? 0) + (store.overview?.security.warning ?? 0));
  const scanner = $derived(store.overview?.scanner);
  const iface = $derived(scanner?.interfaces?.find((i) => i.active));

  function badgeFor(name: string): number {
    if (name === 'devices') return newCount;
    if (name === 'security') return secCount;
    return 0;
  }
</script>

<a class="skip" href="#main">{t('nav.skip')}</a>
<div class="shell">
  <header class="band">
    <div class="band-inner">
      <a class="brand" href="/" use:link aria-label="Netzwart">
        <Logo size={26} tile={false} />
        <span class="brand-name">Netzwart</span>
      </a>
      {#if store.overview && router.route.name !== 'overview'}
        <span class="net" title={iface ? `${iface.name} · ${iface.prefix}` : undefined}>
          <span class="net-name">{store.overview.networkName}</span>
          {#if iface}<span class="net-prefix">{iface.prefix}</span>{/if}
        </span>
      {/if}
      <nav class="tabs" aria-label="Navigation">
        {#each nav as item (item.name)}
          {@const badge = badgeFor(item.name)}
          <a href={item.href} use:link aria-current={router.route.name === item.name ? 'page' : undefined}>
            {t(item.label)}
            {#if badge > 0}<span class="count" class:danger={item.name === 'security'}>{badge}</span>{/if}
          </a>
        {/each}
      </nav>
      <div class="right">
        {#if scanner}
          <span class="live" class:scanning={scanner.scanning} role="status">
            <span class="pulse" aria-hidden="true"></span>
            <span class="live-text">
              {scanner.scanning ? t('overview.scanning') : t('overview.lastScan', { t: relTime(scanner.lastScanAt, clock.now) })}
            </span>
          </span>
        {/if}
        <span class="user">
          <span class="user-name">{auth.state?.username}</span>
          <button class="out" type="button" onclick={logout} title={t('nav.logout')} aria-label={t('nav.logout')}>
            <Icon name="sign-out" size={17} />
          </button>
        </span>
      </div>
    </div>
  </header>

  <main id="main">
    {#if !store.connected}
      <div class="conn" role="status"><Icon name="wifi-slash" size={16} /> {t('common.offlineBanner')}</div>
    {/if}
    {@render children()}
  </main>

  <nav class="tabbar" aria-label="Navigation">
    {#each nav as item (item.name)}
      {@const badge = badgeFor(item.name)}
      <a href={item.href} use:link aria-current={router.route.name === item.name ? 'page' : undefined}>
        <span class="tab-icon">
          <Icon name={item.icon} size={22} />
          {#if badge > 0}<span class="tab-badge" class:danger={item.name === 'security'}>{badge}</span>{/if}
        </span>
        <span>{t(item.label)}</span>
      </a>
    {/each}
  </nav>
</div>

<style>
  .skip {
    position: absolute;
    left: -999px;
    top: 8px;
    z-index: 200;
    padding: 8px 12px;
    background: var(--surface);
    color: var(--text);
  }
  .skip:focus {
    left: 8px;
  }
  .shell {
    min-height: 100dvh;
    display: flex;
    flex-direction: column;
  }

  /* Ink band */
  .band {
    position: sticky;
    top: 0;
    z-index: 40;
    background: var(--ink);
    color: var(--on-ink);
    box-shadow: 0 1px 0 var(--ink-line);
  }
  .band :focus-visible {
    outline-color: var(--on-ink);
  }
  .band-inner {
    max-width: var(--page);
    margin: 0 auto;
    height: 56px;
    padding: 0 28px;
    display: flex;
    align-items: center;
    gap: 22px;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 9px;
    color: var(--on-ink);
    text-decoration: none;
  }
  .brand:hover {
    text-decoration: none;
  }
  .brand-name {
    font-weight: 760;
    font-size: 16px;
    letter-spacing: -0.02em;
  }
  .net {
    display: flex;
    align-items: baseline;
    gap: 8px;
    padding-left: 18px;
    border-left: 1px solid var(--ink-line);
    min-width: 0;
  }
  .net-name {
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 160px;
  }
  .net-prefix {
    font-family: var(--mono);
    font-size: 12px;
    color: var(--on-ink-2);
  }
  .tabs {
    display: flex;
    align-self: stretch;
    gap: 2px;
    margin-left: 8px;
  }
  .tabs a {
    position: relative;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 12px;
    color: var(--on-ink-2);
    font-weight: 600;
    font-size: 14px;
    text-decoration: none;
  }
  .tabs a:hover {
    color: var(--on-ink);
  }
  .tabs a[aria-current='page'] {
    color: var(--on-ink);
  }
  .tabs a[aria-current='page']::after {
    content: '';
    position: absolute;
    left: 12px;
    right: 12px;
    bottom: 0;
    height: 3px;
    background: var(--on-ink);
  }
  .count {
    font-family: var(--mono);
    font-size: 11px;
    font-weight: 600;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    display: inline-grid;
    place-items: center;
    border-radius: 3px;
    background: var(--amber);
    color: #1a1204;
  }
  .count.danger {
    background: #ff7b6e;
    color: #2a0904;
  }
  .right {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 18px;
  }
  .live {
    display: flex;
    align-items: center;
    gap: 8px;
    font-family: var(--mono);
    font-size: 12px;
    color: var(--on-ink-2);
    white-space: nowrap;
  }
  .pulse {
    position: relative;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--signal);
  }
  .live.scanning .pulse::after {
    content: '';
    position: absolute;
    inset: -4px;
    border-radius: 50%;
    border: 1.5px solid var(--signal);
    animation: ping 1.4s var(--ease) infinite;
  }
  @keyframes ping {
    from {
      transform: scale(0.6);
      opacity: 1;
    }
    to {
      transform: scale(1.6);
      opacity: 0;
    }
  }
  .user {
    display: flex;
    align-items: center;
    gap: 6px;
    padding-left: 18px;
    border-left: 1px solid var(--ink-line);
  }
  .user-name {
    font-size: 13px;
    color: var(--on-ink-2);
  }
  .out {
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--on-ink);
    cursor: pointer;
  }
  .out:hover {
    background: var(--ink-2);
  }

  main {
    flex: 1;
    min-width: 0;
  }
  .conn {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 8px 16px;
    background: var(--warn-soft);
    color: var(--warn);
    font-size: 13px;
    font-weight: 600;
  }

  /* Phone tab bar */
  .tabbar {
    display: none;
  }

  @media (max-width: 1080px) {
    .net {
      display: none;
    }
    .user-name {
      display: none;
    }
  }
  @media (max-width: 860px) {
    .band-inner {
      padding: 0 16px;
      gap: 12px;
    }
    .tabs,
    .user {
      display: none;
    }
    .net {
      display: flex;
    }
    .tabbar {
      position: fixed;
      left: 0;
      right: 0;
      bottom: 0;
      z-index: 40;
      display: grid;
      grid-template-columns: repeat(5, 1fr);
      padding: 0 4px env(safe-area-inset-bottom);
      background: var(--bg);
      border-top: 1px solid var(--text);
    }
    .tabbar a {
      position: relative;
      display: grid;
      justify-items: center;
      gap: 2px;
      padding: 8px 0 7px;
      color: var(--text-2);
      font-size: 11px;
      font-weight: 600;
      text-decoration: none;
    }
    .tabbar a[aria-current='page'] {
      color: var(--text);
    }
    .tabbar a[aria-current='page']::before {
      content: '';
      position: absolute;
      top: -1px;
      left: 22%;
      right: 22%;
      height: 3px;
      background: var(--text);
    }
    .tab-icon {
      position: relative;
    }
    .tab-badge {
      position: absolute;
      top: -4px;
      right: -12px;
      min-width: 17px;
      height: 16px;
      padding: 0 4px;
      border-radius: 3px;
      background: var(--amber);
      color: #1a1204;
      font-family: var(--mono);
      font-size: 10px;
      font-weight: 700;
      display: grid;
      place-items: center;
    }
    .tab-badge.danger {
      background: var(--danger);
      color: #fff;
    }
    main {
      padding-bottom: calc(64px + env(safe-area-inset-bottom));
    }
  }
  @media (max-width: 520px) {
    .net-prefix,
    .live-text {
      display: none;
    }
  }
</style>
