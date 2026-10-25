<script lang="ts">
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import Info from '@lucide/svelte/icons/info';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import DeviceIcon from '../lib/components/DeviceIcon.svelte';
  import { toast } from '../lib/components/toast.svelte';
  import { api, ApiError } from '../lib/api';
  import { auth, clock, store } from '../lib/state.svelte';
  import { link } from '../lib/router.svelte';
  import { displayName } from '../lib/devices';
  import { relTime, t } from '../lib/i18n.svelte';
  import type { Finding } from '../lib/types';

  let findings = $state<Finding[]>([]);
  let status = $state<{ running: boolean; lastRunAt: number }>({ running: false, lastRunAt: 0 });
  let loading = $state(true);
  let showAccepted = $state(false);

  async function load() {
    try {
      const r = await api.security();
      findings = r.findings;
      status = r.status;
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void store.overview?.security.running;
    void store.overview?.security.lastRunAt;
    void load();
  });

  async function run() {
    try {
      await api.runSecurity();
      status = { ...status, running: true };
    } catch (e) {
      toast(t(`err.${e instanceof ApiError ? e.code : 'network'}`), 'error');
    }
  }

  async function dismiss(f: Finding, v: boolean) {
    try {
      await api.dismissFinding(f.id, v);
      findings = findings.map((x) => (x.id === f.id ? { ...x, dismissed: v } : x));
    } catch {
      toast(t('common.error'), 'error');
    }
  }

  const running = $derived(status.running || (store.overview?.security.running ?? false));
  const open = $derived(findings.filter((f) => !f.dismissed));
  const accepted = $derived(findings.filter((f) => f.dismissed));
  const unknownCount = $derived([...store.devices.values()].filter((d) => !d.known && !d.isSelf).length);
  const order = { critical: 0, warning: 1, info: 2 } as const;

  function title(f: Finding): string {
    switch (f.check) {
      case 'risky_port':
        return t('security.port.title', { service: (f.data.service ?? '').toUpperCase(), port: f.data.port ?? '' });
      case 'upnp_igd':
        return t('security.upnp.title');
      case 'ip_conflict':
        return t('security.conflict.title', { ip: f.data.ip ?? '' });
      case 'unknown_devices':
        return t('security.unknown.title', { n: unknownCount });
    }
  }
  function why(f: Finding): string {
    if (f.check === 'risky_port') return t(`security.port.why.${f.data.service}`);
    return t({ upnp_igd: 'security.upnp.why', ip_conflict: 'security.conflict.why', unknown_devices: 'security.unknown.why' }[f.check]);
  }
  function todo(f: Finding): string {
    if (f.check === 'risky_port') return t(`security.port.todo.${f.data.service}`);
    return t({ upnp_igd: 'security.upnp.todo', ip_conflict: 'security.conflict.todo', unknown_devices: 'security.unknown.todo' }[f.check]);
  }
</script>

<div class="page">
  <header class="head">
    <div class="title">
      <h1>{t('security.title')}</h1>
      <p class="muted lead">{t('security.lead')}</p>
    </div>
    <div class="run">
      <button class="btn primary" type="button" onclick={run} disabled={running || store.overview?.scanner.mode === 'demo'}>
        <span class:spin={running} class="ico"><RefreshCw size={16} /></span>
        {running ? t('security.running') : t('security.run')}
      </button>
      <span class="faint small">{status.lastRunAt ? t('security.last', { t: relTime(status.lastRunAt, clock.now) }) : ''}</span>
    </div>
  </header>

  {#if !loading && !status.lastRunAt && !running}
    <div class="notice info"><Info size={17} /> {t('security.never')}</div>
  {/if}

  {#if loading}
    <div class="skeleton" style:height="120px"></div>
  {:else if open.length === 0 && status.lastRunAt}
    <div class="card all-good">
      <span class="good-icon"><ShieldCheck size={26} /></span>
      <p>{t('security.allGood')}</p>
    </div>
  {:else}
    <ul class="findings">
      {#each [...open].sort((a, b) => order[a.severity] - order[b.severity]) as f (f.id)}
        {@const dev = f.deviceId ? store.devices.get(f.deviceId) : undefined}
        <li class="card finding {f.severity}">
          <div class="f-head">
            <button class="btn small ghost accept" type="button" onclick={() => dismiss(f, true)}>{t('security.accept')}</button>
            <span class="sev {f.severity}">
              {#if f.severity === 'critical'}<ShieldAlert size={18} />{:else if f.severity === 'warning'}<TriangleAlert size={18} />{:else}<Info size={18} />{/if}
            </span>
            <div class="f-title">
              <span class="badge {f.severity === 'critical' ? 'danger' : f.severity === 'warning' ? 'warn' : ''}">{t(`security.${f.severity}`)}</span>
              <h2>{title(f)}</h2>
              {#if dev}
                <a class="dev" href="/devices/{dev.id}" use:link><DeviceIcon type={dev.type} size={22} /> {displayName(dev)} <span class="mono faint">{dev.ip}</span></a>
              {/if}
            </div>
          </div>
          <details>
            <summary>{t('security.why')} <ChevronDown size={15} /></summary>
            <p>{why(f)}</p>
            <h3>{t('security.todo')}</h3>
            <p>{todo(f)}</p>
          </details>
          {#if f.check === 'unknown_devices'}
            <div class="f-actions">
              <a class="btn small" href="/devices?filter=unreviewed" use:link>{t('security.unknown.open')}</a>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  {#if auth.state && !auth.state.https}
    <section class="card hints">
      <div class="card-head"><h2>{t('security.hints')}</h2></div>
      <div class="card-body"><p class="muted">{t('overview.http')}</p></div>
    </section>
  {/if}

  {#if accepted.length}
    <section>
      <button class="btn ghost small" type="button" onclick={() => (showAccepted = !showAccepted)} aria-expanded={showAccepted}>
        {t('security.acceptedTitle')} ({accepted.length}) <ChevronDown size={15} />
      </button>
      {#if showAccepted}
        <ul class="accepted">
          {#each accepted as f (f.id)}
            <li class="row">
              <span class="muted">{title(f)}</span>
              <span class="spacer"></span>
              <button class="btn small ghost" type="button" onclick={() => dismiss(f, false)}>{t('security.unaccept')}</button>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}
</div>

<style>
  .page {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 16px;
    max-width: 920px;
  }
  .head {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    gap: 16px;
    flex-wrap: wrap;
  }
  .title {
    display: grid;
    gap: 6px;
    max-width: 60ch;
  }
  .run {
    display: grid;
    justify-items: end;
    gap: 6px;
  }
  .ico {
    display: inline-grid;
  }
  .small {
    font-size: 12.5px;
  }
  .all-good {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 18px 20px;
  }
  .good-icon {
    display: grid;
    place-items: center;
    width: 46px;
    height: 46px;
    border-radius: 50%;
    background: var(--ok-soft);
    color: var(--ok);
  }
  .findings {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 10px;
  }
  .finding {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
    padding: 16px 18px;
    border-left: 3px solid var(--border-strong);
  }
  .finding.critical {
    border-left-color: var(--danger);
  }
  .finding.warning {
    border-left-color: var(--warn);
  }
  .f-head {
    position: relative;
    display: flex;
    gap: 12px;
    padding-right: 96px;
  }
  .accept {
    position: absolute;
    top: -4px;
    right: -6px;
    color: var(--text-2);
  }
  .sev {
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    flex: none;
    border-radius: 50%;
    background: var(--surface-2);
    color: var(--text-2);
  }
  .sev.critical {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .sev.warning {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .f-title {
    display: grid;
    gap: 5px;
    justify-items: start;
    min-width: 0;
  }
  .dev {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 8px;
    color: var(--text);
    font-size: 13.5px;
  }
  details {
    margin-left: 46px;
  }
  summary {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    cursor: pointer;
    color: var(--accent);
    font-size: 13.5px;
    list-style: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  details p {
    margin-top: 6px;
    color: var(--text-2);
    max-width: 70ch;
  }
  details h3 {
    margin-top: 12px;
  }
  .f-actions {
    display: flex;
    gap: 8px;
    margin-left: 46px;
  }
  .accepted {
    list-style: none;
    padding: 0;
    margin: 8px 0 0;
    display: grid;
    gap: 4px;
  }
  @media (max-width: 639px) {
    .run {
      justify-items: start;
    }
    .f-head {
      padding-right: 0;
      padding-top: 26px;
    }
    .accept {
      top: -10px;
    }
    details,
    .f-actions {
      margin-left: 0;
    }
  }
</style>
