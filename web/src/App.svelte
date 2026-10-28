<script lang="ts">
  import AppShell from './lib/components/AppShell.svelte';
  import Toaster from './lib/components/Toaster.svelte';
  import Logo from './lib/components/Logo.svelte';
  import Setup from './pages/Setup.svelte';
  import Login from './pages/Login.svelte';
  import Overview from './pages/Overview.svelte';
  import Devices from './pages/Devices.svelte';
  import Activity from './pages/Activity.svelte';
  import Security from './pages/Security.svelte';
  import Wlan from './pages/Wlan.svelte';
  import WifiMap from './pages/WifiMap.svelte';
  import Persons from './pages/Persons.svelte';
  import SettingsPage from './pages/Settings.svelte';
  import { auth, connect, loadAll, loadAuth } from './lib/state.svelte';
  import { router } from './lib/router.svelte';
  import { i18n, t } from './lib/i18n.svelte';

  $effect(() => {
    document.documentElement.lang = i18n.lang;
  });

  void loadAuth();

  let started = false;
  $effect(() => {
    if (auth.state?.authenticated && !started) {
      started = true;
      void loadAll().then(connect);
    }
    if (!auth.state?.authenticated) started = false;
  });

  const titles: Record<string, string> = {
    overview: 'nav.overview', devices: 'nav.devices', activity: 'nav.activity', security: 'nav.security', settings: 'nav.settings', wlan: 'wlan.title', wifimap: 'wifimap.title', persons: 'persons.page',
  };
  $effect(() => {
    document.title = auth.state?.authenticated ? `${t(titles[router.route.name])} · Netzwart` : 'Netzwart';
  });
</script>

{#if !auth.loaded}
  <div class="boot" aria-busy="true"><Logo size={40} /></div>
{:else if !auth.state}
  <div class="boot"><p class="muted">{t('err.network')}</p><button class="btn" onclick={() => location.reload()}>{t('common.retry')}</button></div>
{:else if auth.state.setupRequired}
  <Setup />
{:else if !auth.state.authenticated}
  <Login />
{:else}
  <AppShell>
    {#if router.route.name === 'overview'}
      <Overview />
    {:else if router.route.name === 'devices'}
      <Devices />
    {:else if router.route.name === 'activity'}
      <Activity />
    {:else if router.route.name === 'security'}
      <Security />
    {:else if router.route.name === 'wlan'}
      <Wlan />
    {:else if router.route.name === 'wifimap'}
      <WifiMap />
    {:else if router.route.name === 'persons'}
      <Persons />
    {:else if router.route.name === 'settings'}
      <SettingsPage />
    {/if}
  </AppShell>
{/if}
<Toaster />

<style>
  .boot {
    min-height: 100dvh;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: 14px;
  }
</style>
