<script lang="ts">
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import Sparkline from '../../lib/components/Sparkline.svelte';
  import { toast } from '../../lib/components/toast.svelte';
  import { api } from '../../lib/api';
  import { clock, store, updateSettings } from '../../lib/state.svelte';
  import { dateTime, duration, relTime, t } from '../../lib/i18n.svelte';

  const init = store.settings!; // starting values of the form
  const s = $derived(store.settings!); // last saved values, for dirty tracking
  let enabled = $state(init.internetCheck);
  let targets = $state((init.internetTargets ?? []).join('\n'));
  let busy = $state(false);
  let error = $state('');
  let data = $state<Awaited<ReturnType<typeof api.internet>> | null>(null);

  async function load() {
    try {
      data = await api.internet();
    } catch {
      /* ignore */
    }
  }
  $effect(() => {
    void load();
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  });

  const list = $derived(targets.split(/[\n,]+/).map((x) => x.trim()).filter(Boolean));
  const dirty = $derived(enabled !== s.internetCheck || list.join(',') !== (s.internetTargets ?? []).join(','));

  async function save() {
    busy = true;
    error = await updateSettings((x) => {
      x.internetCheck = enabled;
      x.internetTargets = list;
    });
    busy = false;
    if (!error) toast(t('common.saved'));
  }
  const st = $derived(data?.status ?? store.overview?.internet);
</script>

<SettingsCard title={t('settings.internet')} lead={t('settings.internet.lead')} {dirty} {busy} {error} onsave={save}>
  <Toggle bind:checked={enabled} label={t('settings.internet.enabled')} />
  {#if st}
    <p>
      <strong>{t(`internet.${st.state}`)}</strong>
      {#if st.state === 'online' && st.latencyMs >= 0}<span class="muted"> · {t('internet.latency', { ms: st.latencyMs })}</span>{/if}
      {#if st.checkedAt}<span class="faint"> · {relTime(st.checkedAt, clock.now)}</span>{/if}
    </p>
  {/if}
  {#if data && data.history.length > 2}
    <div class="field">
      <span class="label">{t('settings.internet.history')}</span>
      <div class="spark"><Sparkline points={data.history} height={48} /></div>
    </div>
  {/if}
  {#if data}
    <div>
      <p class="label">{data.outages.length ? t('internet.outages', { n: data.outages.length }) : t('internet.noOutages')}</p>
      <ul class="outages">
        {#each data.outages.slice(0, 8) as o (o.id)}
          <li><span>{dateTime(o.start)}</span><span class="muted">{o.end ? duration(o.end - o.start) : '…'}</span></li>
        {/each}
      </ul>
    </div>
  {/if}
  <div class="field">
    <label for="tg">{t('settings.internet.targets')}</label>
    <textarea id="tg" class="textarea mono" rows="3" bind:value={targets} disabled={!enabled}></textarea>
  </div>
</SettingsCard>

<style>
  .spark {
    padding: 8px;
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  .outages {
    list-style: none;
    padding: 0;
    margin: 6px 0 0;
    display: grid;
    gap: 4px;
  }
  .outages li {
    display: flex;
    justify-content: space-between;
    max-width: 380px;
  }
</style>
