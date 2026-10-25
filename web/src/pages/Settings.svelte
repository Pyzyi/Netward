<script lang="ts">
  import Icon from '../lib/components/Icon.svelte';
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

<div class="wrap">
  <header class="page-head">
    <h1>{t('settings.title')}</h1>
    <!-- The band (with sign-out) is reduced on phones. -->
    <div class="account-mobile">
      <span class="muted">{auth.state?.username}</span>
      <button class="btn small" type="button" onclick={logout}><Icon name="sign-out" size={15} /> {t('nav.logout')}</button>
    </div>
  </header>
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
  .layout {
    display: grid;
    grid-template-columns: 200px minmax(0, 1fr);
    gap: 40px;
    align-items: start;
  }
  .subnav {
    position: sticky;
    top: 80px;
    display: grid;
  }
  .subnav a {
    padding: 8px 0 8px 14px;
    border-left: 1px solid var(--border-strong);
    color: var(--text-2);
    font-weight: 600;
    text-decoration: none;
  }
  .subnav a:hover {
    color: var(--text);
  }
  .subnav a[aria-current='page'] {
    color: var(--text);
    border-left: 3px solid var(--text);
    padding-left: 12px;
  }
  /* Every settings page uses one measure: fields, rules, notices, buttons. */
  .content {
    display: grid;
    gap: 36px;
    min-width: 0;
    max-width: 680px;
  }
  /* The page rule above already separates the first section. */
  .content > :global(.block:first-of-type) {
    border-top: 0;
  }
  .content > :global(.block:first-of-type .block-head) {
    padding-top: 0;
  }
  .account-mobile {
    display: none;
    align-items: center;
    gap: 10px;
  }
  @media (max-width: 860px) {
    .account-mobile {
      display: flex;
    }
    .layout {
      grid-template-columns: 1fr;
      gap: 18px;
    }
    .subnav {
      position: static;
      display: flex;
      overflow-x: auto;
      scrollbar-width: none;
      border-bottom: 1px solid var(--border);
    }
    .subnav a {
      white-space: nowrap;
      border-left: 0;
      padding: 8px 12px 10px 0;
      margin-right: 6px;
    }
    .subnav a[aria-current='page'] {
      border-left: 0;
      padding-left: 0;
      box-shadow: inset 0 -2px 0 var(--text);
    }
  }
</style>
