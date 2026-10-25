<script lang="ts">
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { toast } from '../../lib/components/toast.svelte';
  import { store, updateSettings } from '../../lib/state.svelte';
  import { duration, number, t } from '../../lib/i18n.svelte';

  const s = store.settings!;
  let interfaces = $state<string[]>([...(s.interfaces ?? [])]);
  let scanInterval = $state(s.scanInterval);
  let offlineAfter = $state(s.offlineAfter);
  let offlineAfterMobile = $state(s.offlineAfterMobile);
  let allowLarge = $state(s.allowLargeSubnets);
  let resolveNames = $state(s.resolveNames);
  let busy = $state(false);
  let error = $state('');

  const ifaces = $derived(store.overview?.scanner.interfaces ?? []);
  const mode = $derived(store.overview?.scanner.mode ?? 'raw');
  const dirty = $derived(
    JSON.stringify([...interfaces].sort()) !== JSON.stringify([...(s.interfaces ?? [])].sort()) ||
      scanInterval !== s.scanInterval || offlineAfter !== s.offlineAfter || offlineAfterMobile !== s.offlineAfterMobile ||
      allowLarge !== s.allowLargeSubnets || resolveNames !== s.resolveNames,
  );

  const intervals = [60, 90, 120, 300, 600];
  const offlines = [180, 300, 600, 900, 1800, 3600];

  function toggleIface(name: string, on: boolean) {
    interfaces = on ? [...interfaces, name] : interfaces.filter((i) => i !== name);
  }

  async function save() {
    busy = true;
    error = await updateSettings((x) => {
      x.interfaces = interfaces;
      x.scanInterval = scanInterval;
      x.offlineAfter = Math.max(offlineAfter, scanInterval * 2);
      x.offlineAfterMobile = offlineAfterMobile;
      x.allowLargeSubnets = allowLarge;
      x.resolveNames = resolveNames;
    });
    busy = false;
    if (!error) toast(t('common.saved'));
  }
</script>

<SettingsCard title={t('settings.interfaces')} lead={t('settings.interfacesHint')} {dirty} {busy} {error} onsave={save}>
  <div class="ifaces">
    {#each ifaces as i (i.name)}
      <label class="check iface">
        <input type="checkbox" checked={interfaces.includes(i.name) || (interfaces.length === 0 && i.default)}
          disabled={interfaces.length === 0 && i.default}
          onchange={(e) => toggleIface(i.name, e.currentTarget.checked)} />
        <span class="iface-text">
          <span class="row"><strong class="mono">{i.name}</strong>
            {#if i.default}<span class="badge accent">{t('settings.interfaceDefault')}</span>{/if}
          </span>
          <span class="muted"><span class="mono">{i.prefix}</span> · {t('settings.hosts', { n: number(i.hosts) })}</span>
          {#if i.hosts > 65536}<span class="hint warn">{t('settings.tooLarge')}</span>
          {:else if i.hosts > 1022}<span class="hint warn">{t('settings.large')}</span>{/if}
        </span>
      </label>
    {/each}
  </div>
  <div class="three">
    <div class="field">
      <label for="si">{t('settings.scanInterval')}</label>
      <select id="si" class="select" bind:value={scanInterval}>
        {#each intervals as v (v)}<option value={v}>{duration(v * 1000)}</option>{/each}
      </select>
    </div>
    <div class="field">
      <label for="oa">{t('settings.offlineAfter')}</label>
      <select id="oa" class="select" bind:value={offlineAfter}>
        {#each offlines.filter((v) => v >= scanInterval * 2) as v (v)}<option value={v}>{duration(v * 1000)}</option>{/each}
      </select>
    </div>
    <div class="field">
      <label for="om">{t('settings.offlineAfterMobile')}</label>
      <select id="om" class="select" bind:value={offlineAfterMobile}>
        {#each offlines as v (v)}<option value={v}>{duration(v * 1000)}</option>{/each}
      </select>
    </div>
  </div>
  <p class="hint">{t('settings.offlineHint')}</p>
  <Toggle bind:checked={allowLarge} label={t('settings.allowLarge')} />
  <Toggle bind:checked={resolveNames} label={t('settings.resolveNames')} />
  <p class="muted">{t('settings.mode')}: <strong>{t(`settings.mode.${mode}`)}</strong></p>
</SettingsCard>

<style>
  .ifaces {
    display: grid;
    gap: 10px;
  }
  .iface {
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  .iface-text {
    display: grid;
    gap: 2px;
  }
  .warn {
    color: var(--warn);
  }
  .three {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 16px;
  }
  @media (max-width: 760px) {
    .three {
      grid-template-columns: 1fr;
    }
  }
</style>
