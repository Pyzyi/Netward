<script lang="ts">
  import Icon from '../../lib/components/Icon.svelte';
  import Dialog from '../../lib/components/Dialog.svelte';
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

  let confirmAck = $state(false);
  const unknownCount = $derived([...store.devices.values()].filter((d) => !d.known && !d.isSelf).length);
  async function ackAll() {
    confirmAck = false;
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
    <a class="btn" href="/api/v1/export?format=csv" download><Icon name="download-simple" size={15} /> CSV</a>
    <a class="btn" href="/api/v1/export?format=json" download><Icon name="download-simple" size={15} /> JSON</a>
  </div>
</SettingsCard>

<SettingsCard title={t('settings.ackAll')}>
  <div><button class="btn" type="button" disabled={unknownCount === 0} onclick={() => (confirmAck = true)}><Icon name="check" size={15} /> {t('settings.ackAll')}</button></div>
</SettingsCard>

<Dialog bind:open={confirmAck} title={t('overview.markAllTitle', { n: unknownCount })}>
  <p>{t('overview.markAllText')}</p>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (confirmAck = false)}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" onclick={ackAll}>{t('settings.ackAll')}</button>
  {/snippet}
</Dialog>

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
