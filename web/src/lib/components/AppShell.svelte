<script lang="ts">
  import type { Snippet } from 'svelte';
  import LayoutGrid from '@lucide/svelte/icons/layout-grid';
  import MonitorSmartphone from '@lucide/svelte/icons/monitor-smartphone';
  import Activity from '@lucide/svelte/icons/activity';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import Settings from '@lucide/svelte/icons/settings';
  import LogOut from '@lucide/svelte/icons/log-out';
  import WifiOff from '@lucide/svelte/icons/wifi-off';
  import Logo from './Logo.svelte';
  import { link, router } from '../router.svelte';
  import { auth, clock, logout, store } from '../state.svelte';
  import { relTime, t } from '../i18n.svelte';

  let { children }: { children: Snippet } = $props();

  const nav = [
    { name: 'overview', href: '/', label: 'nav.overview', icon: LayoutGrid },
    { name: 'devices', href: '/devices', label: 'nav.devices', icon: MonitorSmartphone },
    { name: 'activity', href: '/activity', label: 'nav.activity', icon: Activity },
    { name: 'security', href: '/security', label: 'nav.security', icon: ShieldCheck },
    { name: 'settings', href: '/settings', label: 'nav.settings', icon: Settings },
  ] as const;

  const newCount = $derived([...store.devices.values()].filter((d) => d.new).length);
  const secCount = $derived((store.overview?.security.critical ?? 0) + (store.overview?.security.warning ?? 0));
  const scanner = $derived(store.overview?.scanner);

  function badgeFor(name: string): number {
    if (name === 'devices') return newCount;
    if (name === 'security') return secCount;
    return 0;
  }
</script>

<a class="skip" href="#main">{t('nav.skip')}</a>
<div class="shell">
  <aside class="sidebar" aria-label="Navigation">
    <a class="brand" href="/" use:link>
      <Logo size={28} />
      <span>Netzwart</span>
    </a>
    <nav>
      {#each nav as item (item.name)}
        {@const Icon = item.icon}
        {@const badge = badgeFor(item.name)}
        <a href={item.href} use:link class="nav-item" aria-current={router.route.name === item.name ? 'page' : undefined} title={t(item.label)}>
          <Icon size={19} strokeWidth={1.8} />
          <span class="label">{t(item.label)}</span>
          {#if badge > 0}<span class="nav-badge" class:danger={item.name === 'security'}>{badge}</span>{/if}
        </a>
      {/each}
    </nav>
    <div class="side-foot">
      {#if scanner}
        <div class="scan-status" title={scanner.interfaces?.filter((i) => i.active).map((i) => `${i.name} ${i.prefix}`).join(', ')}>
          <span class="pulse" class:active={scanner.scanning}></span>
          <span class="label">
            {scanner.scanning ? t('overview.scanning') : t('overview.lastScan', { t: relTime(scanner.lastScanAt, clock.now) })}
          </span>
        </div>
      {/if}
      <div class="user">
        <span class="label user-name">{auth.state?.username}</span>
        <button class="btn ghost icon small" type="button" onclick={logout} title={t('nav.logout')} aria-label={t('nav.logout')}>
          <LogOut size={17} />
        </button>
      </div>
    </div>
  </aside>

  <main id="main">
    {#if !store.connected}
      <div class="conn notice warn" role="status"><WifiOff size={17} /> {t('common.offlineBanner')}</div>
    {/if}
    {@render children()}
  </main>

  <nav class="tabbar" aria-label="Navigation">
    {#each nav as item (item.name)}
      {@const Icon = item.icon}
      {@const badge = badgeFor(item.name)}
      <a href={item.href} use:link aria-current={router.route.name === item.name ? 'page' : undefined}>
        <span class="tab-icon">
          <Icon size={22} strokeWidth={1.8} />
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
    border-radius: 6px;
  }
  .skip:focus {
    left: 8px;
  }
  .shell {
    display: grid;
    grid-template-columns: var(--sidebar) 1fr;
    min-height: 100dvh;
  }
  .sidebar {
    position: sticky;
    top: 0;
    height: 100dvh;
    display: flex;
    flex-direction: column;
    gap: 18px;
    padding: 18px 12px 14px;
    border-right: 1px solid var(--border);
    background: var(--surface);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 2px 8px;
    color: var(--text);
    font-weight: 650;
    font-size: 16.5px;
    letter-spacing: -0.01em;
    text-decoration: none;
  }
  nav {
    display: grid;
    gap: 2px;
  }
  .nav-item {
    display: flex;
    align-items: center;
    gap: 11px;
    height: 38px;
    padding: 0 10px;
    border-radius: 8px;
    color: var(--text-2);
    font-weight: 520;
    text-decoration: none;
  }
  .nav-item:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .nav-item[aria-current='page'] {
    background: var(--accent-soft);
    color: var(--accent);
  }
  .nav-badge,
  .tab-badge {
    margin-left: auto;
    min-width: 20px;
    height: 20px;
    padding: 0 6px;
    border-radius: 999px;
    background: var(--warn-soft);
    color: var(--warn);
    font-size: 11.5px;
    font-weight: 650;
    display: inline-grid;
    place-items: center;
    font-variant-numeric: tabular-nums;
  }
  .nav-badge.danger,
  .tab-badge.danger {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .side-foot {
    margin-top: auto;
    display: grid;
    gap: 10px;
    padding: 0 6px;
  }
  .scan-status {
    display: flex;
    align-items: center;
    gap: 9px;
    font-size: 12.5px;
    color: var(--text-2);
  }
  .pulse {
    width: 8px;
    height: 8px;
    flex: none;
    border-radius: 50%;
    background: var(--ok);
  }
  .pulse.active {
    background: var(--accent);
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent) 60%, transparent);
    animation: pulse 1.4s infinite;
  }
  @keyframes pulse {
    70% {
      box-shadow: 0 0 0 7px transparent;
    }
  }
  .user {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-top: 10px;
    border-top: 1px solid var(--border);
  }
  .user-name {
    font-weight: 550;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  main {
    min-width: 0;
    padding: 28px 32px 48px;
  }
  .conn {
    margin-bottom: 16px;
  }
  .tabbar {
    display: none;
  }

  @media (max-width: 1023px) {
    .shell {
      grid-template-columns: 68px 1fr;
    }
    .sidebar {
      align-items: center;
      padding: 16px 8px;
    }
    .sidebar .label,
    .brand span,
    .nav-badge {
      display: none;
    }
    .nav-item {
      justify-content: center;
      width: 44px;
      height: 44px;
      padding: 0;
    }
    .user {
      justify-content: center;
    }
    main {
      padding: 24px 24px 40px;
    }
  }

  @media (max-width: 639px) {
    .shell {
      grid-template-columns: 1fr;
    }
    .sidebar {
      display: none;
    }
    main {
      padding: 18px 16px calc(84px + env(safe-area-inset-bottom));
    }
    .tabbar {
      position: fixed;
      left: 0;
      right: 0;
      bottom: 0;
      z-index: 40;
      display: grid;
      grid-template-columns: repeat(5, 1fr);
      padding: 6px 4px calc(6px + env(safe-area-inset-bottom));
      background: color-mix(in srgb, var(--surface) 94%, transparent);
      backdrop-filter: blur(12px);
      border-top: 1px solid var(--border);
    }
    .tabbar a {
      display: grid;
      justify-items: center;
      gap: 3px;
      padding: 4px 0;
      color: var(--text-3);
      font-size: 10.5px;
      font-weight: 550;
      text-decoration: none;
    }
    .tabbar a[aria-current='page'] {
      color: var(--accent);
    }
    .tab-icon {
      position: relative;
      display: grid;
      place-items: center;
    }
    .tab-badge {
      position: absolute;
      top: -6px;
      right: -12px;
      min-width: 17px;
      height: 17px;
      padding: 0 4px;
      font-size: 10.5px;
      border: 2px solid var(--surface);
    }
  }
</style>
