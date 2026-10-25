<script lang="ts">
  import Icon from '../lib/components/Icon.svelte';
  import { errorText } from '../lib/errors';
  import DeviceIcon from '../lib/components/DeviceIcon.svelte';
  import { toast } from '../lib/components/toast.svelte';
  import { api } from '../lib/api';
  import { auth, clock, store } from '../lib/state.svelte';
  import { link } from '../lib/router.svelte';
  import { displayName } from '../lib/devices';
  import { has, relTime, t } from '../lib/i18n.svelte';
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
      toast(errorText(e), 'error');
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
        return t('security.port.title', {
          service: has(`security.port.name.${f.data.service}`) ? t(`security.port.name.${f.data.service}`) : (f.data.service ?? '').toUpperCase(),
          port: f.data.port ?? '',
        });
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

<div class="wrap">
  <header class="page-head">
    <div>
      <h1>{t('security.title')}</h1>
      <p class="lead">{t('security.lead')}</p>
    </div>
    <div class="run">
      <button class="btn primary" type="button" onclick={run} disabled={running || store.overview?.scanner.mode === 'demo'}>
        <span class:spin={running} class="ico"><Icon name="arrows-clockwise" size={16} /></span>
        {running ? t('security.running') : t('security.run')}
      </button>
      <span class="faint small">
        {store.overview?.scanner.mode === 'demo' ? t('err.demo_mode') : status.lastRunAt ? t('security.last', { t: relTime(status.lastRunAt, clock.now) }) : ''}
      </span>
    </div>
  </header>

  {#if !loading && !status.lastRunAt && !running}
    <div class="notice info"><Icon name="info" size={17} /> {t('security.never')}</div>
  {/if}

  {#if !loading && open.length}
    <dl class="tally">
      {#each ['critical', 'warning', 'info'] as sev (sev)}
        {@const n = open.filter((f) => f.severity === sev).length}
        <div class={sev} class:zero={n === 0}><dt>{t(`security.${sev}`)}</dt><dd class="num">{n}</dd></div>
      {/each}
    </dl>
  {/if}

  {#if loading}
    <div class="skeleton" style:height="120px"></div>
  {:else if open.length === 0 && status.lastRunAt}
    <div class="all-good">
      <Icon name="shield-check" size={28} />
      <p>{t('security.allGood')}</p>
    </div>
  {:else}
    <ol class="findings">
      {#each [...open].sort((a, b) => order[a.severity] - order[b.severity]) as f (f.id)}
        {@const dev = f.deviceId ? store.devices.get(f.deviceId) : undefined}
        <li class="finding {f.severity}">
          <span class="sev">{t(`security.${f.severity}`)}</span>
          <div class="f-body">
            <h2>{title(f)}</h2>
            {#if dev}
              <a class="dev" href="/devices/{dev.id}" use:link><DeviceIcon type={dev.type} size={24} /> {displayName(dev)} <span class="mono faint">{dev.ip}</span></a>
            {/if}
            <details open={f.severity === 'critical'}>
              <summary class="custom">{t('security.why')} <Icon name="caret-down" size={14} /></summary>
              <p>{why(f)}</p>
              <h3>{t('security.todo')}</h3>
              <p>{todo(f)}</p>
            </details>
            {#if f.check === 'unknown_devices'}
              <a class="btn small" href="/devices?filter=unreviewed" use:link>{t('security.unknown.open')}</a>
            {/if}
          </div>
          <button class="btn small ghost accept" type="button" title={t('security.acceptHint')} onclick={() => dismiss(f, true)}>
            <Icon name="eye-slash" size={14} /> {t('security.accept')}
          </button>
        </li>
      {/each}
    </ol>
  {/if}

  {#if auth.state && !auth.state.https}
    <section class="hints">
      <h2 class="eyebrow">{t('security.hints')}</h2>
      <p class="muted">{t('overview.http')}</p>
    </section>
  {/if}

  {#if accepted.length}
    <section class="hidden-findings">
      <button class="btn ghost small" type="button" onclick={() => (showAccepted = !showAccepted)} aria-expanded={showAccepted}>
        {t('security.acceptedTitle')} ({accepted.length}) <Icon name="caret-down" size={14} />
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
  .wrap > :global(*) {
    max-width: 1000px;
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
    font-size: 13px;
  }
  .tally {
    display: flex;
    gap: 0;
    margin: 0 0 22px;
  }
  .tally div {
    padding: 0 32px 0 0;
    margin-right: 32px;
    border-right: 1px solid var(--border);
  }
  .tally div:last-child {
    border-right: 0;
  }
  .tally dt {
    font-family: var(--mono);
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-3);
  }
  .tally dd {
    margin: 0;
    font-size: 40px;
    font-weight: 800;
    letter-spacing: -0.04em;
    line-height: 1.1;
  }
  .tally .critical dd {
    color: var(--danger);
  }
  .tally .warning dd {
    color: var(--warn);
  }
  .tally .zero dd {
    color: var(--text-3);
  }
  .all-good {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 22px 0;
    color: var(--ok);
    font-size: 16px;
    font-weight: 600;
  }
  .findings {
    list-style: none;
    margin: 0;
    padding: 0;
    border-top: 1px solid var(--text);
  }
  .finding {
    display: grid;
    grid-template-columns: 110px minmax(0, 1fr) auto;
    gap: 18px;
    padding: 18px 0;
    border-bottom: 1px solid var(--border);
  }
  .sev {
    align-self: start;
    padding: 3px 0 3px 10px;
    border-left: 3px solid var(--text-3);
    font-family: var(--mono);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-2);
  }
  .critical .sev {
    border-left-color: var(--danger);
    color: var(--danger);
  }
  .warning .sev {
    border-left-color: var(--amber);
    color: var(--warn);
  }
  .f-body {
    display: grid;
    gap: 8px;
    justify-items: start;
    min-width: 0;
  }
  .f-body h2 {
    font-size: 16px;
  }
  .dev {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    color: var(--text);
    font-weight: 600;
  }
  details {
    width: 100%;
    max-width: 66ch;
    color: var(--text-2);
  }
  summary {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 4px 0;
    cursor: pointer;
    list-style: none;
    font-weight: 600;
    color: var(--link);
  }
  summary::-webkit-details-marker {
    display: none;
  }
  details[open] summary :global(svg) {
    transform: rotate(180deg);
  }
  details p {
    margin: 6px 0 0;
  }
  details h3 {
    margin-top: 12px;
    color: var(--text);
  }
  .accept {
    align-self: start;
  }
  .hints {
    margin-top: 32px;
    display: grid;
    gap: 8px;
  }
  .hidden-findings {
    margin-top: 26px;
  }
  .accepted {
    list-style: none;
    margin: 8px 0 0;
    padding: 0;
  }
  .accepted li {
    padding: 8px 0;
    border-bottom: 1px solid var(--border);
  }
  @media (max-width: 640px) {
    .run {
      justify-items: start;
    }
    .finding {
      grid-template-columns: minmax(0, 1fr);
      gap: 10px;
    }
    .accept {
      justify-self: start;
      padding-left: 0;
    }
    .tally div {
      padding-right: 18px;
      margin-right: 18px;
    }
    .tally dd {
      font-size: 32px;
    }
  }
</style>
