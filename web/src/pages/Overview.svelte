<script lang="ts">
  import Icon from '../lib/components/Icon.svelte';
  import DeviceIcon from '../lib/components/DeviceIcon.svelte';
  import NetRing from '../lib/components/NetRing.svelte';
  import ScanButton from '../lib/components/ScanButton.svelte';
  import Sparkline from '../lib/components/Sparkline.svelte';
  import Dialog from '../lib/components/Dialog.svelte';
  import { toast } from '../lib/components/toast.svelte';
  import { api } from '../lib/api';
  import { auth, clock, store } from '../lib/state.svelte';
  import { link } from '../lib/router.svelte';
  import { category, deviceTypes, displayName, subtitle, typeLabel, type Category } from '../lib/devices';
  import { eventText, overviewKinds } from '../lib/events';
  import type { EventItem, PersonStatus } from '../lib/types';
  import { initials, isHome, presenceText } from '../lib/persons';
  import { number, relTime, shortDay, t, time } from '../lib/i18n.svelte';
  import type { DeviceType } from '../lib/types';

  const devices = $derived([...store.devices.values()]);
  const online = $derived(devices.filter((d) => d.online).length);
  const fresh = $derived(devices.filter((d) => d.new).sort((a, b) => b.firstSeen - a.firstSeen));
  // Devices whose software needs attention, the worst first.
  const swRank: Record<string, number> = { outdated: 0, update: 1, aging: 2 };
  const swList = $derived(
    devices.filter((d) => d.software && d.software.status in swRank)
      .sort((a, b) => swRank[a.software!.status] - swRank[b.software!.status] || displayName(a).localeCompare(displayName(b))),
  );
  const swCounts = $derived.by(() => {
    const c = { outdated: 0, update: 0, aging: 0, supported: 0, unknown: 0 };
    for (const d of devices) c[d.software?.status ?? 'unknown']++;
    return c;
  });
  const ov = $derived(store.overview);
  const scanner = $derived(ov?.scanner);
  const active = $derived(scanner?.interfaces?.filter((i) => i.active) ?? []);

  // Device mix: one stacked bar by category, the types listed beneath.
  const order: Category[] = ['net', 'compute', 'mobile', 'media', 'home', 'peripheral', 'unknown'];
  const mix = $derived.by(() => {
    const types = new Map<DeviceType, number>();
    for (const d of devices) types.set(d.type, (types.get(d.type) ?? 0) + 1);
    return order
      .map((cat) => {
        const items = deviceTypes.filter((ty) => category(ty) === cat && types.has(ty)).map((ty) => ({ type: ty, count: types.get(ty)! }));
        return { cat, items, count: items.reduce((n, i) => n + i.count, 0) };
      })
      .filter((g) => g.count > 0);
  });

  let history = $state<{ t: number; ms: number }[]>([]);
  async function loadHistory() {
    try {
      history = (await api.internet()).history.slice(-240);
    } catch {
      /* not critical */
    }
  }
  $effect(() => {
    void loadHistory();
    const id = setInterval(loadHistory, 60_000);
    return () => clearInterval(id);
  });

  async function markKnown(id: number) {
    try {
      await api.patchDevice(id, { known: true });
    } catch {
      toast(t('common.error'), 'error');
    }
  }
  let confirmAll = $state(false);
  async function markAll() {
    confirmAll = false;
    try {
      const r = await api.acknowledgeAll();
      toast(t('settings.ackDone', { n: r.count }));
    } catch {
      toast(t('common.error'), 'error');
    }
  }

  // Alerts that silently go nowhere are worse than none: say so.
  let noChannel = $state(false);
  $effect(() => {
    if (!store.settings?.notifyNewDevice || scanner?.mode === 'demo') return;
    api.channels().then((r) => (noChannel = !r.channels.some((c) => c.enabled)), () => {});
  });

  // The overview shows what happened on the network; sign-ins and settings
  // changes stay in the activity log. Reloaded when events arrive; a burst
  // of events costs one request, and only the newest answer is used.
  let feed = $state<EventItem[]>([]);
  let feedLoaded = $state(false);
  let feedSeq = 0;
  $effect(() => {
    void store.eventTick;
    const n = ++feedSeq;
    const timer = setTimeout(
      () =>
        api.events({ kinds: overviewKinds, limit: 8 }).then(
          (r) => {
            if (n !== feedSeq) return;
            feed = r;
            feedLoaded = true;
          },
          () => {
            if (n === feedSeq) feedLoaded = true;
          },
        ),
      n === 1 ? 0 : 300,
    );
    return () => clearTimeout(timer);
  });

  const inetState = $derived(ov?.internet.state ?? 'unknown');
  const inetOk = $derived(inetState === 'online' || inetState === 'degraded');
  const deviceName = (id?: number) => (id ? store.devices.get(id) : undefined);
  // The ring is drawn for the space it gets (labels need ~60 px on each side).
  let ringBox = $state(0);
  const ringLabels = $derived(ringBox >= 400);
  const ringSize = $derived(Math.min(420, ringBox - (ringLabels ? 150 : 24)));

  // Who is at home (docs/09, F4). The live state comes from the device list,
  // "since" from the server; it is reloaded when someone comes or goes.
  let people = $state<PersonStatus[] | null>(null);
  async function loadPeople() {
    try {
      people = await api.persons();
    } catch {
      people = [];
    }
  }
  const homeKey = $derived((people ?? []).map((p) => (isHome(p, store.devices) ? 1 : 0)).join(''));
  $effect(() => {
    void homeKey;
    void loadPeople();
  });
</script>

<Dialog bind:open={confirmAll} title={t('overview.markAllTitle', { n: fresh.length })}>
  <p>{t('overview.markAllText')}</p>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (confirmAll = false)}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" onclick={markAll}>{t('overview.markAllKnown')}</button>
  {/snippet}
</Dialog>

<section class="stage" aria-labelledby="net-title">
  <div class="stage-inner">
    <div class="stage-text">
      <p class="where">
        {#each active as i (i.name)}<span>{i.name}</span>{/each}
        {#if scanner?.gateways?.length}<span>{t('overview.gateway')} <span class="mono">{scanner.gateways[0]}</span></span>{/if}
      </p>

      <h1 id="net-title" class="headline">
        <a href="/devices?filter=online" use:link>
          <span class="hl-num num">{store.loading ? '–' : number(online)}</span>
          {t('overview.headline', { n: devices.length })}
        </a>
      </h1>

      <dl class="facts">
        <a class="fact" href="/devices?filter=new" use:link>
          <dt>{t('overview.new')}</dt>
          <dd class="num" class:amber={fresh.length > 0}>{fresh.length}</dd>
        </a>
        <a class="fact" href="/devices?filter=offline" use:link>
          <dt>{t('overview.offline')}</dt>
          <dd class="num">{devices.length - online}</dd>
        </a>
        <div class="fact inet">
          <dt>{t('overview.internet')}</dt>
          <dd>
            <span class="inet-state" class:ok={inetOk} class:bad={inetState === 'offline' || inetState === 'lan_down'}>
              {t(`internet.${inetState}`)}
            </span>
            {#if inetOk && ov?.internet.latencyMs !== undefined && ov.internet.latencyMs >= 0}
              <span class="lat num">{ov.internet.latencyMs} ms</span>
            {/if}
          </dd>
          {#if history.length > 1}<span class="spark"><Sparkline points={history} height={20} /></span>{/if}
        </div>
      </dl>

      <div class="stage-actions"><ScanButton onInk /></div>
    </div>

    <div class="stage-ring" bind:clientWidth={ringBox}>
      {#if devices.length && ringBox}
        <NetRing {devices} size={ringSize} labels={ringLabels} />
        <ul class="legend" aria-hidden="true">
          <li><span class="key on"></span>{t('overview.online')}</li>
          <li><span class="key"></span>{t('overview.offline')}</li>
          <li><span class="key new"></span>{t('overview.new')}</li>
        </ul>
      {/if}
    </div>
  </div>
</section>

<div class="wrap">
  {#if noChannel}
    <div class="notice warn row-notice" role="status">
      <Icon name="warning" size={17} />
      <span>{t('settings.channelsMissing')}</span>
      <a class="btn small" href="/settings/notifications" use:link>{t('settings.channelsSetup')}</a>
    </div>
  {/if}
  {#if scanner?.mode === 'demo'}
    <div class="notice info"><Icon name="info" size={17} /> {t('overview.demo')}</div>
  {:else if scanner?.mode === 'fallback'}
    <div class="notice warn"><Icon name="warning" size={17} /> {t('overview.fallback')}</div>
  {/if}
  {#if scanner?.error}
    <div class="notice danger"><Icon name="warning" size={17} /> {t('overview.scanError', { msg: scanner.error })}</div>
  {/if}
  {#each scanner?.interfaces?.filter((i) => i.active && (i.skipped === 'large' || i.skipped === 'too_large')) ?? [] as i (i.name)}
    <div class="notice warn"><Icon name="warning" size={17} /> {t('overview.subnetSkipped', { p: i.prefix })}</div>
  {/each}
  {#if ov && !ov.baseline && scanner?.mode !== 'demo'}
    <div class="notice info"><Icon name="info" size={17} /> {t('overview.learning')}</div>
  {/if}

  <div class="grid">
    <div class="col">
      <section class="section" aria-labelledby="att">
        <div class="section-head">
          <h2 id="att">{t('overview.attention')} <span class="count-label">{fresh.length}</span></h2>
          {#if fresh.length > 1}
            <button class="btn small" type="button" onclick={() => (confirmAll = true)}><Icon name="check" size={14} /> {t('overview.markAllKnown')}</button>
          {/if}
        </div>
        {#if store.loading}
          {#each [1, 2] as i (i)}<div class="skeleton sk-row"></div>{/each}
        {:else if fresh.length === 0}
          <p class="empty"><Icon name="check-circle" size={18} /> {t('overview.attentionEmpty')}</p>
        {:else}
          <ul class="fresh">
            {#each fresh.slice(0, 6) as d (d.id)}
              <li>
                <a class="fresh-main" href="/devices/{d.id}" use:link>
                  <DeviceIcon type={d.type} online={d.online} fresh size={38} />
                  <span class="fresh-text">
                    <span class="fresh-name">{displayName(d)}</span>
                    <span class="sub">{subtitle(d) || typeLabel(d.type)}</span>
                  </span>
                  <span class="mono ip">{d.ip}</span>
                  <span class="faint seen">{relTime(d.firstSeen, clock.now)}</span>
                </a>
                <button class="btn small" type="button" onclick={() => markKnown(d.id)}><Icon name="check" size={14} /> {t('overview.markKnown')}</button>
              </li>
            {/each}
          </ul>
          {#if fresh.length > 6}
            <a class="more" href="/devices?filter=new" use:link>{t('common.showAll')} ({fresh.length}) <Icon name="caret-right" size={14} /></a>
          {/if}
        {/if}
      </section>

      <section class="section" aria-labelledby="sw">
        <div class="section-head">
          <h2 id="sw">{t('overview.software')}</h2>
          {#if swList.length > 5}<a class="small" href="/devices?filter=software" use:link>{t('common.showAll')}</a>{/if}
        </div>
        {#if store.loading}
          <div class="skeleton sk-row"></div>
        {:else if swList.length === 0}
          <p class="empty"><Icon name="check-circle" size={18} /> {t('overview.software.none')}</p>
        {:else}
          <ul class="swl">
            {#each swList.slice(0, 5) as d (d.id)}
              <li>
                <a href="/devices/{d.id}" use:link>
                  <DeviceIcon type={d.type} online={d.online} size={34} />
                  <span class="fresh-text">
                    <span class="fresh-name">{displayName(d)}</span>
                    <span class="sub">{d.software?.name}</span>
                  </span>
                  <span class="badge {d.software?.status === 'outdated' ? 'danger' : 'warn'}">{t(`overview.software.state.${d.software?.status}`)}</span>
                </a>
              </li>
            {/each}
          </ul>
        {/if}
        <p class="sw-tally faint small">
          {t('overview.software.tally', { outdated: swCounts.outdated, update: swCounts.update + swCounts.aging, ok: swCounts.supported, unknown: swCounts.unknown })}
        </p>
        {#if swCounts.unknown > 0}
          <details class="sw-why">
            <summary>{t('overview.software.why')}</summary>
            <p class="small">{t('overview.software.whyText')}</p>
          </details>
        {/if}
      </section>

      <section class="section" aria-labelledby="types">
        <div class="section-head"><h2 id="types">{t('overview.byType')}</h2></div>
        <div class="mixbar" aria-hidden="true">
          {#each mix as g, i (g.cat)}
            <span class="seg-{i % 7}" style:flex-grow={g.count} title="{t(`category.${g.cat}`)} {g.count}"></span>
          {/each}
        </div>
        <dl class="mix">
          {#each mix as g, i (g.cat)}
            <div class="mix-group">
              <dt><span class="key seg-{i % 7}"></span>{t(`category.${g.cat}`)} <span class="num faint">{g.count}</span></dt>
              <dd>
                {#each g.items as it (it.type)}
                  <a href="/devices?type={it.type}" use:link>{typeLabel(it.type)} <span class="num">{it.count}</span></a>
                {/each}
              </dd>
            </div>
          {/each}
        </dl>
      </section>
    </div>

    <div class="col">
      <section class="section" aria-labelledby="home">
        <div class="section-head">
          <h2 id="home">{t('persons.title')}</h2>
          <a class="small" href="/persons" use:link>{t('persons.manage')}</a>
        </div>
        {#if people === null}
          <div class="skeleton sk-row"></div>
        {:else if people.length === 0}
          <p class="faint small">{t('persons.emptyShort')}</p>
        {:else}
          <ul class="people">
            {#each people as p (p.id)}
              {@const home = isHome(p, store.devices)}
              <li class:home>
                <span class="avatar" aria-hidden="true">{initials(p.name)}</span>
                <span class="who">
                  <span class="pname">{p.name}</span>
                  <span class="sub">{presenceText(p, home, clock.now)}</span>
                </span>
                {#if p.devices.some((d) => d.presence)}
                  <span class="badge {home ? 'ok' : ''}">{home ? t('persons.badge.home') : t('persons.badge.away')}</span>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="section" aria-labelledby="act">
        <div class="section-head">
          <h2 id="act">{t('overview.activity')}</h2>
          <a class="small" href="/activity" use:link>{t('common.showAll')}</a>
        </div>
        <ol class="feed">
          {#each feed as e (e.id)}
            {@const dev = deviceName(e.deviceId)}
            {@const day = shortDay(e.ts, clock.now)}
            <li class={e.severity}>
              <time class="num" datetime={new Date(e.ts).toISOString()} title={new Date(e.ts).toLocaleString()}>
                {time(e.ts)}{#if day}<span class="day">{day}</span>{/if}
              </time>
              <span class="sev" aria-hidden="true">{#if e.severity === 'warning' || e.severity === 'critical'}<span class="dot {e.severity === 'critical' ? 'danger' : 'warn'}"></span>{/if}</span>
              <span class="feed-text">
                {#if dev}
                  <a href="/devices/{dev.id}" use:link>{eventText(e, displayName(dev))}</a>
                {:else}
                  {eventText(e)}
                {/if}
              </span>
            </li>
          {:else}
            {#if feedLoaded}<li class="feed-empty">{t('overview.activityEmpty')}</li>{/if}
          {/each}
        </ol>
      </section>

      <section class="section" aria-labelledby="sec">
        <div class="section-head"><h2 id="sec">{t('overview.security')}</h2></div>
        <a class="sec" href="/security" use:link>
          {#if (ov?.security.critical ?? 0) + (ov?.security.warning ?? 0) > 0}
            <span class="sec-figures">
              {#if ov?.security.critical}<span class="crit"><strong class="num">{ov.security.critical}</strong> {t('security.critical')}</span>{/if}
              {#if ov?.security.warning}<span class="warnc"><strong class="num">{ov.security.warning}</strong> {t('security.count.warningWord', { n: ov.security.warning })}</span>{/if}
            </span>
          {:else}
            <span class="sec-ok"><Icon name="shield-check" size={20} /> {ov?.security.lastRunAt ? t('overview.securityOk') : t('overview.securityNever')}</span>
          {/if}
          <span class="faint small when">{ov?.security.lastRunAt ? t('overview.securityLast', { t: relTime(ov.security.lastRunAt, clock.now) }) : ''}</span>
          <Icon name="caret-right" size={16} />
        </a>
        {#if auth.state && !auth.state.https}
          <p class="hint http">{t('overview.http')}</p>
        {/if}
      </section>
    </div>
  </div>
</div>

<style>
  /* Stage: the dark band continues into the page and carries the ring. */
  .stage {
    background: var(--ink);
    color: var(--on-ink);
    border-bottom: 1px solid var(--ink-line);
  }
  .stage :focus-visible {
    outline-color: var(--on-ink);
  }
  .stage-inner {
    max-width: var(--page);
    margin: 0 auto;
    padding: 24px 32px 28px;
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 48px;
    align-items: center;
  }
  .stage-text {
    display: grid;
    gap: 22px;
    align-content: center;
    min-width: 0;
    max-width: 520px;
  }
  .where {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 0;
    font-size: 13px;
    color: var(--on-ink-2);
  }
  .where > span + span::before {
    content: '·';
    margin: 0 8px;
    opacity: 0.6;
  }
  .where .mono {
    font-size: 12px;
  }
  .headline {
    font-size: 30px;
    line-height: 1.2;
    font-weight: 500;
    letter-spacing: -0.015em;
  }
  .headline a {
    color: var(--on-ink);
  }
  .headline a:hover {
    text-decoration: none;
  }
  .headline a:hover .hl-num {
    text-decoration: underline;
    text-decoration-thickness: 2px;
    text-underline-offset: 5px;
  }
  .hl-num {
    font-weight: 600;
    color: var(--signal);
  }
  .facts {
    display: grid;
    grid-template-columns: auto auto minmax(0, 1fr);
    gap: 0 36px;
    margin: 0;
    padding-top: 18px;
    border-top: 1px solid var(--ink-line);
  }
  .fact {
    display: grid;
    gap: 2px;
    align-content: start;
    color: var(--on-ink);
    text-decoration: none;
  }
  a.fact:hover {
    text-decoration: none;
  }
  a.fact:hover dd {
    text-decoration: underline;
    text-underline-offset: 4px;
  }
  .fact dt {
    font-size: 13px;
    color: var(--on-ink-2);
  }
  .fact dd {
    margin: 0;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 22px;
    font-weight: 600;
    line-height: 1.3;
  }
  .inet-state {
    font-size: 16px;
  }
  .inet-state.ok {
    color: var(--signal);
  }
  .inet-state.bad {
    color: #ff8478; /* readable on the dark stage in both themes */
  }
  .fact dd.amber {
    color: var(--amber);
  }
  .lat {
    font-size: 13px;
    font-weight: 400;
    color: var(--on-ink-2);
  }
  .spark {
    display: block;
    max-width: 220px;
    margin-top: 6px;
  }
  .stage-actions {
    display: flex;
    gap: 10px;
  }
  .stage-ring {
    display: grid;
    justify-items: center;
    gap: 6px;
    width: 580px;
  }
  .legend {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    gap: 18px;
    font-size: 12px;
    color: var(--on-ink-2);
  }
  .legend li {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .legend .key {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    border: 1.5px solid var(--on-ink-2);
  }
  .legend .key.on {
    background: var(--signal);
    border-color: var(--signal);
  }
  .legend .key.new {
    background: var(--amber);
    border-color: var(--amber);
  }

  /* Canvas */
  .wrap {
    max-width: var(--page);
    margin: 0 auto;
    padding: 24px 32px 56px;
    display: grid;
    gap: 12px;
  }
  .row-notice {
    align-items: center;
    flex-wrap: wrap;
  }
  .row-notice span {
    flex: 1 1 260px;
  }
  .grid {
    display: grid;
    grid-template-columns: minmax(0, 1.3fr) minmax(0, 1fr);
    gap: 16px;
    margin-top: 4px;
    align-items: start;
  }
  .col {
    display: grid;
    gap: 16px;
    min-width: 0;
  }
  h2 {
    display: flex;
    align-items: baseline;
    gap: 8px;
    white-space: nowrap;
  }
  .small {
    font-size: 13px;
  }
  .section-head a.small {
    color: var(--text-2);
    font-weight: 500;
  }
  .section-head a.small:hover {
    color: var(--text);
  }
  .empty {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 0;
    color: var(--text-2);
  }
  .empty :global(svg) {
    color: var(--ok);
  }
  .sk-row {
    height: 48px;
    margin: 4px 0;
  }

  .fresh,
  .swl,
  .people,
  .feed {
    list-style: none;
    margin: 0 -16px;
    padding: 0;
  }
  .fresh li + li,
  .swl li + li,
  .people li + li,
  .feed li + li {
    border-top: 1px solid var(--border);
  }
  .fresh li {
    display: flex;
    align-items: center;
    gap: 12px;
    padding-right: 16px;
  }
  .fresh-main {
    flex: 1;
    min-width: 0;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto auto;
    align-items: center;
    gap: 12px;
    padding: 10px 0 10px 16px;
    color: inherit;
    text-decoration: none;
  }
  .fresh-main:hover .fresh-name,
  .swl a:hover .fresh-name {
    text-decoration: underline;
    text-underline-offset: 3px;
  }
  .fresh-text {
    display: grid;
    min-width: 0;
  }
  .fresh-name {
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sub {
    font-size: 13px;
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .seen {
    font-size: 13px;
  }
  .more {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-top: 10px;
    font-weight: 500;
  }

  .swl a {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 12px;
    padding: 9px 16px;
    color: inherit;
    text-decoration: none;
  }
  .swl a:hover {
    background: var(--surface-2);
  }
  .sw-tally {
    margin-top: 12px;
  }
  .sw-why summary {
    margin-top: 6px;
    font-size: 13px;
    color: var(--text-2);
  }
  .sw-why p {
    margin-top: 6px;
    color: var(--text-2);
    max-width: 62ch;
  }

  /* Device mix */
  .mixbar {
    display: flex;
    height: 8px;
    gap: 2px;
    margin: 2px 0 16px;
    border-radius: 4px;
    overflow: hidden;
  }
  .mixbar span {
    flex-basis: 0;
    min-width: 4px;
  }
  .seg-0 {
    background: var(--text);
  }
  .seg-1 {
    background: color-mix(in srgb, var(--text) 78%, var(--surface));
  }
  .seg-2 {
    background: color-mix(in srgb, var(--text) 60%, var(--surface));
  }
  .seg-3 {
    background: color-mix(in srgb, var(--text) 44%, var(--surface));
  }
  .seg-4 {
    background: color-mix(in srgb, var(--text) 30%, var(--surface));
  }
  .seg-5 {
    background: color-mix(in srgb, var(--text) 22%, var(--surface));
  }
  .seg-6 {
    background: color-mix(in srgb, var(--text) 16%, var(--surface));
  }
  .mix {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
    gap: 14px 24px;
    margin: 0;
  }
  .mix dt {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 500;
  }
  .key {
    width: 10px;
    height: 10px;
    border-radius: 3px;
    flex: none;
  }
  .mix dd {
    margin: 4px 0 0 18px;
    display: flex;
    flex-wrap: wrap;
    gap: 2px 12px;
    font-size: 13px;
  }
  .mix dd a {
    color: var(--text-2);
  }
  .mix dd a:hover {
    color: var(--text);
  }

  /* Activity */
  .feed li {
    display: grid;
    grid-template-columns: 6ch 8px minmax(0, 1fr);
    gap: 10px;
    padding: 9px 16px;
  }
  .feed .sev {
    padding-top: 7px;
  }
  .feed .sev .dot {
    display: block;
  }
  .feed time {
    color: var(--text-2);
    font-size: 13px;
    padding-top: 1px;
    white-space: nowrap;
    overflow: hidden;
  }
  .feed .day {
    display: block;
    font-size: 12px;
  }
  .feed-text a {
    color: var(--text);
    text-decoration: none;
  }
  .feed-text a:hover {
    text-decoration: underline;
  }
  .feed li.feed-empty {
    display: block;
    color: var(--text-2);
  }

  /* Security */
  .sec {
    display: flex;
    align-items: center;
    gap: 16px;
    margin: 0 -16px -16px;
    padding: 12px 16px;
    border-radius: 0 0 var(--radius) var(--radius);
    color: var(--text);
    text-decoration: none;
  }
  .sec:hover {
    text-decoration: none;
    background: var(--surface-2);
  }
  .sec-figures {
    display: flex;
    gap: 20px;
  }
  .sec-figures strong {
    font-size: 20px;
    font-weight: 600;
    margin-right: 4px;
  }
  .crit {
    color: var(--danger);
    font-weight: 500;
  }
  .warnc {
    color: var(--warn);
    font-weight: 500;
  }
  .sec-ok {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--ok);
    font-weight: 500;
  }
  .sec > :global(:last-child) {
    margin-left: auto;
    color: var(--text-2);
  }
  .http {
    margin-top: 14px;
  }

  /* People */
  .people li {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 12px;
    padding: 9px 16px;
  }
  .avatar {
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: var(--surface-3);
    color: var(--text-2);
    font-weight: 600;
    font-size: 13px;
  }
  .home .avatar {
    background: var(--ok-soft);
    color: var(--ok);
  }
  .who {
    display: grid;
    min-width: 0;
  }
  .pname {
    font-weight: 500;
  }

  @media (max-width: 480px) {
    /* The check time goes under the figures instead of wrapping beside them. */
    .sec {
      flex-wrap: wrap;
      row-gap: 4px;
    }
    .sec .when {
      order: 3;
      flex-basis: 100%;
    }
  }
  @media (max-width: 1100px) {
    .fresh-main {
      grid-template-columns: auto minmax(0, 1fr) auto;
    }
    .fresh-main .seen {
      display: none;
    }
  }
  @media (max-width: 1180px) {
    .stage-ring {
      width: 460px;
    }
  }
  @media (max-width: 960px) {
    .stage-inner {
      grid-template-columns: minmax(0, 1fr);
      gap: 28px;
    }
    .stage-text {
      max-width: none;
    }
    .stage-ring {
      width: 100%;
    }
    .grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  @media (max-width: 640px) {
    .stage-inner {
      padding: 18px 16px 20px;
      gap: 16px;
    }
    .stage-text {
      gap: 14px;
    }
    .facts {
      padding-top: 12px;
    }
    .headline {
      font-size: 26px;
    }
    .facts {
      gap: 0 24px;
    }
    .fact dd {
      font-size: 20px;
    }
    .spark,
    .lat {
      display: none;
    }
    .section-head {
      flex-wrap: wrap;
    }
    .wrap {
      padding: 16px 16px 40px;
    }
    .fresh-main {
      grid-template-columns: auto minmax(0, 1fr);
    }
    .fresh-main .ip,
    .fresh-main .seen {
      display: none;
    }
  }
</style>
