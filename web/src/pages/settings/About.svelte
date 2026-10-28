<script lang="ts">
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import Logo from '../../lib/components/Logo.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { toast } from '../../lib/components/toast.svelte';
  import { api } from '../../lib/api';
  import { clock, store, updateSettings } from '../../lib/state.svelte';
  import { bytes, dateLong, duration, relTime, t } from '../../lib/i18n.svelte';
  import type { SystemInfo } from '../../lib/types';

  let info = $state<SystemInfo | null>(null);
  const load = () => api.system().then((i) => (info = i), () => {});
  void load();

  const init = store.settings!;
  const s = $derived(store.settings!);
  let check = $state(init.updateCheck ?? false);
  let busy = $state(false);
  let error = $state('');
  const dirty = $derived(check !== (s.updateCheck ?? false));
  const upd = $derived(info?.update);

  async function save() {
    busy = true;
    error = await updateSettings((x) => {
      x.updateCheck = check;
    });
    busy = false;
    if (!error) {
      toast(t('common.saved'));
      // The first check runs right away; show its result shortly after.
      setTimeout(load, 3000);
    }
  }
</script>

<SettingsCard>
  <div class="brand"><Logo size={40} /><div><strong>Netzwart</strong><p class="muted">{info?.version ?? ''}</p></div></div>
  {#if info}
    <dl class="dl">
      <dt>{t('settings.about.version')}</dt><dd>{info.version} <span class="faint">({info.goVersion})</span></dd>
      <dt>{t('settings.about.softwareData')}</dt><dd>{dateLong(info.softwareDataAsOf)}</dd>
      <dt>{t('settings.about.mode')}</dt><dd>{t(`settings.mode.${info.mode}`)}</dd>
      <dt>{t('settings.about.user')}</dt><dd>UID {info.uid}</dd>
      <dt>{t('settings.about.uptime')}</dt><dd>{duration(clock.now - info.startedAt)}</dd>
      <dt>{t('settings.about.db')}</dt><dd>{bytes(info.dbSize)}</dd>
      <dt>{t('settings.about.memory')}</dt><dd>{bytes(info.memory)}</dd>
    </dl>
  {/if}
  <p class="hint">{t('settings.about.license')}</p>
</SettingsCard>

<SettingsCard title={t('settings.updates')} lead={t('settings.updates.lead')} {dirty} {busy} {error} onsave={save}>
  <Toggle bind:checked={check} label={t('settings.updates.check')} hint={t('settings.updates.privacy')} />
  {#if upd?.enabled}
    {#if upd.available && upd.url}
      <p class="notice info">
        <span>{t('settings.updates.available', { v: upd.latest ?? '' })} <a href={upd.url} target="_blank" rel="noopener noreferrer">{t('settings.updates.notes')}</a></span>
      </p>
    {:else if upd.error}
      <p class="muted small">{t('settings.updates.failed')}</p>
    {:else if upd.checkedAt}
      <p class="muted small">{upd.latest ? t('settings.updates.current', { v: upd.latest }) : ''} · {relTime(upd.checkedAt, clock.now)}</p>
    {/if}
  {/if}
  <div class="howto">
    <p class="label">{t('settings.updates.how')}</p>
    <p class="small">{t('settings.updates.howZima')}</p>
    <pre class="mono">docker compose pull
docker compose up -d</pre>
    <p class="hint">{t('settings.updates.keep')}</p>
  </div>
</SettingsCard>

<style>
  .brand {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .howto {
    display: grid;
    gap: 6px;
  }
  pre {
    margin: 0;
    padding: 10px 12px;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: 4px;
    font-size: 13px;
    overflow-x: auto;
  }
  .small {
    font-size: 13px;
  }
</style>
