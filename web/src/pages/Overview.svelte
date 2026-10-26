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
  import type { EventItem } from '../lib/types';
  import { number, relTime, shortDay, t, time } from '../lib/i18n.svelte';
  import type { DeviceType } from '../lib/types';

  const devices = $derived([...store.devices.values()]);
  const online = $derived(devices.filter((d) => d.online).length);
  const fresh = $derived(devices.filter((d) => d.new).sort((a, b) => b.firstSeen - a.firstSeen));
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
  const ringLabels = $derived(ringBox >= 380);
  const ringSize = $derived(Math.min(380, ringBox - (ringLabels ? 120 : 30)));
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
      <!-- Network name and subnet are in the band above; the stage leads with the count. -->
      <h1 id="net-title" class="sr-only">{ov?.networkName || store.settings?.networkName || t('overview.unnamed')}</h1>
      <p class="eyebrow on-ink">
        {#each active as i (i.name)}<span>{i.name}</span>{/each}
        {#if scanner?.gateways?.length}<span>{t('overview.gateway')} {scanner.gateways[0]}</span>{/if}
      </p>

      <a class="headline" href="/devices?filter=online" use:link>
        <span class="big num">{store.loading ? '–' : number(online)}</span>
        <span class="big-of">
          <span>{t('overview.onlineOf')}</span>
          <strong>{t('overview.devicesCount', { n: devices.length })}</strong>
        </span>
      </a>

      <dl class="metrics">
        <a class="metric" href="/devices?filter=new" use:link>
          <dt>{t('overview.new')}</dt>
          <dd class="num" class:amber={fresh.length > 0}>{fresh.length}</dd>
        </a>
        <a class="metric" href="/devices?filter=offline" use:link>
          <dt>{t('overview.offline')}</dt>
          <dd class="num">{devices.length - online}</dd>
        </a>
        <div class="metric inet">
          <dt>{t('overview.internet')}</dt>
          <dd>
            <span class="inet-state" class:ok={inetOk} class:bad={inetState === 'offline' || inetState === 'lan_down'}>
              {t(`internet.${inetState}`)}
            </span>
            {#if inetOk && ov?.internet.latencyMs !== undefined && ov.internet.latencyMs >= 0}
              <span class="lat">{ov.internet.latencyMs} ms</span>
            {/if}
          </dd>
          {#if history.length > 1}<span class="spark"><Sparkline points={history} height={22} /></span>{/if}
        </div>
      </dl>

      <div class="stage-actions"><ScanButton onInk /></div>
    </div>

    <div class="stage-ring" bind:clientWidth={ringBox}>
      {#if devices.length && ringBox}<NetRing {devices} size={ringSize} labels={ringLabels} />{/if}
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
          <h2 id="att">{t('overview.attention')} <span class="eyebrow">{fresh.length}</span></h2>
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
                  <span class="mono faint seen">{relTime(d.firstSeen, clock.now)}</span>
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
              <time class="mono" datetime={new Date(e.ts).toISOString()} title={new Date(e.ts).toLocaleString()}>
                {time(e.ts)}{#if day}<span class="day">{day}</span>{/if}
              </time>
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
  /* Stage: the ink band continues into the page */
  .stage {
    background: var(--ink);
    color: var(--on-ink);
  }
  .stage :focus-visible {
    outline-color: var(--on-ink);
  }
  .stage-inner {
    max-width: var(--page);
    margin: 0 auto;
    padding: 30px 28px 40px;
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 40px;
    align-items: center;
  }
  .stage-text {
    display: grid;
    gap: 18px;
    align-content: center;
    min-width: 0;
  }
  .eyebrow.on-ink {
    color: var(--on-ink-2);
    display: flex;
    flex-wrap: wrap;
    gap: 6px 14px;
  }
  .headline {
    display: flex;
    align-items: flex-end;
    gap: 16px;
    color: var(--on-ink);
    text-decoration: none;
  }
  .headline:hover {
    text-decoration: none;
  }
  .big {
    font-size: 120px;
    line-height: 0.82;
    font-weight: 800;
    letter-spacing: -0.06em;
    color: var(--signal);
  }
  .big-of {
    display: grid;
    padding-bottom: 6px;
    font-size: 16px;
    line-height: 1.25;
    color: var(--on-ink-2);
  }
  .big-of strong {
    color: var(--on-ink);
    font-weight: 600;
    font-size: 20px;
    letter-spacing: -0.015em;
  }
  .metrics {
    display: grid;
    grid-template-columns: auto auto minmax(0, 1fr);
    margin: 4px 0 0;
    border-top: 1px solid var(--ink-line);
  }
  .metric {
    display: grid;
    gap: 2px;
    align-content: start;
    padding: 14px 28px 0 0;
    margin-right: 28px;
    border-right: 1px solid var(--ink-line);
    color: var(--on-ink);
    text-decoration: none;
  }
  .metric:hover {
    text-decoration: none;
  }
  .metric:hover dt {
    color: var(--on-ink);
  }
  .metric.inet {
    border-right: 0;
    margin-right: 0;
    padding-right: 0;
  }
  .metric dt {
    font-family: var(--mono);
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--on-ink-2);
  }
  .metric dd {
    margin: 0;
    font-size: 30px;
    font-weight: 800;
    letter-spacing: -0.03em;
    display: flex;
    align-items: baseline;
    gap: 10px;
  }
  .metric dd.amber {
    color: var(--amber);
  }
  .inet-state {
    font-size: 20px;
    font-weight: 800;
    line-height: 1.35;
  }
  .inet-state.ok {
    color: var(--signal);
  }
  .inet-state.bad {
    color: #ff7b6e;
  }
  .lat {
    font-family: var(--mono);
    font-size: 13px;
    font-weight: 500;
    color: var(--on-ink-2);
    letter-spacing: 0;
  }
  .spark {
    display: block;
    max-width: 260px;
    margin-top: 4px;
  }
  .stage-actions {
    display: flex;
    gap: 10px;
  }
  .stage-ring {
    display: grid;
    place-items: center;
    width: 510px;
    padding: 34px 0;
  }

  /* Paper */
  .wrap {
    max-width: var(--page);
    margin: 0 auto;
    padding: 28px 28px 56px;
    display: grid;
    gap: 16px;
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
    grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr);
    gap: 40px;
    margin-top: 12px;
    align-items: start;
  }
  .col {
    display: grid;
    gap: 40px;
    min-width: 0;
  }
  h2 {
    display: flex;
    align-items: baseline;
    gap: 10px;
    white-space: nowrap;
  }
  .small {
    font-size: 13px;
  }
  .empty {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 0;
    color: var(--text-2);
  }
  .sk-row {
    height: 52px;
    margin: 4px 0;
  }

  .fresh {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .fresh li {
    display: flex;
    align-items: center;
    gap: 12px;
    border-bottom: 1px solid var(--border);
  }
  .fresh-main {
    flex: 1;
    min-width: 0;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto auto;
    align-items: center;
    gap: 14px;
    padding: 10px 0;
    color: inherit;
    text-decoration: none;
  }
  .fresh-main:hover .fresh-name {
    text-decoration: underline;
    text-underline-offset: 3px;
  }
  .fresh-text {
    display: grid;
    min-width: 0;
  }
  .fresh-name {
    font-weight: 600;
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
  .more {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-top: 10px;
    font-weight: 600;
  }

  /* Device mix */
  .mixbar {
    display: flex;
    height: 12px;
    gap: 2px;
    margin: 6px 0 16px;
  }
  .mixbar span {
    flex-basis: 0;
    min-width: 4px;
  }
  .seg-0 {
    background: var(--text);
  }
  .seg-1 {
    background: color-mix(in srgb, var(--text) 78%, var(--bg));
  }
  .seg-2 {
    background: color-mix(in srgb, var(--text) 60%, var(--bg));
  }
  .seg-3 {
    background: color-mix(in srgb, var(--text) 44%, var(--bg));
  }
  .seg-4 {
    background: color-mix(in srgb, var(--text) 30%, var(--bg));
  }
  .seg-5 {
    background: color-mix(in srgb, var(--text) 20%, var(--bg));
  }
  .seg-6 {
    background: color-mix(in srgb, var(--text) 12%, var(--bg));
  }
  .mix {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 14px 24px;
    margin: 0;
  }
  .mix dt {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
  }
  .key {
    width: 10px;
    height: 10px;
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
  .feed {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .feed li {
    display: grid;
    grid-template-columns: 8ch minmax(0, 1fr);
    gap: 12px;
    padding: 8px 0 8px 12px;
    border-left: 2px solid var(--border-strong);
    font-size: 14px;
  }
  .feed li.warning {
    border-left-color: var(--amber);
  }
  .feed li.critical {
    border-left-color: var(--danger);
  }
  .feed time {
    color: var(--text-3);
    padding-top: 1px;
    white-space: nowrap;
    overflow: hidden;
  }
  .feed .day {
    display: block;
    font-size: 11px;
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
    padding: 6px 0;
    color: var(--text);
    text-decoration: none;
  }
  .sec:hover {
    text-decoration: none;
  }
  .sec-figures {
    display: flex;
    gap: 22px;
  }
  .sec-figures strong {
    font-size: 30px;
    font-weight: 800;
    letter-spacing: -0.03em;
    margin-right: 4px;
  }
  .crit {
    color: var(--danger);
    font-weight: 600;
  }
  .warnc {
    color: var(--warn);
    font-weight: 600;
  }
  .sec-ok {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--ok);
    font-weight: 600;
  }
  .sec > :global(:last-child) {
    margin-left: auto;
  }
  .http {
    margin-top: 10px;
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
      width: 400px;
    }
    .big {
      font-size: 96px;
    }
  }
  @media (max-width: 900px) {
    .stage-inner {
      grid-template-columns: minmax(0, 1fr);
    }
    .stage-ring {
      width: 100%;
      padding: 10px 0 0;
    }
    .grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  @media (max-width: 640px) {
    .stage-inner {
      padding: 22px 16px 28px;
      gap: 24px;
    }
    .big {
      font-size: 88px;
    }
    .metrics {
      grid-template-columns: auto auto minmax(0, 1fr);
    }
    .metric {
      padding-right: 16px;
      margin-right: 16px;
    }
    .metric dd {
      font-size: 30px;
    }
    .inet-state {
      font-size: 16px;
    }
    .spark,
    .lat {
      display: none;
    }
    /* Phones: the answer first; the ring needs room to be useful. */
    .stage-ring {
      display: none;
    }
    .section-head {
      flex-wrap: wrap;
    }
    .wrap {
      padding: 20px 16px 40px;
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
