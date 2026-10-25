<script lang="ts">
  import { errorText } from '../lib/errors';
  import X from '@lucide/svelte/icons/x';
  import Check from '@lucide/svelte/icons/check';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Star from '@lucide/svelte/icons/star';
  import Bell from '@lucide/svelte/icons/bell';
  import Power from '@lucide/svelte/icons/power';
  import Radar from '@lucide/svelte/icons/radar';
  import Activity from '@lucide/svelte/icons/activity';
  import Ellipsis from '@lucide/svelte/icons/ellipsis';
  import Merge from '@lucide/svelte/icons/merge';
  import Trash from '@lucide/svelte/icons/trash-2';
  import DeviceIcon from '../lib/components/DeviceIcon.svelte';
  import CopyButton from '../lib/components/CopyButton.svelte';
  import Timeline from '../lib/components/Timeline.svelte';
  import Dialog from '../lib/components/Dialog.svelte';
  import { toast } from '../lib/components/toast.svelte';
  import { api, ApiError } from '../lib/api';
  import { clock, store } from '../lib/state.svelte';
  import { deviceTypes, displayName, modelName, typeLabel, vendorName } from '../lib/devices';
  import { navigate } from '../lib/router.svelte';
  import { eventText } from '../lib/events';
  import { dateTime, durationSince, relTime, t } from '../lib/i18n.svelte';
  import type { DeviceResponse, EventItem, Presence } from '../lib/types';

  let { id, onclose }: { id: number; onclose: () => void } = $props();

  let detail = $state<DeviceResponse | null>(null);
  let notFound = $state(false);
  let presence = $state<Presence | null>(null);
  let range = $state<'24h' | '7d' | '30d'>('24h');
  let history = $state<EventItem[]>([]);
  let nameDraft = $state('');
  let notes = $state('');
  let location = $state('');
  let menuOpen = $state(false);
  let forgetOpen = $state(false);
  let mergeOpen = $state(false);
  let mergeTarget = $state<number | ''>('');
  let pinging = $state(false);
  let panel: HTMLElement | undefined = $state();
  let heading: HTMLElement | undefined = $state();
  let moreBtn: HTMLButtonElement | undefined = $state();
  let menuEl: HTMLElement | undefined = $state();

  // Keyboard focus: move into the panel when a device opens, and back to its
  // row in the list when the panel closes.
  const opener = document.activeElement as HTMLElement | null;
  let focusedFor = -1;
  $effect(() => {
    if (d && heading && focusedFor !== id) {
      focusedFor = id;
      heading.focus({ preventScroll: true });
    }
  });
  $effect(() => {
    const myId = id;
    return () => {
      const row = document.querySelector<HTMLElement>(`a.main[href^="/devices/${myId}"]`);
      queueMicrotask(() => {
        if (document.activeElement && document.activeElement !== document.body && document.contains(document.activeElement)) return;
        (row ?? (opener && document.contains(opener) ? opener : null))?.focus({ preventScroll: true });
      });
    };
  });

  // Below 1100px the panel covers the page: make it a modal dialog and the
  // rest of the app inert, so keyboard and screen-reader focus stay inside.
  let overlay = $state(window.matchMedia('(max-width: 1100px)').matches);
  $effect(() => {
    const mq = window.matchMedia('(max-width: 1100px)');
    const on = () => (overlay = mq.matches);
    mq.addEventListener('change', on);
    return () => mq.removeEventListener('change', on);
  });
  $effect(() => {
    if (!overlay || !panel) return;
    const changed: HTMLElement[] = [];
    for (let node: HTMLElement | null = panel; node && node !== document.body; node = node.parentElement) {
      for (const sib of node.parentElement?.children ?? []) {
        const el = sib as HTMLElement;
        if (el === node || el.inert || el.classList.contains('backdrop') || el.getAttribute('aria-live')) continue;
        el.inert = true;
        changed.push(el);
      }
    }
    return () => changed.forEach((el) => (el.inert = false));
  });

  // Return focus to the "more" button after a dialog opened from its menu closes.
  let dialogWasOpen = false;
  $effect(() => {
    const anyOpen = forgetOpen || mergeOpen;
    if (dialogWasOpen && !anyOpen) moreBtn?.focus();
    dialogWasOpen = anyOpen;
  });
  $effect(() => {
    if (menuOpen) menuEl?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
  });
  function closeMenu(refocus = true) {
    menuOpen = false;
    if (refocus) moreBtn?.focus();
  }
  function menuKey(e: KeyboardEvent) {
    const items = [...(menuEl?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])];
    const i = items.indexOf(document.activeElement as HTMLElement);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      const next = (i + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
      items[next]?.focus();
    } else if (e.key === 'Tab') {
      closeMenu(false);
    }
  }

  const d = $derived(store.devices.get(id) ?? detail?.device);
  const scanning = $derived(store.portScans[id] ?? false);
  const demo = $derived(store.overview?.scanner.mode === 'demo');

  async function load() {
    notFound = false;
    try {
      const r = await api.device(id);
      detail = r;
      nameDraft = r.device.name;
      notes = r.device.notes;
      location = r.device.location;
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) notFound = true;
    }
  }
  async function loadPresence() {
    try {
      presence = await api.presence(id, range);
    } catch {
      presence = null;
    }
  }
  async function loadHistory() {
    try {
      history = await api.deviceEvents(id);
    } catch {
      history = [];
    }
  }

  $effect(() => {
    void id;
    detail = null;
    presence = null;
    void load();
    void loadHistory();
    panel?.scrollTo({ top: 0 });
  });
  $effect(() => {
    void id;
    void range;
    void loadPresence();
  });
  // Refresh presence and history when the device changes state or new events arrive.
  $effect(() => {
    void d?.online;
    void store.eventTick;
    void loadPresence();
    void loadHistory();
  });
  // Reload ports/addresses after a port scan finishes.
  $effect(() => {
    if (d?.portsScannedAt) void load();
  });

  // Quiet confirmation for edits that save on blur or click.
  let savedFlash = $state(false);
  let flashTimer: ReturnType<typeof setTimeout> | undefined;
  async function patch(p: Parameters<typeof api.patchDevice>[1], okText?: string) {
    try {
      const upd = await api.patchDevice(id, p);
      store.devices.set(upd.id, upd);
      if (okText) toast(okText);
      savedFlash = true;
      clearTimeout(flashTimer);
      flashTimer = setTimeout(() => (savedFlash = false), 2000);
    } catch (e) {
      toast(errorText(e), 'error');
    }
  }

  function saveName() {
    if (!d || nameDraft.trim() === d.name) return;
    void patch({ name: nameDraft.trim() });
  }

  async function ping() {
    pinging = true;
    try {
      const r = await api.ping(id);
      toast(r.ok ? t('device.actions.pingOk', { ms: r.rttMs.toFixed(1) }) : t('device.actions.pingFail'), r.ok ? 'ok' : 'info');
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      pinging = false;
    }
  }

  async function wake() {
    try {
      await api.wake(id);
      toast(t('device.actions.wakeSent'));
    } catch (e) {
      toast(errorText(e), 'error');
    }
  }

  async function scanPorts() {
    store.portScans = { ...store.portScans, [id]: true };
    try {
      const r = await api.portScan(id);
      if (detail) detail.ports = r.ports;
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      store.portScans = { ...store.portScans, [id]: false };
    }
  }

  async function forget() {
    try {
      await api.deleteDevice(id);
      store.devices.delete(id);
      forgetOpen = false;
      onclose();
    } catch (e) {
      toast(errorText(e), 'error');
    }
  }

  async function merge() {
    if (!mergeTarget) return;
    try {
      const into = await api.mergeDevice(id, Number(mergeTarget));
      store.devices.delete(id);
      store.devices.set(into.id, into);
      mergeOpen = false;
      navigate(`/devices/${into.id}`);
    } catch (e) {
      toast(errorText(e), 'error');
    }
  }

  const others = $derived(
    [...store.devices.values()].filter((x) => x.id !== id).sort((a, b) => displayName(a).localeCompare(displayName(b))),
  );

  const sinceText = $derived.by(() => {
    if (!d) return '';
    if (d.online) {
      const open = presence?.spans.find((s) => s.end === 0);
      return open ? t('device.status.onlineSince', { t: durationSince(clock.now - open.start) }) : t('device.status.online');
    }
    return t('device.status.offlineSince', { t: relTime(d.lastSeen, clock.now) });
  });

  const names = $derived(Object.entries(detail?.details.names ?? {}).filter(([, v]) => v));
  const services = $derived(detail?.details.services ?? []);
  const otherAddresses = $derived((detail?.addresses ?? []).filter((a) => a.ip !== d?.ip));

  function onKey(e: KeyboardEvent) {
    if (e.key !== 'Escape' || forgetOpen || mergeOpen) return;
    if (menuOpen) {
      closeMenu(); // Escape closes the innermost layer first
      return;
    }
    if (!(e.target as HTMLElement).closest('input, textarea, select')) onclose();
  }
  function onWindowClick(e: MouseEvent) {
    if (menuOpen && !(e.target as HTMLElement).closest('.menu-wrap')) closeMenu(false);
  }
</script>

<svelte:window onkeydown={onKey} onclick={onWindowClick} />

<div class="backdrop" onclick={onclose} aria-hidden="true"></div>
<aside
  class="detail"
  bind:this={panel}
  aria-label={d ? displayName(d) : ''}
  role={overlay ? 'dialog' : undefined}
  aria-modal={overlay ? 'true' : undefined}
>
  {#if notFound}
    <div class="pad">
      <button class="btn ghost icon small close" type="button" onclick={onclose} aria-label={t('common.close')}><X size={18} /></button>
      <p class="muted">{t('device.notFound')}</p>
    </div>
  {:else if !d}
    <div class="pad stack">
      <div class="skeleton" style:height="56px" style:width="56px" style:border-radius="50%"></div>
      <div class="skeleton" style:height="22px" style:width="60%"></div>
      <div class="skeleton" style:height="120px"></div>
    </div>
  {:else}
    <h2 class="sr-only" tabindex="-1" bind:this={heading}>{displayName(d)}</h2>
    <header class="pad hero">
      <button class="btn ghost icon small close" type="button" onclick={onclose} aria-label={t('common.close')}><X size={18} /></button>
      <DeviceIcon type={d.type} online={d.online} size={56} />
      <div class="ident">
        <div class="name-wrap">
        <input
          class="name-input"
          bind:value={nameDraft}
          placeholder={d.hostname || displayName(d)}
          aria-label={t('device.name.placeholder')}
          maxlength="64"
          onblur={saveName}
          onkeydown={(e) => {
            if (e.key === 'Enter') e.currentTarget.blur();
            if (e.key === 'Escape') {
              nameDraft = d.name;
              e.currentTarget.blur();
            }
          }}
        />
          <Pencil size={15} class="name-pencil" aria-hidden="true" />
        </div>
        <p class="muted sub">
          {[vendorName(d), modelName(d.model)].filter(Boolean).join(' · ') || typeLabel(d.type)}
        </p>
        <div class="status-row">
          <span class="badge" class:ok={d.online}>{sinceText}</span>
          <span class="saved" role="status">{#if savedFlash}<Check size={13} /> {t('common.saved')}{/if}</span>
          {#if d.new}<span class="badge warn">{t('devices.newBadge')}</span>{/if}
          {#if d.isSelf}<span class="badge accent">{t('devices.thisServer')}</span>{/if}
          {#if d.isGateway}<span class="badge">{t('devices.router')}</span>{/if}
          {#if d.proxy}<span class="badge">{t('devices.repeater')}</span>{/if}
          {#if d.viaProxy}<span class="badge">{t('devices.viaRepeater')}</span>{/if}
          {#if d.macKind === 'random'}<span class="badge" title={t('device.macKind.random')}>{t('devices.private')}</span>{/if}
          {#if d.macKind === 'virtual'}<span class="badge">{t('devices.virtual')}</span>{/if}
        </div>
      </div>
    </header>

    <div class="actions pad">
      {#if !d.known}
        <button class="btn primary small" type="button" onclick={() => patch({ known: true })}><Check size={15} /> {t('device.actions.known')}</button>
      {/if}
      <button class="btn small" class:pressed={d.favorite} type="button" aria-pressed={d.favorite} onclick={() => patch({ favorite: !d.favorite })}>
        <Star size={15} fill={d.favorite ? 'currentColor' : 'none'} /> {t('device.actions.favorite')}
      </button>
      <button class="btn small" class:pressed={d.watch} type="button" aria-pressed={d.watch} title={t('device.actions.watchHint')} onclick={() => patch({ watch: !d.watch })}>
        <Bell size={15} /> {t('device.actions.watch')}
      </button>
      {#if !d.online && !d.isSelf}
        <button class="btn small" type="button" onclick={wake} disabled={demo} title={demo ? t('err.demo_mode') : t('device.actions.wakeHint')}><Power size={15} /> {t('device.actions.wake')}</button>
      {/if}
      <button class="btn small" type="button" onclick={ping} disabled={pinging || demo} title={demo ? t('err.demo_mode') : undefined}>
        <span class:spin={pinging} class="ico"><Activity size={15} /></span> {t('device.actions.ping')}
      </button>
      <div class="menu-wrap">
        <button class="btn small icon" type="button" bind:this={moreBtn} aria-haspopup="menu" aria-expanded={menuOpen} aria-label={t('common.more')} onclick={() => (menuOpen = !menuOpen)}>
          <Ellipsis size={16} />
        </button>
        {#if menuOpen}
          <div class="menu" role="menu" tabindex="-1" bind:this={menuEl} onkeydown={menuKey}>
            <button role="menuitem" type="button" onclick={() => { menuOpen = false; mergeOpen = true; }}><Merge size={15} /> {t('device.actions.merge')}</button>
            <button role="menuitem" type="button" class="danger" onclick={() => { menuOpen = false; forgetOpen = true; }}><Trash size={15} /> {t('device.actions.forget')}</button>
          </div>
        {/if}
      </div>
    </div>

    <section class="section">
      <div class="sec-head">
        <h3>{t('device.section.presence')}</h3>
        <div class="seg">
          {#each ['24h', '7d', '30d'] as r (r)}
            <button type="button" aria-pressed={range === r} onclick={() => (range = r as typeof range)}>{r === '24h' ? '24 h' : r === '7d' ? (t('common.days', { n: 7 })) : t('common.days', { n: 30 })}</button>
          {/each}
        </div>
      </div>
      {#if presence}
        <Timeline data={presence} since={d.firstSeen} />
        <p class="muted small">{t('device.presence.online', { pct: Math.round(presence.onlinePct) })}</p>
      {:else}
        <div class="skeleton" style:height="50px"></div>
      {/if}
    </section>

    <section class="section">
      <h3>{t('device.section.network')}</h3>
      <dl class="dl">
        <dt>{t('device.field.ip')}</dt>
        <dd class="mono copy">{d.ip} <CopyButton value={d.ip} /></dd>
        <dt>{t('device.field.mac')}</dt>
        <dd class="mono copy">{d.mac} <CopyButton value={d.mac} /></dd>
        {#if d.macKind !== 'universal'}
          <dt></dt>
          <dd class="hint">{t(`device.macKind.${d.macKind}`)}</dd>
        {/if}
        <dt>{t('device.field.vendor')}</dt>
        <dd title={d.vendorOfficial || (d.vendor ? undefined : t('device.vendorFromModel'))}>{vendorName(d) || t('common.unknown')}</dd>
        {#if d.model}
          <dt>{t('device.field.model')}</dt>
          <dd title={modelName(d.model) !== d.model ? d.model : undefined}>{modelName(d.model)}</dd>
        {/if}
        <dt><label for="dtype">{t('device.field.type')}</label></dt>
        <dd>
          <select id="dtype" class="select small-select" value={d.typeOverride} onchange={(e) => patch({ type: e.currentTarget.value })}>
            <option value="">{t('device.field.typeAuto', { type: typeLabel(d.detectedType) })}</option>
            {#each deviceTypes as ty (ty)}<option value={ty}>{typeLabel(ty)}</option>{/each}
          </select>
        </dd>
        {#if d.os}
          <dt>{t('device.field.os')}</dt>
          <dd>{d.os}</dd>
        {/if}
        {#if d.interface}
          <dt>{t('device.field.interface')}</dt>
          <dd class="mono">{d.interface}</dd>
        {/if}
        <dt>{t('device.field.firstSeen')}</dt>
        <dd>{dateTime(d.firstSeen)}</dd>
        <dt>{t('device.field.lastSeen')}</dt>
        <dd>{d.online ? t('device.status.online') : dateTime(d.lastSeen)}</dd>
        {#if otherAddresses.length}
          <dt>{t('device.field.previousIps')}</dt>
          <dd class="mono">{otherAddresses.map((a) => a.ip).join(', ')}</dd>
        {/if}
      </dl>
    </section>

    <section class="section">
      <h3>{t('device.section.identity')}</h3>
      {#if names.length === 0 && services.length === 0}
        <p class="muted">{t('device.identity.empty')}</p>
      {:else}
        <dl class="dl">
          {#each names as [src, n] (src)}
            <dt>{t(`device.names.source.${src}`)}</dt>
            <dd>{n}</dd>
          {/each}
        </dl>
        {#if services.length}
          <p class="label svc-label">{t('device.identity.services')}</p>
          <div class="chips">
            {#each services as s (s)}<span class="badge mono" title={detail?.details.instances[s] ?? ''}>{s.replace(/\._(tcp|udp)$/, '').replace(/^_/, '')}</span>{/each}
          </div>
        {/if}
      {/if}
    </section>

    <section class="section">
      <div class="sec-head">
        <h3>{t('device.section.ports')}</h3>
        <button class="btn small" type="button" onclick={scanPorts} disabled={scanning || demo} title={demo ? t('err.demo_mode') : t('device.ports.hint')}>
          <span class:spin={scanning} class="ico"><Radar size={15} /></span>
          {scanning ? t('device.ports.scanning') : t('device.actions.ports')}
        </button>
      </div>
      {#if !d.portsScannedAt}
        <p class="muted">{t('device.ports.never')}</p>
      {:else if (detail?.ports ?? []).length === 0}
        <p class="muted">{t('device.ports.none')}</p>
      {:else}
        <ul class="ports">
          {#each detail?.ports ?? [] as p (p.port)}
            <li><span class="mono port">{p.port}</span><span>{p.service || '–'}</span></li>
          {/each}
        </ul>
      {/if}
      {#if d.portsScannedAt}<p class="faint small">{t('device.ports.scanned', { t: relTime(d.portsScannedAt, clock.now) })}</p>{/if}
    </section>

    <section class="section">
      <h3>{t('device.section.notes')}</h3>
      <div class="field">
        <label for="loc">{t('device.field.location')}</label>
        <input id="loc" class="input" bind:value={location} maxlength="64" placeholder={t('device.field.locationPlaceholder')}
          onblur={() => location !== d.location && patch({ location })} />
      </div>
      <div class="field">
        <label for="notes">{t('device.field.notes')}</label>
        <textarea id="notes" class="textarea" bind:value={notes} maxlength="2000" placeholder={t('device.field.notesPlaceholder')}
          onblur={() => notes !== d.notes && patch({ notes })}></textarea>
      </div>
    </section>

    <section class="section last">
      <h3>{t('device.section.history')}</h3>
      {#if history.length === 0}
        <p class="muted">{t('device.history.empty')}</p>
      {:else}
        <ul class="history">
          {#each history.slice(0, 15) as e (e.id)}
            <li>
              <span>{eventText(e, displayName(d))}</span>
              <time class="faint small" datetime={new Date(e.ts).toISOString()} title={dateTime(e.ts)}>{relTime(e.ts, clock.now)}</time>
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <Dialog bind:open={forgetOpen} title={t('device.forget.title', { name: displayName(d) })}>
      <p class="muted">{t('device.forget.body')}</p>
      {#snippet footer()}
        <button class="btn" type="button" onclick={() => (forgetOpen = false)}>{t('common.cancel')}</button>
        <button class="btn danger solid" type="button" onclick={forget}>{t('device.actions.forget')}</button>
      {/snippet}
    </Dialog>

    <Dialog bind:open={mergeOpen} title={t('device.merge.title')}>
      <div class="stack">
        <p class="muted">{t('device.merge.body', { name: displayName(d) })}</p>
        <div class="field">
          <label for="mt">{t('device.merge.target')}</label>
          <select id="mt" class="select" bind:value={mergeTarget}>
            <option value="">–</option>
            {#each others as o (o.id)}<option value={o.id}>{displayName(o)} · {o.ip || o.mac}</option>{/each}
          </select>
        </div>
      </div>
      {#snippet footer()}
        <button class="btn" type="button" onclick={() => (mergeOpen = false)}>{t('common.cancel')}</button>
        <button class="btn primary" type="button" onclick={merge} disabled={!mergeTarget}>{t('device.merge.submit')}</button>
      {/snippet}
    </Dialog>
  {/if}
</aside>

<style>
  .backdrop {
    display: none;
  }
  .detail {
    position: sticky;
    top: 20px;
    align-self: start;
    max-height: calc(100dvh - 40px);
    overflow-y: auto;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-l);
    overscroll-behavior: contain;
  }
  .pad {
    padding: 18px 20px;
  }
  .hero {
    position: relative;
    display: flex;
    gap: 16px;
    align-items: flex-start;
    padding-bottom: 8px;
  }
  .close {
    position: absolute;
    top: 12px;
    right: 12px;
  }
  .ident {
    display: grid;
    gap: 4px;
    min-width: 0;
    flex: 1;
    padding-right: 28px;
  }
  .name-input {
    width: 100%;
    padding: 2px 28px 2px 6px;
    margin-left: -6px;
    border: 1px solid transparent;
    border-radius: 6px;
    background: transparent;
    font-size: 20px;
    font-weight: 620;
    letter-spacing: -0.01em;
  }
  .name-input:hover {
    border-color: var(--border);
  }
  .name-wrap {
    position: relative;
    display: flex;
    align-items: center;
  }
  .name-wrap :global(.name-pencil) {
    position: absolute;
    right: 8px;
    color: var(--text-3);
    pointer-events: none;
  }
  .name-wrap:focus-within :global(.name-pencil) {
    display: none;
  }
  .saved {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 12.5px;
    color: var(--ok);
    min-height: 1em;
  }
  .name-input:focus {
    outline: none;
    border-color: var(--accent);
    background: var(--surface);
  }
  .name-input::placeholder {
    color: var(--text);
    opacity: 1;
  }
  .sub {
    font-size: 13.5px;
  }
  .status-row {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 4px;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    padding-top: 6px;
    padding-bottom: 16px;
    border-bottom: 1px solid var(--border);
  }
  .ico {
    display: inline-grid;
  }
  .menu-wrap {
    position: relative;
  }
  .menu {
    position: absolute;
    right: 0;
    top: calc(100% + 4px);
    z-index: 20;
    min-width: 260px;
    padding: 6px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: var(--shadow-pop);
    display: grid;
  }
  .menu button {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 8px 10px;
    border: 0;
    border-radius: 6px;
    background: none;
    text-align: left;
    cursor: pointer;
  }
  .menu button:hover {
    background: var(--surface-2);
  }
  .menu .danger {
    color: var(--danger);
  }
  .section {
    display: grid;
    gap: 12px;
    padding: 18px 20px;
    border-bottom: 1px solid var(--border);
  }
  .section.last {
    border-bottom: 0;
  }
  .sec-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .small {
    font-size: 12.5px;
  }
  .copy {
    display: flex;
    align-items: center;
    gap: 2px;
    margin: -6px 0;
  }
  .small-select {
    height: 32px;
  }
  .svc-label {
    margin-top: 4px;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .chips .badge {
    font-size: 11.5px;
  }
  .ports,
  .history {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 6px;
  }
  .ports li {
    display: grid;
    grid-template-columns: 64px 1fr;
  }
  .port {
    font-weight: 500;
  }
  .history li {
    display: flex;
    justify-content: space-between;
    gap: 12px;
  }
  @media (max-width: 1100px) {
    .backdrop {
      display: block;
      position: fixed;
      inset: 0;
      z-index: 50;
      background: rgb(10 14 20 / 0.35);
    }
    .detail {
      position: fixed;
      left: auto;
      top: 0;
      right: 0;
      bottom: 0;
      z-index: 51;
      width: min(480px, 100%);
      max-height: none;
      border-radius: 0;
      border-width: 0 0 0 1px;
      box-shadow: var(--shadow-panel);
      animation: slide 200ms var(--ease);
    }
    @keyframes slide {
      from {
        transform: translateX(24px);
        opacity: 0.6;
      }
    }
  }
  @media (max-width: 639px) {
    .detail {
      width: 100%;
      padding-bottom: env(safe-area-inset-bottom);
    }
    .pad,
    .section {
      padding-left: 16px;
      padding-right: 16px;
    }
  }
</style>
