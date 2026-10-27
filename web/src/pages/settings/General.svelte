<script lang="ts">
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import { toast } from '../../lib/components/toast.svelte';
  import { store, updateSettings } from '../../lib/state.svelte';
  import { setLang, t, type Lang } from '../../lib/i18n.svelte';

  const init = store.settings!; // starting values of the form
  const s = $derived(store.settings!); // last saved values, for dirty tracking
  let networkName = $state(init.networkName);
  let language = $state<Lang>(init.language);
  let timezone = $state(init.timezone);
  let busy = $state(false);
  let error = $state('');

  const zones = (() => {
    try {
      return (Intl as unknown as { supportedValuesOf(k: string): string[] }).supportedValuesOf('timeZone');
    } catch {
      return [s.timezone, 'UTC', 'Europe/Berlin', 'Europe/Vienna', 'Europe/Zurich'];
    }
  })();

  const dirty = $derived(networkName !== s.networkName || language !== s.language || timezone !== s.timezone);

  async function save() {
    busy = true;
    error = await updateSettings((x) => {
      x.networkName = networkName.trim();
      x.language = language;
      x.timezone = timezone;
    });
    busy = false;
    if (!error) {
      setLang(language);
      toast(t('common.saved'));
    }
  }

  let theme = $state(readTheme());
  function readTheme() {
    try {
      return localStorage.getItem('netzwart.theme') ?? 'system';
    } catch {
      return 'system';
    }
  }
  function applyTheme(v: string) {
    theme = v;
    try {
      if (v === 'system') localStorage.removeItem('netzwart.theme');
      else localStorage.setItem('netzwart.theme', v);
    } catch {
      /* ignore */
    }
    if (v === 'system') delete document.documentElement.dataset.theme;
    else document.documentElement.dataset.theme = v;
  }
</script>

<SettingsCard {dirty} {busy} {error} onsave={save}>
  <div class="field">
    <label for="nn">{t('settings.networkName')}</label>
    <input id="nn" class="input" bind:value={networkName} maxlength="64" placeholder={t('setup.networkPlaceholder')} />
  </div>
  <div class="two">
    <div class="field">
      <label for="lang">{t('settings.language')}</label>
      <select id="lang" class="select" bind:value={language}>
        <option value="de">Deutsch</option>
        <option value="en">English</option>
      </select>
    </div>
    <div class="field">
      <label for="tz">{t('settings.timezone')}</label>
      <select id="tz" class="select" bind:value={timezone}>
        {#each zones as z (z)}<option value={z}>{z}</option>{/each}
      </select>
      <p class="hint">{t('settings.timezoneHint')}</p>
    </div>
  </div>
</SettingsCard>

<SettingsCard title={t('settings.theme')}>
  <div class="seg" role="group" aria-label={t('settings.theme')}>
    {#each ['system', 'light', 'dark'] as v (v)}
      <button type="button" aria-pressed={theme === v} onclick={() => applyTheme(v)}>{t(`settings.theme.${v}`)}</button>
    {/each}
  </div>
</SettingsCard>

<style>
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
  }
  @media (max-width: 640px) {
    .two {
      grid-template-columns: 1fr;
    }
  }
</style>
