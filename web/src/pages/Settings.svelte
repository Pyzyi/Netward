<script lang="ts">
  import General from './settings/General.svelte';
  import Scanning from './settings/Scanning.svelte';
  import Notifications from './settings/Notifications.svelte';
  import HomeAssistant from './settings/HomeAssistant.svelte';
  import Internet from './settings/Internet.svelte';
  import FritzBox from './settings/FritzBox.svelte';
  import Account from './settings/Account.svelte';
  import Data from './settings/Data.svelte';
  import About from './settings/About.svelte';
  import { link, router } from '../lib/router.svelte';
  import LogOut from '@lucide/svelte/icons/log-out';
  import { auth, logout, store } from '../lib/state.svelte';
  import { t } from '../lib/i18n.svelte';

  const sections = [
    { id: 'general', label: 'settings.general', comp: General },
    { id: 'scanning', label: 'settings.scanning', comp: Scanning },
    { id: 'notifications', label: 'settings.notifications', comp: Notifications },
    { id: 'homeassistant', label: 'settings.homeassistant', comp: HomeAssistant },
    { id: 'internet', label: 'settings.internet', comp: Internet },
    { id: 'fritzbox', label: 'settings.fritzbox', comp: FritzBox },
    { id: 'account', label: 'settings.account', comp: Account },
    { id: 'data', label: 'settings.data', comp: Data },
    { id: 'about', label: 'settings.about', comp: About },
  ] as const;

  const current = $derived(
    sections.find((s) => router.route.name === 'settings' && s.id === router.route.section) ?? sections[0],
  );
  const Comp = $derived(current.comp);

  // On narrow screens the section tabs scroll sideways; keep the active one visible.
  let subnav: HTMLElement | undefined = $state();
  $effect(() => {
    void current.id;
    subnav?.querySelector<HTMLElement>('[aria-current="page"]')?.scrollIntoView({ block: 'nearest', inline: 'center' });
  });
</script>

<div class="page">
  <div class="title-row">
    <h1>{t('settings.title')}</h1>
    <!-- The sidebar (with sign-out) is hidden on phones. -->
    <div class="account-mobile">
      <span class="muted">{auth.state?.username}</span>
      <button class="btn small" type="button" onclick={logout}><LogOut size={15} /> {t('nav.logout')}</button>
    </div>
  </div>
  <div class="layout">
    <nav class="subnav" aria-label={t('settings.title')} bind:this={subnav}>
      {#each sections as s (s.id)}
        <a href="/settings/{s.id}" use:link aria-current={current.id === s.id ? 'page' : undefined}>{t(s.label)}</a>
      {/each}
    </nav>
    <div class="content">
      <h2 class="sr-only">{t(current.label)}</h2>
      {#if store.settings}
        <Comp />
      {:else}
        <div class="skeleton" style:height="200px"></div>
      {/if}
    </div>
  </div>
</div>

<style>
  .page {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 18px;
    max-width: 1080px;
  }
  .layout {
    display: grid;
    grid-template-columns: 210px minmax(0, 1fr);
    gap: 28px;
    align-items: start;
  }
  .subnav {
    position: sticky;
    top: 20px;
    display: grid;
    gap: 2px;
  }
  .subnav a {
    padding: 8px 10px;
    border-radius: 7px;
    color: var(--text-2);
    font-weight: 520;
    text-decoration: none;
  }
  .subnav a:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .subnav a[aria-current='page'] {
    background: var(--surface);
    color: var(--text);
    box-shadow: inset 0 0 0 1px var(--border);
  }
  .content {
    display: grid;
    gap: 16px;
    min-width: 0;
  }
  .title-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }
  .account-mobile {
    display: none;
    align-items: center;
    gap: 10px;
  }
  @media (max-width: 639px) {
    .account-mobile {
      display: flex;
    }
  }
  @media (max-width: 860px) {
    .layout {
      grid-template-columns: 1fr;
      gap: 14px;
    }
    .subnav {
      position: static;
      display: flex;
      overflow-x: auto;
      gap: 4px;
      scrollbar-width: none;
      border-bottom: 1px solid var(--border);
      padding-bottom: 8px;
    }
    .subnav a {
      white-space: nowrap;
    }
  }
</style>
