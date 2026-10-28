<script lang="ts">
  import Icon from '../lib/components/Icon.svelte';
  import { errorText } from '../lib/errors';
  import DeviceIcon from '../lib/components/DeviceIcon.svelte';
  import { toast } from '../lib/components/toast.svelte';
  import { api } from '../lib/api';
  import { auth, clock, store } from '../lib/state.svelte';
  import { link } from '../lib/router.svelte';
  import { displayName } from '../lib/devices';
  import { dateLong, has, relTime, t } from '../lib/i18n.svelte';
  import type { DnsResult, Finding, NetworkState } from '../lib/types';

  let findings = $state<Finding[]>([]);
  let status = $state<{ running: boolean; lastRunAt: number }>({ running: false, lastRunAt: 0 });
  let loading = $state(true);
  let showAccepted = $state(false);
  // Router and DHCP server details for the network findings.
  let net = $state<NetworkState>({ gateways: [], dhcpServers: [] });
  let busy = $state(false);

  async function load() {
    try {
      const r = await api.security();
      findings = r.findings;
      status = r.status;
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      loading = false;
    }
    try {
      net = await api.network();
    } catch {
      /* the findings still show; only the router details are missing */
    }
  }

  async function acceptNetwork(f: Finding) {
    busy = true;
    try {
      net = await api.acceptNetwork(f.check === 'gateway_mac' ? { ip: f.data.ip ?? '' } : { server: f.data.server ?? '' });
      await load();
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      busy = false;
    }
  }
  async function resolveIdentity(f: Finding, setUpAnew: boolean) {
    if (!f.deviceId) return;
    busy = true;
    try {
      await api.resolveIdentity(f.deviceId, setUpAnew);
      await load();
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      busy = false;
    }
  }
  const gateway = (f: Finding) => net.gateways.find((g) => g.ip === f.data.ip);
  const dnsOf = (f: Finding) => net.dns?.results.find((r) => r.server === f.data.server);

  // The LAN attack detection overview (docs/10, F2): one line per trick.
  const arpState = $derived.by(() => {
    const g = net.gateways[0];
    if (!g) return { tone: '', text: t('security.ids.arp.none') };
    const bad = net.gateways.find((x) => x.conflict);
    if (bad) return { tone: 'danger', text: t('security.ids.arp.conflict', { ip: bad.ip }) };
    const moved = net.gateways.find((x) => x.change);
    if (moved) return { tone: 'warn', text: t('security.ids.arp.changed', { ip: moved.ip }) };
    return { tone: 'ok', text: t('security.ids.arp.ok', { ip: g.ip, vendor: g.vendor || t('security.net.unknownVendor'), d: dateLong(new Date(g.since).toLocaleDateString('sv')) }) };
  });
  const dhcpTone = $derived(
    net.dhcpServers.length === 0 ? '' : findings.some((f) => !f.dismissed && f.check === 'dhcp_rogue' && f.severity !== 'info') ? 'warn' : 'ok',
  );
  const dnsTone = $derived.by(() => {
    const rs = net.dns?.results ?? [];
    if ((store.settings && !store.settings.dnsCheck) || rs.length === 0) return '';
    if (rs.some((r) => r.verdict === 'manipulated')) return 'danger';
    if (rs.some((r) => r.verdict === 'redirects')) return 'warn';
    return rs.every((r) => r.verdict === 'unreachable') ? '' : 'ok';
  });
  const svcName = (s: string) => (has(`tripwire.svc.${s}`) ? t(`tripwire.svc.${s}`) : s);
  const tripTone = $derived.by(() => {
    const tw = net.tripwire;
    if (!tw?.enabled) return '';
    if (findings.some((f) => f.check === 'tripwire' && !f.dismissed)) return 'danger';
    if (tw.ports.some((p) => p.state !== 'open') || tw.ports.length === 0) return 'warn';
    return 'ok';
  });
  function dnsLine(r: DnsResult): string {
    switch (r.verdict) {
      case 'manipulated': {
        const a = r.answers.find((x) => x.status === 'wrong')!;
        return t('security.ids.dns.manipulated', { server: r.server, name: a.name, got: (a.got ?? []).join(', ') });
      }
      case 'redirects':
        return t('security.ids.dns.redirects', { server: r.server, got: (r.missing.got ?? []).join(', ') });
      case 'unreachable':
        return t('security.ids.dns.unreachable', { server: r.server });
    }
    const blocked = r.answers.filter((x) => x.status === 'blocked').map((x) => x.name);
    return blocked.length ? t('security.ids.dns.blocked', { server: r.server, names: blocked.join(', ') }) : t('security.ids.dns.ok', { server: r.server });
  }
  const server = (f: Finding) => net.dhcpServers.find((d) => d.serverId === f.data.server);

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

  const demo = $derived(store.overview?.scanner.mode === 'demo');
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
      case 'gateway_mac':
        return t(`security.gateway.${f.data.kind}.title`, { ip: f.data.ip ?? '' });
      case 'identity_changed': {
        const dev = f.deviceId ? store.devices.get(f.deviceId) : undefined;
        return t('security.identity.title', { name: dev ? displayName(dev) : t('common.unknown'), to: t(`os.family.${f.data.to}`) });
      }
      case 'known_exploited': {
        const name = (f.deviceId ? store.devices.get(f.deviceId)?.software?.name : '') || t('common.unknown');
        return t(`security.exploited.title${f.data.manual ? '.manual' : f.data.uncertain ? '.uncertain' : ''}`, { name });
      }
      case 'dhcp_rogue':
        return t(f.severity === 'info' ? 'security.dhcp.titleAsk' : 'security.dhcp.title', { server: f.data.server ?? '' });
      case 'dns_manipulated':
        return t('security.dns.manipulated.title', { server: f.data.server ?? '' });
      case 'dns_redirect':
        return t('security.dns.redirect.title', { server: f.data.server ?? '' });
      case 'tripwire': {
        if (f.data.many) return t('security.tripwire.many');
        const dev = f.deviceId ? store.devices.get(f.deviceId) : undefined;
        return t('security.tripwire.title', { name: dev ? displayName(dev) : f.data.ip ?? t('common.unknown') });
      }
      case 'software_outdated':
      case 'software_update': {
        const name = (f.deviceId ? store.devices.get(f.deviceId)?.software?.name : '') || t('common.unknown');
        return t(`security.software.title.${f.data.status}`, { name });
      }
    }
  }
  // Software findings explain themselves on the device page; here a short version.
  function softwareAdvice(f: Finding): string {
    const product = f.deviceId ? store.devices.get(f.deviceId)?.software?.product : '';
    for (const key of [`software.advice.${product}.${f.data.status}`, `software.advice.${f.data.status}`]) {
      if (has(key)) return t(key);
    }
    return '';
  }
  function why(f: Finding): string {
    if (f.check === 'software_outdated' || f.check === 'software_update') return t(`security.software.why.${f.data.status}`);
    if (f.check === 'risky_port') return t(`security.port.why.${f.data.service}`);
    if (f.check === 'gateway_mac') return t(`security.gateway.${f.data.kind}.why`);
    if (f.check === 'identity_changed') return t('security.identity.why', { from: t(`os.family.${f.data.from}`), to: t(`os.family.${f.data.to}`) });
    if (f.check === 'known_exploited') return t('security.exploited.why') + (f.data.manual ? ' ' + t('security.exploited.whyManual') : '');
    if (f.check === 'dhcp_rogue') return t('security.dhcp.why') + (f.severity === 'critical' ? ' ' + t('security.dhcp.whyOther') : '');
    if (f.check === 'dns_manipulated') return t('security.dns.manipulated.why');
    if (f.check === 'dns_redirect') return t('security.dns.redirect.why');
    if (f.check === 'tripwire') return t('security.tripwire.why');
    return t({ upnp_igd: 'security.upnp.why', ip_conflict: 'security.conflict.why', unknown_devices: 'security.unknown.why' }[f.check]);
  }
  function todo(f: Finding): string {
    if (f.check === 'software_outdated' || f.check === 'software_update') return softwareAdvice(f);
    if (f.check === 'risky_port') return t(`security.port.todo.${f.data.service}`);
    if (f.check === 'gateway_mac') return t(`security.gateway.${f.data.kind}.todo`);
    if (f.check === 'identity_changed') return t('security.identity.todo');
    if (f.check === 'known_exploited') return t(f.data.uncertain ? 'security.exploited.todo.uncertain' : 'security.exploited.todo', { fixed: f.data.fixed ?? '' });
    if (f.check === 'dhcp_rogue') return t('security.dhcp.todo');
    if (f.check === 'dns_manipulated') return t('security.dns.manipulated.todo');
    if (f.check === 'dns_redirect') return t('security.dns.redirect.todo');
    if (f.check === 'tripwire') return t('security.tripwire.todo');
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
    <p class="tally">
      {#each ['critical', 'warning', 'info'] as sev (sev)}
        {@const n = open.filter((f) => f.severity === sev).length}
        <span class={sev} class:zero={n === 0}><strong class="num">{n}</strong> {t(`security.count.${sev}Word`, { n })}</span>
      {/each}
    </p>
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
          <div class="f-body">
            <h2><span class="sev">{t(`security.${f.severity}`)}</span> {title(f)}</h2>
            {#if dev}
              <a class="dev" href="/devices/{dev.id}" use:link><DeviceIcon type={dev.type} size={24} /> {displayName(dev)} <span class="mono faint">{dev.ip}</span></a>
            {/if}
            <details open={f.severity === 'critical'}>
              <summary class="custom">{t('security.why')} <Icon name="caret-down" size={14} /></summary>
              <p>{why(f)}</p>
              <h3>{t('security.todo')}</h3>
              <p>{todo(f)}</p>
            </details>
            {#if f.check === 'gateway_mac'}
              {@const g = gateway(f)}
              {#if g}
                <dl class="net">
                  {#if g.mac}
                    <div><dt>{g.change ? t('security.gateway.before') : t('security.gateway.yours')}</dt><dd>{g.vendor || t('security.net.unknownVendor')} <span class="mono faint">{g.mac}</span></dd></div>
                  {/if}
                  {#if g.conflict && g.others?.length}
                    <div><dt>{t('security.gateway.others')}</dt><dd class="mono">{g.others.join(', ')}</dd></div>
                  {/if}
                  {#if g.change}
                    <div><dt>{t('security.gateway.now')}</dt><dd>{g.change.vendor || t('security.net.unknownVendor')} <span class="mono faint">{g.change.mac}</span></dd></div>
                  {/if}
                </dl>
                {#if g.change?.oldActive}<p class="notice warn small">{t('security.gateway.oldActive')}</p>{/if}
                {#if g.change}
                  <button class="btn small" type="button" disabled={busy || demo} title={demo ? t('err.demo_mode') : undefined} onclick={() => acceptNetwork(f)}>
                    <Icon name="check" size={14} /> {t('security.gateway.accept')}
                  </button>
                {/if}
              {/if}
            {:else if f.check === 'dhcp_rogue'}
              {@const d = server(f)}
              {#if d}
                <dl class="net">
                  <div><dt>{t('security.net.device')}</dt><dd>{d.vendor || t('security.net.unknownVendor')} <span class="mono faint">{d.mac}</span></dd></div>
                  {#if d.router || d.dns?.length}
                    <div><dt>{t('security.dhcp.gives')}</dt><dd>{t('security.dhcp.announces', { router: d.router || '–', dns: d.dns?.join(', ') || '–' })}</dd></div>
                  {/if}
                </dl>
              {/if}
              <button class="btn small" type="button" disabled={busy || demo} title={demo ? t('err.demo_mode') : undefined} onclick={() => acceptNetwork(f)}>
                <Icon name="check" size={14} /> {t('security.dhcp.accept')}
              </button>
            {:else if f.check === 'dns_manipulated' || f.check === 'dns_redirect'}
              {@const r = dnsOf(f)}
              {#if r}<p class="dns-detail">{dnsLine(r)}</p>{/if}
            {:else if f.check === 'tripwire'}
              <p class="faint small">{t('security.tripwire.last', { t: relTime(f.lastSeen, clock.now) })}</p>
            {:else if f.check === 'identity_changed'}
              <div class="row-btns">
                <button class="btn small" type="button" disabled={busy || demo} title={demo ? t('err.demo_mode') : undefined} onclick={() => resolveIdentity(f, true)}>
                  <Icon name="check" size={14} /> {t('security.identity.setUp')}
                </button>
                <button class="btn small" type="button" disabled={busy || demo} title={demo ? t('err.demo_mode') : undefined} onclick={() => resolveIdentity(f, false)}>
                  <Icon name="question" size={14} /> {t('security.identity.unknown')}
                </button>
              </div>
            {:else if f.check === 'unknown_devices'}
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

  {#if !loading}
    <section class="ids" aria-labelledby="ids-title">
      <h2 id="ids-title">{t('security.ids.title')}</h2>
      <p class="hint">{t('security.ids.lead')}</p>
      <ul class="ids-list">
        <li>
          <span class="dot {arpState.tone}" aria-hidden="true"></span>
          <div>
            <h3>{t('security.ids.arp')}</h3>
            <p>{arpState.text}</p>
          </div>
        </li>
        <li>
          <span class="dot {dhcpTone}" aria-hidden="true"></span>
          <div>
            <h3>{t('security.ids.dhcp')}</h3>
            <p>
              {#if net.dhcpServers.length === 0}{t('security.ids.dhcp.none')}
              {:else if net.dhcpServers.length === 1}{t('security.ids.dhcp.one', { server: net.dhcpServers[0].serverId })}
              {:else}{t('security.ids.dhcp.many', { n: net.dhcpServers.length })}{/if}
            </p>
            {#if net.dhcpServers.length}
              <dl class="net">
                {#each net.dhcpServers as d (d.serverId)}
                  <div>
                    <dt>{t('security.net.dhcp', { ip: d.serverId })}</dt>
                    <dd>{d.vendor || t('security.net.unknownVendor')} <span class="mono faint">{d.mac}</span>{#if d.accepted}{' '}<span class="faint">· {t('security.net.accepted')}</span>{/if}</dd>
                  </div>
                {/each}
              </dl>
            {/if}
            <p class="faint small">
              {#if store.settings && !store.settings.dhcpCheck}{t('security.net.probeOff')}
              {:else if net.probe}{t(`security.net.probe.${net.probe.answers > 1 ? 'n' : net.probe.answers}`, { t: relTime(net.probe.at, clock.now), n: net.probe.answers })}
              {:else}{t('security.net.probeNever')}{/if}
            </p>
          </div>
        </li>
        <li>
          <span class="dot {dnsTone}" aria-hidden="true"></span>
          <div>
            <h3>{t('security.ids.dns')}</h3>
            {#if store.settings && !store.settings.dnsCheck}
              <p>{t('security.ids.dns.off')}</p>
            {:else if !net.dns}
              <p>{t('security.ids.dns.never')}</p>
            {:else if net.dns.results.length === 0}
              <p>{t('security.ids.dns.empty')}</p>
            {:else}
              {#each net.dns.results as r (r.server)}<p>{dnsLine(r)}</p>{/each}
              <p class="faint small">{t('security.ids.checked', { t: relTime(net.dns.at, clock.now) })}</p>
            {/if}
          </div>
        </li>
        {#if net.tripwire}
          {@const tw = net.tripwire}
          {@const live = tw.ports.filter((p) => p.state === 'open')}
          {@const ports = live.map((p) => p.port).join(', ')}
          <li>
            <span class="dot {tripTone}" aria-hidden="true"></span>
            <div>
              <h3>{t('security.ids.tripwire')}</h3>
              {#if !tw.enabled}
                <p>{t('security.ids.tripwire.off')}</p>
              {:else if live.length === 0}
                <p>{t('security.ids.tripwire.none')}</p>
              {:else if tw.lastHit}
                <p>{t('security.ids.tripwire.hit', { ports, t: relTime(tw.lastHit.at, clock.now), ip: tw.lastHit.ip, service: svcName(tw.lastHit.service) })}</p>
              {:else}
                <p>{t('security.ids.tripwire.quiet', { ports })}</p>
              {/if}
              {#each tw.ports.filter((p) => p.state !== 'open') as p (p.port)}
                <p class="faint small">Port {p.port}: {t(`tripwire.state.${p.state}`)}</p>
              {/each}
            </div>
          </li>
        {/if}
      </ul>
    </section>
  {/if}

  {#if auth.state && !auth.state.https}
    <section class="hints">
      <h2>{t('security.hints')}</h2>
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
    flex-wrap: wrap;
    gap: 4px 20px;
    margin: 0 0 14px;
    color: var(--text-2);
  }
  .tally strong {
    font-size: 18px;
    font-weight: 600;
    margin-right: 2px;
    color: var(--text);
  }
  .tally .critical strong {
    color: var(--danger);
  }
  .tally .warning strong {
    color: var(--warn);
  }
  .tally .zero strong {
    color: var(--text-2);
  }
  .all-good {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 20px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--ok);
    font-size: 15px;
    font-weight: 500;
  }
  .findings {
    list-style: none;
    margin: 0;
    padding: 0;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .finding {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 16px;
    padding: 16px;
  }
  .finding + .finding {
    border-top: 1px solid var(--border);
  }
  .sev {
    display: inline-flex;
    align-items: center;
    height: 20px;
    margin-right: 6px;
    padding: 0 7px;
    vertical-align: 2px;
    border-radius: 5px;
    background: var(--surface-2);
    font-size: 12px;
    font-weight: 500;
    color: var(--text-2);
  }
  .critical .sev {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .warning .sev {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .f-body {
    display: grid;
    gap: 8px;
    justify-items: start;
    min-width: 0;
  }
  .f-body h2 {
    font-size: 15px;
    line-height: 1.5;
  }
  .dev {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    color: var(--text);
    font-weight: 500;
  }
  details {
    width: 100%;
    max-width: 70ch;
    color: var(--text-2);
  }
  summary {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 0;
    cursor: pointer;
    list-style: none;
    font-weight: 500;
    color: var(--text);
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
    color: var(--text-2);
  }
  .net {
    display: grid;
    gap: 6px;
    margin: 0;
    font-size: 14px;
  }
  .row-btns {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .ids,
  .hints {
    margin-top: 16px;
    display: grid;
    gap: 10px;
    padding: 16px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .ids-list {
    list-style: none;
    margin: 4px 0 0;
    padding: 0;
  }
  .ids-list > li {
    display: grid;
    grid-template-columns: 8px minmax(0, 1fr);
    gap: 12px;
    padding: 12px 0;
  }
  .ids-list > li + li {
    border-top: 1px solid var(--border);
  }
  .ids-list > li > .dot {
    margin-top: 6px;
  }
  .ids-list > li > div {
    display: grid;
    gap: 4px;
    min-width: 0;
  }
  .ids-list h3 {
    font-size: 14px;
  }
  .ids .net div {
    grid-template-columns: 220px minmax(0, 1fr);
  }
  .dns-detail {
    font-size: 14px;
  }
  .net div {
    display: grid;
    grid-template-columns: 110px minmax(0, 1fr);
    gap: 10px;
  }
  .net dt {
    color: var(--text-2);
    font-size: 13px;
  }
  .net dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .hidden-findings {
    margin-top: 16px;
  }
  .accepted {
    list-style: none;
    margin: 8px 0 0;
    padding: 0 16px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .accepted li {
    padding: 8px 0;
  }
  .accepted li + li {
    border-top: 1px solid var(--border);
  }
  @media (max-width: 640px) {
    .ids .net div {
      grid-template-columns: minmax(0, 1fr);
      gap: 0;
    }
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
  }
</style>
