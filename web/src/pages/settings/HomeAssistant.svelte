<script lang="ts">
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { toast } from '../../lib/components/toast.svelte';
  import { api } from '../../lib/api';
  import { store, updateSettings } from '../../lib/state.svelte';
  import { t } from '../../lib/i18n.svelte';
  import type { MQTTSettings } from '../../lib/types';

  const MASK = '••••••••';
  const s = store.settings!;
  let m = $state<MQTTSettings>({ ...s.mqtt, password: '' });
  const hasPassword = s.mqtt.password === MASK;
  let busy = $state(false);
  let error = $state('');
  let mqttState = $state<{ state: string; error?: string } | null>(null);

  async function loadStatus() {
    try {
      mqttState = (await api.system()).mqtt;
    } catch {
      /* ignore */
    }
  }
  $effect(() => {
    void loadStatus();
    const id = setInterval(loadStatus, 5000);
    return () => clearInterval(id);
  });

  const dirty = $derived(JSON.stringify({ ...m, password: '' }) !== JSON.stringify({ ...s.mqtt, password: '' }) || m.password !== '');

  async function save() {
    busy = true;
    error = await updateSettings((x) => {
      x.mqtt = { ...m, port: Number(m.port) || 1883, password: m.password };
    });
    busy = false;
    if (!error) {
      m.password = '';
      toast(t('common.saved'));
      void loadStatus();
    }
  }
</script>

<SettingsCard title={t('settings.homeassistant')} lead={t('settings.mqtt.lead')} {dirty} {busy} {error} onsave={save}>
  {#snippet extra()}
    {#if mqttState}
      <span class="badge {mqttState.state === 'connected' ? 'ok' : mqttState.state === 'error' ? 'danger' : ''}" title={mqttState.error ?? ''}>
        {t(`settings.mqtt.state.${mqttState.state}`)}
      </span>
    {/if}
  {/snippet}
  <Toggle bind:checked={m.enabled} label={t('settings.mqtt.enabled')} />
  {#if mqttState?.state === 'error' && mqttState.error}<p class="error-text">{mqttState.error}</p>{/if}
  <div class="grid">
    <div class="field wide">
      <label for="mh">{t('settings.mqtt.host')}</label>
      <input id="mh" class="input" bind:value={m.host} placeholder="192.168.1.10" disabled={!m.enabled} />
    </div>
    <div class="field">
      <label for="mp">{t('settings.mqtt.port')}</label>
      <input id="mp" class="input" type="number" min="1" max="65535" bind:value={m.port} disabled={!m.enabled} />
    </div>
    <div class="field">
      <label for="mu">{t('settings.mqtt.username')}</label>
      <input id="mu" class="input" bind:value={m.username} autocomplete="off" disabled={!m.enabled} />
    </div>
    <div class="field">
      <label for="mpw">{t('settings.mqtt.password')}</label>
      <input id="mpw" class="input" type="password" bind:value={m.password} autocomplete="new-password" disabled={!m.enabled}
        placeholder={hasPassword ? t('settings.secretKept') : ''} />
    </div>
    <div class="field">
      <label for="mpre">{t('settings.mqtt.prefix')}</label>
      <input id="mpre" class="input mono" bind:value={m.discoveryPrefix} disabled={!m.enabled} />
    </div>
    <div class="field">
      <label for="mb">{t('settings.mqtt.base')}</label>
      <input id="mb" class="input mono" bind:value={m.baseTopic} disabled={!m.enabled} />
    </div>
  </div>
  <Toggle bind:checked={m.tls} label={t('settings.mqtt.tls')} disabled={!m.enabled} />
  <p class="hint">{t('settings.localhostHint')}</p>
</SettingsCard>

<style>
  .grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 14px 16px;
  }
  .wide {
    grid-column: span 1;
  }
  @media (max-width: 640px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
