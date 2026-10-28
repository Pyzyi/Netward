<script lang="ts">
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { toast } from '../../lib/components/toast.svelte';
  import { store, updateSettings } from '../../lib/state.svelte';
  import { api } from '../../lib/api';
  import { duration, number, t } from '../../lib/i18n.svelte';
  import type { TripwireStatus } from '../../lib/types';

  const init = store.settings!; // starting values of the form
  const s = $derived(store.settings!); // last saved values, for dirty tracking
  let interfaces = $state<string[]>([...(init.interfaces ?? [])]);
  let scanInterval = $state(init.scanInterval);
  let offlineAfter = $state(init.offlineAfter);
  let offlineAfterMobile = $state(init.offlineAfterMobile);
  let allowLarge = $state(init.allowLargeSubnets);
  let resolveNames = $state(init.resolveNames);
  let schedule = $state(store.settings!.securityScanDays);
  async function saveSchedule() {
    const err = await updateSettings((x) => (x.securityScanDays = Number(schedule)));
    toast(err ? t('err.invalid_settings', { msg: err }) : t('common.saved'), err ? 'error' : 'ok');
  }
  let dhcpCheck = $state(init.dhcpCheck ?? true);
  async function saveDhcpCheck(v: boolean) {
    const err = await updateSettings((x) => (x.dhcpCheck = v));
    if (err) dhcpCheck = !v;
    toast(err ? t('err.invalid_settings', { msg: err }) : t('common.saved'), err ? 'error' : 'ok');
  }
  let dnsCheck = $state(init.dnsCheck ?? true);
  async function saveDnsCheck(v: boolean) {
    const err = await updateSettings((x) => (x.dnsCheck = v));
    if (err) dnsCheck = !v;
    toast(err ? t('err.invalid_settings', { msg: err }) : t('common.saved'), err ? 'error' : 'ok');
  }
  let busy = $state(false);
  let error = $state('');

  // Tripwire (docs/10, F3).
  const split = (v: string) => v.split(/[\s,;]+/).filter(Boolean);
  let tripOn = $state(init.tripwire ?? false);
  let tripPorts = $state((init.tripwirePorts ?? []).join(', '));
  let tripAllow = $state((init.tripwireAllow ?? []).join(', '));
  let tripBusy = $state(false);
  let tripError = $state('');
  let tripStatus = $state<TripwireStatus | undefined>();
  const portList = $derived(split(tripPorts).map(Number));
  const portsValid = $derived(portList.every((p) => Number.isInteger(p) && p >= 1024 && p <= 65535) && new Set(portList).size === portList.length);
  const tripDirty = $derived(
    tripOn !== (s.tripwire ?? false) || JSON.stringify(portList) !== JSON.stringify(s.tripwirePorts ?? []) ||
      JSON.stringify(split(tripAllow)) !== JSON.stringify(s.tripwireAllow ?? []),
  );
  async function loadTrip() {
    try {
      tripStatus = (await api.network()).tripwire;
    } catch {
      /* the form still works without the status */
    }
  }
  loadTrip();
  async function saveTrip() {
    if (!portsValid) {
      tripError = t('settings.tripwireBadPorts');
      return;
    }
    tripBusy = true;
    tripError = await updateSettings((x) => {
      x.tripwire = tripOn;
      x.tripwirePorts = portList;
      x.tripwireAllow = split(tripAllow);
    });
    tripBusy = false;
    if (!tripError) {
      toast(t('common.saved'));
      await loadTrip();
    }
  }

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

<SettingsCard title={t('settings.securitySchedule')}>
  <div class="field narrow">
    <select class="select" bind:value={schedule} onchange={saveSchedule} aria-label={t('settings.securitySchedule')}>
      <option value={0}>{t('settings.schedule.off')}</option>
      <option value={1}>{t('settings.schedule.daily')}</option>
      <option value={7}>{t('settings.schedule.weekly')}</option>
    </select>
  </div>
  <Toggle bind:checked={dhcpCheck} label={t('settings.dhcpCheck')} hint={t('settings.dhcpCheckHint')} onchange={saveDhcpCheck} />
  <Toggle bind:checked={dnsCheck} label={t('settings.dnsCheck')} hint={t('settings.dnsCheckHint')} onchange={saveDnsCheck} />
</SettingsCard>

<SettingsCard title={t('settings.tripwire')} lead={t('settings.tripwireLead')} dirty={tripDirty} busy={tripBusy} error={tripError} onsave={saveTrip}>
  <Toggle bind:checked={tripOn} label={t('settings.tripwireOn')} />
  <div class="two">
    <div class="field">
      <label for="tw-ports">{t('settings.tripwirePorts')}</label>
      <input id="tw-ports" class="input mono" inputmode="numeric" autocomplete="off" bind:value={tripPorts} aria-invalid={!portsValid} aria-describedby="tw-ports-hint" />
      <p class="hint" id="tw-ports-hint">{t('settings.tripwirePortsHint')}</p>
    </div>
    <div class="field">
      <label for="tw-allow">{t('settings.tripwireAllow')}</label>
      <input id="tw-allow" class="input mono" autocomplete="off" bind:value={tripAllow} aria-describedby="tw-allow-hint" />
      <p class="hint" id="tw-allow-hint">{t('settings.tripwireAllowHint')}</p>
    </div>
  </div>
  {#if tripStatus?.enabled && tripStatus.ports.length}
    <div class="field">
      <span class="label">{t('settings.tripwireStatus')}</span>
      <ul class="tw-ports">
        {#each tripStatus.ports as p (p.port)}
          <li><span class="dot {p.state === 'open' ? 'ok' : 'warn'}" aria-hidden="true"></span>
            <span class="mono">{p.port}</span> {t(`tripwire.svc.${p.service}`)} <span class="faint">· {t(`tripwire.state.${p.state}`)}</span></li>
        {/each}
      </ul>
    </div>
  {/if}
</SettingsCard>

<style>
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
  }
  .tw-ports {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 4px;
  }
  .tw-ports li {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .narrow {
    max-width: 280px;
  }
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
    .two,
    .three {
      grid-template-columns: 1fr;
    }
  }
</style>
