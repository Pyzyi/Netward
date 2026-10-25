<script lang="ts">
  import Download from '@lucide/svelte/icons/download';
  import Check from '@lucide/svelte/icons/check';
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import { toast } from '../../lib/components/toast.svelte';
  import { api } from '../../lib/api';
  import { store, updateSettings } from '../../lib/state.svelte';
  import { t } from '../../lib/i18n.svelte';

  const s = store.settings!;
  let retention = $state(s.retentionDays);
  let expire = $state(s.expireRandomDays);

  async function save() {
    const err = await updateSettings((x) => {
      x.retentionDays = Number(retention);
      x.expireRandomDays = Number(expire);
    });
    toast(err ? t('err.invalid_settings', { msg: err }) : t('common.saved'), err ? 'error' : 'ok');
  }

  async function ackAll() {
    const r = await api.acknowledgeAll();
    toast(t('settings.ackDone', { n: r.count }));
  }
</script>

<SettingsCard title={t('settings.data')}>
  <div class="two">
    <div class="field">
      <label for="ret">{t('settings.retention')}</label>
      <select id="ret" class="select" bind:value={retention} onchange={save}>
        {#each [30, 90, 180, 365, 730] as d (d)}<option value={d}>{t('common.days', { n: d })}</option>{/each}
      </select>
    </div>
    <div class="field">
      <label for="exp">{t('settings.expireRandom')}</label>
      <select id="exp" class="select" bind:value={expire} onchange={save}>
        <option value={0}>{t('settings.expireNever')}</option>
        {#each [7, 14, 30, 90] as d (d)}<option value={d}>{t('common.days', { n: d })}</option>{/each}
      </select>
    </div>
  </div>
  <p class="hint">{t('settings.backupHint')}</p>
</SettingsCard>

<SettingsCard title={t('settings.export')} lead={t('settings.exportHint')}>
  <div class="row wrap">
    <a class="btn" href="/api/v1/export?format=csv" download><Download size={15} /> CSV</a>
    <a class="btn" href="/api/v1/export?format=json" download><Download size={15} /> JSON</a>
  </div>
</SettingsCard>

<SettingsCard title={t('settings.ackAll')}>
  <div><button class="btn" type="button" onclick={ackAll}><Check size={15} /> {t('settings.ackAll')}</button></div>
</SettingsCard>

<style>
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
  }
  .wrap {
    flex-wrap: wrap;
  }
  @media (max-width: 640px) {
    .two {
      grid-template-columns: 1fr;
    }
  }
</style>
