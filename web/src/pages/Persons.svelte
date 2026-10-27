<script lang="ts">
  import Icon from '../lib/components/Icon.svelte';
  import DeviceIcon from '../lib/components/DeviceIcon.svelte';
  import Dialog from '../lib/components/Dialog.svelte';
  import Toggle from '../lib/components/Toggle.svelte';
  import { toast } from '../lib/components/toast.svelte';
  import { api } from '../lib/api';
  import { errorText } from '../lib/errors';
  import { clock, store } from '../lib/state.svelte';
  import { link } from '../lib/router.svelte';
  import { displayName } from '../lib/devices';
  import { dateLong, duration, i18n, number, t } from '../lib/i18n.svelte';
  import { initials, isHome, presenceText } from '../lib/persons';
  import type { Device, PersonStatus } from '../lib/types';

  // Persons instead of devices (docs/09, F4). Privacy by default: only
  // "at home since"; time per day is opt-in and needs the person's consent.
  let people = $state<PersonStatus[] | null>(null);
  let newName = $state('');
  let busy = $state(false);
  let consentFor = $state<PersonStatus | null>(null);
  let consentMode = $state<'time' | 'ha'>('time');
  let consentOpen = $state(false);
  let consentOk = $state(false);
  let deleteFor = $state<PersonStatus | null>(null);
  let deleteOpen = $state(false);
  // Closing the consent dialog without confirming puts the switch back
  // (the switch flips itself on click; re-creating it shows the real state).
  let resetSwitches = $state(0);
  $effect(() => {
    if (!consentOpen && consentFor) {
      consentFor = null;
      resetSwitches++;
    }
  });
  let renaming = $state<number | null>(null);
  let renameValue = $state('');

  async function load() {
    try {
      people = await api.persons();
    } catch (e) {
      toast(errorText(e), 'error');
      people ??= [];
    }
  }
  $effect(() => {
    void load();
    const timer = setInterval(load, 60_000);
    return () => clearInterval(timer);
  });

  async function run(fn: () => Promise<unknown>) {
    busy = true;
    try {
      await fn();
      await load();
    } catch (e) {
      toast(errorText(e), 'error');
      resetSwitches++; // a switch flipped itself; show the saved state again
    } finally {
      busy = false;
    }
  }

  const assigned = $derived(new Set((people ?? []).flatMap((p) => p.devices.map((d) => d.deviceId))));
  // Devices people carry first: phones, watches, tablets, laptops.
  const order: Record<string, number> = { phone: 0, watch: 1, tablet: 2, laptop: 3 };
  const free = $derived(
    [...store.devices.values()]
      .filter((d) => !assigned.has(d.id) && !d.isSelf)
      .sort((a, b) => (order[a.type] ?? 9) - (order[b.type] ?? 9) || displayName(a).localeCompare(displayName(b))),
  );
  const carried = (d: Device) => d.type === 'phone' || d.type === 'watch';

  function options(p: PersonStatus, patch: Partial<{ name: string; trackTime: boolean; consent: boolean; homeAssistant: boolean }>) {
    return run(() => api.updatePerson(p.id, { name: p.name, trackTime: p.trackTime, homeAssistant: p.homeAssistant, ...patch }));
  }
  // Both options record a person; the first one switched on asks for the
  // confirmation that the person knows.
  function switchOn(p: PersonStatus, mode: 'time' | 'ha', on: boolean) {
    const patch = mode === 'time' ? { trackTime: on } : { homeAssistant: on };
    if (!on || p.consentAt > 0) {
      void options(p, patch);
      return;
    }
    consentFor = p;
    consentMode = mode;
    consentOk = false;
    consentOpen = true;
  }
  function setDevices(p: PersonStatus, devices: PersonStatus['devices']) {
    return run(() => api.setPersonDevices(p.id, devices));
  }
  function addDevice(p: PersonStatus, id: number) {
    const d = store.devices.get(id);
    if (!d) return;
    void setDevices(p, [...p.devices, { deviceId: id, presence: carried(d) }]);
  }
  function hours(ms: number) {
    return number(Math.round((ms / 3_600_000) * 10) / 10);
  }
  function weekday(date: string) {
    return new Intl.DateTimeFormat(i18n.lang === 'de' ? 'de-DE' : 'en-GB', { weekday: 'short' }).format(new Date(`${date}T12:00:00`));
  }
</script>

<div class="wrap">
  <header class="page-head">
    <div>
      <a class="crumb" href="/" use:link><Icon name="caret-left" size={14} /> {t('nav.overview')}</a>
      <h1>{t('persons.page')}</h1>
      <p class="lead">{t('persons.lead')}</p>
    </div>
  </header>

  <p class="notice info"><Icon name="info" size={17} /> {t('persons.consentHint')}</p>

  <form class="add" onsubmit={(e) => { e.preventDefault(); const n = newName.trim(); if (n) void run(async () => { await api.createPerson(n); newName = ''; }); }}>
    <input class="input" bind:value={newName} maxlength="40" placeholder={t('persons.namePlaceholder')} aria-label={t('persons.name')} />
    <button class="btn primary" type="submit" disabled={busy || !newName.trim()}><Icon name="user-plus" size={16} /> {t('persons.add')}</button>
  </form>

  {#if people === null}
    <div class="skeleton" style:height="120px"></div>
  {:else if people.length === 0}
    <p class="muted">{t('persons.empty')}</p>
  {:else}
    <div class="people">
    {#each people as p (p.id)}
      {@const home = isHome(p, store.devices)}
      <section class="person" class:home aria-labelledby="p-{p.id}">
        <header class="p-head">
          <span class="avatar" aria-hidden="true">{initials(p.name)}</span>
          <div class="p-title">
            {#if renaming === p.id}
              <form class="rename" onsubmit={(e) => { e.preventDefault(); renaming = null; if (renameValue.trim()) void options(p, { name: renameValue.trim() }); }}>
                <input class="input" bind:value={renameValue} maxlength="40" aria-label={t('persons.name')} />
                <button class="btn small primary" type="submit">{t('common.save')}</button>
                <button class="btn small" type="button" onclick={() => (renaming = null)}>{t('common.cancel')}</button>
              </form>
            {:else}
              <h2 id="p-{p.id}">{p.name}
                <button class="btn ghost icon small" type="button" aria-label={t('persons.rename')} onclick={() => { renaming = p.id; renameValue = p.name; }}>
                  <Icon name="pencil-simple" size={14} />
                </button>
              </h2>
            {/if}
            <p class="sub">{presenceText(p, home, clock.now)}</p>
          </div>
          {#if p.devices.some((d) => d.presence)}
            <span class="badge {home ? 'ok' : ''}">{home ? t('persons.badge.home') : t('persons.badge.away')}</span>
          {/if}
        </header>

        <h3 class="eyebrow">{t('persons.devices')}</h3>
        {#if p.devices.length === 0}
          <p class="faint small">{t('persons.noDevices')}</p>
        {:else}
          <ul class="devs">
            {#each p.devices as pd (pd.deviceId)}
              {@const d = store.devices.get(pd.deviceId)}
              {#if d}
                <li>
                  <a href="/devices/{d.id}" use:link class="dname"><DeviceIcon type={d.type} online={d.online} size={28} /> {displayName(d)}</a>
                  <label class="check small">
                    <input type="checkbox" checked={pd.presence} disabled={busy}
                      onchange={(e) => setDevices(p, p.devices.map((x) => (x.deviceId === pd.deviceId ? { ...x, presence: e.currentTarget.checked } : x)))} />
                    {t('persons.presence')}
                  </label>
                  <button class="btn ghost icon small" type="button" aria-label={t('persons.removeDevice', { name: displayName(d) })} disabled={busy}
                    onclick={() => setDevices(p, p.devices.filter((x) => x.deviceId !== pd.deviceId))}>
                    <Icon name="x" size={14} />
                  </button>
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
        <select class="select addsel" aria-label={t('persons.addDevice')} disabled={busy || free.length === 0}
          onchange={(e) => { const v = Number(e.currentTarget.value); e.currentTarget.value = ''; if (v) addDevice(p, v); }}>
          <option value="">{t('persons.addDevice')}</option>
          {#each free as d (d.id)}<option value={d.id}>{displayName(d)}</option>{/each}
        </select>

        <div class="opts">
          {#key resetSwitches}
            <Toggle checked={p.trackTime && p.consentAt > 0} label={t('persons.trackTime')} hint={t('persons.trackTimeHint')} disabled={busy}
              onchange={(v) => switchOn(p, 'time', v)} />
            <Toggle checked={p.homeAssistant} label={t('persons.ha')} hint={t('persons.haHint')} disabled={busy}
              onchange={(v) => switchOn(p, 'ha', v)} />
          {/key}
        </div>

        {#if p.days?.length}
          <ul class="days" aria-label={t('persons.daysLabel')}>
            {#each p.days as d (d.date)}
              <li class="day" title="{dateLong(d.date)}: {duration(d.ms)}">
                <span class="bar" aria-hidden="true"><span style:height="{Math.min(100, (d.ms / 86_400_000) * 100)}%"></span></span>
                <span class="h num" aria-hidden="true">{hours(d.ms)}</span>
                <span class="wd" aria-hidden="true">{weekday(d.date)}</span>
                <span class="sr-only">{dateLong(d.date)}: {duration(d.ms)}</span>
              </li>
            {/each}
          </ul>
          <p class="faint small">{t('persons.daysNote')}</p>
        {/if}

        <button class="btn small ghost danger-text" type="button" onclick={() => { deleteFor = p; deleteOpen = true; }}><Icon name="trash" size={14} /> {t('persons.delete')}</button>
      </section>
    {/each}
    </div>
  {/if}

  <section class="hints">
    <h2>{t('security.hints')}</h2>
    <p class="muted small">{t('persons.hintPrivateMac')}</p>
    <p class="muted small">{t('persons.hintHA')}</p>
  </section>
</div>

<Dialog bind:open={consentOpen} title={t('persons.consentTitle', { name: consentFor?.name ?? '' })}>
  {#if consentFor}
    <p>{t(`persons.consentText.${consentMode}`, { name: consentFor.name })}</p>
    <label class="check"><input type="checkbox" bind:checked={consentOk} /> {t('persons.consentCheck', { name: consentFor.name })}</label>
  {/if}
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (consentOpen = false)}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" disabled={!consentOk || busy}
      onclick={() => { const p = consentFor; consentFor = null; consentOpen = false; if (p) void options(p, { ...(consentMode === 'time' ? { trackTime: true } : { homeAssistant: true }), consent: true }); }}>{t('persons.consentConfirm')}</button>
  {/snippet}
</Dialog>

<Dialog bind:open={deleteOpen} title={t('persons.deleteTitle', { name: deleteFor?.name ?? '' })}>
  <p>{t('persons.deleteText')}</p>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (deleteOpen = false)}>{t('common.cancel')}</button>
    <button class="btn danger" type="button" disabled={busy}
      onclick={() => { const p = deleteFor; deleteOpen = false; if (p) void run(() => api.deletePerson(p.id)); }}>{t('persons.delete')}</button>
  {/snippet}
</Dialog>

<style>
  .small {
    font-size: 13px;
  }
  .add {
    display: flex;
    gap: 10px;
    margin: 16px 0;
  }
  .add .input {
    max-width: 320px;
  }
  /* Two columns that fill up like a newspaper, so cards of different
     height leave no holes. */
  .people {
    columns: 2 440px;
    column-gap: 16px;
  }
  .person {
    break-inside: avoid;
    margin-bottom: 16px;
    display: grid;
    gap: 10px;
    padding: 16px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    justify-items: start;
  }
  .person > .eyebrow {
    margin-top: 6px;
  }
  .p-head {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 12px;
    width: 100%;
  }
  .p-title h2 {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 16px;
  }
  .rename {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .rename .input {
    width: 200px;
  }
  .avatar {
    display: grid;
    place-items: center;
    width: 40px;
    height: 40px;
    border-radius: 50%;
    background: var(--surface-3);
    color: var(--text-2);
    font-weight: 600;
  }
  .home .avatar {
    background: var(--ok-soft);
    color: var(--ok);
  }
  .sub {
    color: var(--text-2);
    font-size: 14px;
  }
  .devs {
    list-style: none;
    margin: 0;
    padding: 0;
    width: 100%;
    display: grid;
  }
  .devs li {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto auto;
    align-items: center;
    gap: 10px;
    padding: 6px 0;
  }
  .devs li + li {
    border-top: 1px solid var(--border);
  }
  .dname {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    color: var(--text);
    font-weight: 500;
    min-width: 0;
  }
  .addsel {
    width: auto;
    min-width: 220px;
  }
  .opts {
    display: grid;
    gap: 6px;
    width: 100%;
  }
  .days {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    gap: 10px;
    align-items: flex-end;
  }
  .day {
    display: grid;
    justify-items: center;
    gap: 2px;
    font-size: 11px;
  }
  .bar {
    display: flex;
    align-items: flex-end;
    width: 22px;
    height: 60px;
    border-radius: 4px;
    overflow: hidden;
    background: var(--surface-2);
  }
  .bar span {
    width: 100%;
    background: var(--signal);
  }
  .wd {
    color: var(--text-3);
  }
  .danger-text {
    color: var(--danger);
    margin-left: -9px;
  }
  .hints {
    margin-top: 0;
    display: grid;
    gap: 6px;
    padding: 16px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  @media (max-width: 560px) {
    .add {
      flex-direction: column;
    }
    .add .input {
      max-width: none;
    }
    .devs li {
      grid-template-columns: minmax(0, 1fr) auto;
    }
    .devs li .check {
      grid-row: 2;
    }
  }
</style>
