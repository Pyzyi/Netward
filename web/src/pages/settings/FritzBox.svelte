<script lang="ts">
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { toast } from '../../lib/components/toast.svelte';
  import { api } from '../../lib/api';
  import { clock, store, updateSettings } from '../../lib/state.svelte';
  import { relTime, t } from '../../lib/i18n.svelte';
  import type { FritzBoxSettings, SystemInfo } from '../../lib/types';

  const MASK = '••••••••';
  const init = store.settings!; // starting values of the form
  const s = $derived(store.settings!); // last saved values, for dirty tracking
  let f = $state<FritzBoxSettings>({ ...init.fritzbox, password: '' });
  const hasPassword = $derived(s.fritzbox.password === MASK);
  let busy = $state(false);
  let error = $state('');
  let status = $state<SystemInfo['fritzbox']>(null);

  async function loadStatus() {
    try {
      status = (await api.system()).fritzbox;
    } catch {
      /* ignore */
    }
  }
  $effect(() => {
    void loadStatus();
    const id = setInterval(loadStatus, 5000);
    return () => clearInterval(id);
  });

  const dirty = $derived(JSON.stringify({ ...f, password: '' }) !== JSON.stringify({ ...s.fritzbox, password: '' }) || f.password !== '');

  async function save() {
    busy = true;
    error = await updateSettings((x) => {
      x.fritzbox = { ...f, host: f.host.trim(), username: f.username.trim() };
    });
    busy = false;
    if (!error) {
      f.password = '';
      toast(t('common.saved'));
      void loadStatus();
    }
  }
</script>

<SettingsCard title="FRITZ!Box" lead={t('settings.fritz.lead')} {dirty} {busy} {error} onsave={save}>
  {#snippet extra()}
    {#if status}
      <span class="badge {status.state === 'connected' ? 'ok' : status.state === 'error' ? 'danger' : ''}">
        {t(`settings.mqtt.state.${status.state}`)}
      </span>
    {/if}
  {/snippet}
  <Toggle bind:checked={f.enabled} label={t('settings.fritz.enabled')} />
  {#if status?.state === 'error' && status.error}
    <p class="error-text" role="alert">{t(`settings.fritz.error.${status.error}`)}</p>
  {:else if status?.state === 'connected'}
    <p class="muted small">{t('settings.fritz.synced', { n: status.hosts, when: relTime(status.lastSync, clock.now) })}</p>
  {/if}
  <div class="grid">
    <div class="field">
      <label for="fh">{t('settings.fritz.host')}</label>
      <input id="fh" class="input" bind:value={f.host} placeholder="fritz.box" disabled={!f.enabled} />
    </div>
    <div class="field">
      <label for="fu">{t('settings.mqtt.username')}</label>
      <input id="fu" class="input" bind:value={f.username} autocomplete="off" disabled={!f.enabled} />
    </div>
    <div class="field">
      <label for="fp">{t('settings.mqtt.password')}</label>
      <input id="fp" class="input" type="password" bind:value={f.password} autocomplete="new-password" disabled={!f.enabled}
        placeholder={hasPassword ? t('settings.secretKept') : ''} />
    </div>
  </div>
  <details class="howto">
    <summary>{t('settings.fritz.howto')}</summary>
    <ol>
      <li>{t('settings.fritz.step1')}</li>
      <li>{t('settings.fritz.step2')}</li>
      <li>{t('settings.fritz.step3')}</li>
    </ol>
    <p class="hint">{t('settings.fritz.security')}</p>
  </details>
</SettingsCard>

<style>
  .grid {
    display: grid;
    grid-template-columns: 1fr 1fr 1fr;
    gap: 14px 16px;
  }
  .small {
    font-size: 13px;
  }
  .howto {
    border-top: 1px solid var(--border);
    padding-top: 12px;
    font-size: 14px;
    color: var(--text-2);
  }
  .howto summary {
    cursor: pointer;
    font-weight: 600;
    color: var(--text-1);
  }
  .howto ol {
    margin: 10px 0 8px;
    padding-left: 20px;
    display: grid;
    gap: 6px;
  }
  @media (max-width: 760px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
