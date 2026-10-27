<script lang="ts">
  import Icon from './Icon.svelte';
  import { toast } from './toast.svelte';
  import { api } from '../api';
  import { errorText } from '../errors';
  import { dateLong, dateTime, has, t } from '../i18n.svelte';
  import type { Device, SoftwareAssessment } from '../types';

  // What Netzwart knows about the software of one device, what it means, and
  // what to do. Also where the user fills in what the network cannot see.
  let { device, software, demo = false, onchange }: {
    device: Device;
    software: SoftwareAssessment;
    demo?: boolean;
    onchange: (s: SoftwareAssessment) => void;
  } = $props();

  const s = $derived(software);
  const manual = $derived(s.observations.find((o) => o.source === 'manual'));
  const isWindows = $derived(s.product === 'windows');
  const edition = $derived(s.edition ?? '');
  // Phones and tablets hide their system version; let the user say it.
  const canManual = $derived(
    !isWindows && (['phone', 'tablet', 'watch', 'laptop', 'computer'].includes(device.type) || s.product === 'ios' || s.product === 'ipados' || s.product === 'macos' || s.product === 'android'),
  );
  const dataOld = $derived(Date.now() - Date.parse(s.dataAsOf) > 183 * 86_400_000);
  // Known exploited vulnerabilities (offline list, docs/09 F6). Nothing is
  // shown when none applies: the list never proves a device is safe.
  const x = $derived(s.exploited && s.exploited.verdict !== 'none' ? s.exploited : undefined);
  const xOld = $derived(!!x && Date.now() - Date.parse(x.dataAsOf) > 90 * 86_400_000);
  const safeUrl = (u?: string) => (u && u.startsWith('https://support.apple.com/') ? u : undefined);

  // A confirmed exploited hole outranks "gets updates": the update is there, but not installed.
  const urgent = $derived(s.exploited?.verdict === 'affected' && !s.exploited.manual);
  const headline = $derived.by(() => {
    if (urgent) return t('software.status.exploited');
    switch (s.status) {
      case 'outdated':
        return s.since ? t('software.status.outdatedSince', { d: dateLong(s.since) }) : t('software.status.outdated');
      case 'aging':
        return s.until ? t('software.status.agingUntil', { d: dateLong(s.until) }) : t('software.status.aging');
      case 'update':
        return t('software.status.update');
      case 'supported':
        return s.until ? t('software.status.supportedUntil', { d: dateLong(s.until) }) : t('software.status.supported');
      default:
        return s.note === 'stale' ? t('software.status.stale') : t('software.status.unknown');
    }
  });

  // Advice by product and status, falling back to a general text.
  const advice = $derived.by(() => {
    if (s.status === 'supported' || s.status === 'unknown') return '';
    for (const key of [`software.advice.${s.product}.${s.status}`, `software.advice.${s.status}`]) {
      if (has(key)) return t(key);
    }
    return '';
  });

  const why = $derived.by(() => {
    if (s.status === 'unknown' && !s.name) {
      if (device.os === 'iOS' || device.os === 'iPadOS' || /^(iPhone|iPad)/.test(device.model)) return t('software.why.apple');
      if (device.type === 'phone' || device.type === 'tablet') return t('software.why.mobile');
      if (device.os === 'Windows' || device.type === 'computer' || device.type === 'laptop') return t('software.why.windows');
      return t('software.why.generic');
    }
    return s.note && has(`software.note.${s.note}`) ? t(`software.note.${s.note}`, { max: s.maxOs ?? '' }) : '';
  });

  let product = $state('ios');
  let version = $state('');
  let exact = $state(''); // optional, e.g. 18.6.2: enables the check for exploited holes
  let editing = $state(false);
  // German keyboards offer a comma: "18,6,2" means 18.6.2.
  const exactNorm = $derived(exact.trim().replaceAll(',', '.'));
  const exactOk = $derived(exactNorm === '' || (/^\d{1,2}(\.\d{1,2}){1,2}$/.test(exactNorm) && exactNorm.startsWith(`${version}.`)));
  let busy = $state(false);

  function startEdit() {
    product = manual?.product ?? (device.os === 'Android' ? 'android' : s.product && ['ios', 'ipados', 'macos', 'android'].includes(s.product) ? s.product : device.type === 'tablet' ? 'ipados' : 'ios');
    version = manual?.version.split('.')[0] ?? '';
    exact = manual?.version.includes('.') ? manual.version : '';
    editing = true;
  }

  const versions: Record<string, string[]> = {
    ios: ['27', '26', '18', '17', '16', '15', '14', '12'],
    ipados: ['27', '26', '18', '17', '16', '15', '14', '12'],
    macos: ['27', '26', '15', '14', '13', '12', '11'],
    android: ['17', '16', '15', '14', '13', '12', '11', '10', '9'],
  };

  async function send(body: Parameters<typeof api.setSoftware>[1]) {
    busy = true;
    try {
      onchange(await api.setSoftware(device.id, body));
      editing = false;
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      busy = false;
    }
  }
</script>

<div class="sw" data-status={urgent ? 'outdated' : s.status}>
  <p class="verdict"><span class="mark" aria-hidden="true"></span><strong>{headline}</strong></p>
  {#if s.name || s.maxOs}
    <p class="system">
      {#if s.name}<span>{s.name}</span>{#if s.version && s.product === 'windows'}<span class="mono faint"> · {s.version}</span>{/if}{/if}
      {#if s.maxOs && s.maxOs !== s.name}
        <span class="faint"> {s.name ? '·' : ''} {s.status === 'unknown' && !s.name ? t('software.canRun', { os: s.maxOs }) : t('software.maxOs', { os: s.maxOs })}</span>
      {/if}
    </p>
  {/if}
  {#if s.conflict}<p class="notice warn small">{t('software.conflict')}</p>{/if}
  {#if x}
    <div class="exploited" class:uncertain={x.verdict === 'uncertain'} class:manual={x.manual}>
      <p class="x-title">
        <strong>
          {#if x.verdict === 'uncertain'}{t('software.exploited.uncertain', { version: x.version, fixed: x.fixed ?? '' })}
          {:else if x.manual}{t(x.cves?.length === 1 ? 'software.exploited.manual.one' : 'software.exploited.manual', { version: x.version, n: x.cves?.length ?? 0 })}
          {:else}{t(x.cves?.length === 1 ? 'software.exploited.affected.one' : 'software.exploited.affected', { n: x.cves?.length ?? 0 })}{/if}
        </strong>
      </p>
      {#if x.verdict === 'affected'}
        <p>{t('software.exploited.fix', { fixed: x.fixed ?? '' })}</p>
        <ul class="cves">
          {#each (x.cves ?? []).slice(0, 5) as c (c.id)}
            <li>
              {#if safeUrl(c.url)}<a href={safeUrl(c.url)} target="_blank" rel="noopener noreferrer">{c.id}</a>{:else}<span>{c.id}</span>{/if}
              <span class="faint">· {t('software.exploited.added', { d: dateLong(c.added) })}</span>
            </li>
          {/each}
          {#if (x.cves?.length ?? 0) > 5}<li class="faint">{t('software.exploited.more', { n: (x.cves?.length ?? 0) - 5 })}</li>{/if}
        </ul>
      {/if}
      <p class="faint small" class:old={xOld}>{t(xOld ? 'software.exploited.listOld' : 'software.exploited.list', { d: dateLong(x.dataAsOf) })}</p>
    </div>
  {/if}
  {#if s.note === 'stale' && s.lastSeen}<p class="hint">{t('software.lastSeen', { d: dateTime(s.lastSeen) })}</p>{/if}
  {#if why}<p class="why">{why}</p>{/if}
  {#if advice}
    <div class="advice">
      <p class="label">{t('software.whatToDo')}</p>
      <p>{advice}</p>
    </div>
  {/if}

  {#if isWindows}
    <div class="edition">
      <span class="label" id="ed-{device.id}">{t('software.edition')}</span>
      <div class="seg" role="group" aria-labelledby="ed-{device.id}">
        {#each ['', 'enterprise', 'ltsc'] as ed (ed)}
          <button type="button" aria-pressed={edition === ed} disabled={busy || demo} onclick={() => send({ edition: ed })}>{t(`software.edition.${ed || 'home'}`)}</button>
        {/each}
      </div>
    </div>
  {/if}

  {#if canManual}
    {#if editing}
      <form class="manual" onsubmit={(e) => { e.preventDefault(); send({ manual: { product, version: exactNorm || version } }); }}>
        <p class="hint">{t(`software.manual.where.${product}`)}</p>
        <div class="row">
          <select class="select" bind:value={product} aria-label={t('software.manual.system')} onchange={() => { version = ''; exact = ''; }}>
            {#each Object.keys(versions) as p (p)}<option value={p}>{t(`software.product.${p}`)}</option>{/each}
          </select>
          <select class="select" bind:value={version} aria-label={t('software.manual.version')} required>
            <option value="" disabled>{t('software.manual.pick')}</option>
            {#each versions[product] as v (v)}<option value={v}>{v}</option>{/each}
          </select>
          {#if product !== 'android'}
            <input class="input exact" bind:value={exact} maxlength="8" placeholder={version ? `${version}.x.x` : ''} disabled={!version}
              aria-label={t('software.manual.exact')} aria-invalid={!exactOk} aria-describedby="exact-{device.id}" />
          {/if}
        </div>
        {#if product !== 'android'}<p class="hint" id="exact-{device.id}" aria-live="polite">{exactOk || !version ? t('software.manual.exactHint') : t('software.manual.exactBad', { v: version })}</p>{/if}
        <div class="row">
          <button class="btn primary small" type="submit" disabled={busy || !version || !exactOk}>{t('common.save')}</button>
          <button class="btn small" type="button" onclick={() => (editing = false)}>{t('common.cancel')}</button>
          {#if manual}<button class="btn small" type="button" disabled={busy} onclick={() => send({ manual: { product: '', version: '' } })}>{t('software.manual.clear')}</button>{/if}
        </div>
      </form>
    {:else}
      <button class="btn small" type="button" disabled={demo} title={demo ? t('err.demo_mode') : undefined} onclick={startEdit}>
        <Icon name="pencil-simple" size={14} /> {manual ? t('software.manual.change') : t('software.manual.add')}
      </button>
    {/if}
  {/if}

  {#if s.observations.length}
    <details class="how">
      <summary>{t('software.how')}</summary>
      <ul>
        {#each s.observations as o (o.source)}
          <li>
            <span>{t(`software.source.${o.source}`)}</span>
            {#if o.evidence}<code>{o.evidence}</code>{/if}
            {#if o.seenAt}<span class="faint small">{dateTime(o.seenAt)}</span>{/if}
          </li>
        {/each}
      </ul>
    </details>
  {/if}
  <p class="faint small" class:old={dataOld}>
    {dataOld ? t('software.dataOld', { d: dateLong(s.dataAsOf) }) : t('software.dataAsOf', { d: dateLong(s.dataAsOf) })}
  </p>
</div>

<style>
  .sw {
    display: grid;
    gap: 10px;
  }
  .verdict {
    display: flex;
    align-items: baseline;
    gap: 9px;
    font-size: 16px;
  }
  .mark {
    flex: none;
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--text-3);
    transform: translateY(-1px);
  }
  [data-status='outdated'] .mark {
    background: var(--danger);
  }
  [data-status='outdated'] .verdict {
    color: var(--danger);
  }
  [data-status='aging'] .mark,
  [data-status='update'] .mark {
    background: var(--amber);
  }
  [data-status='aging'] .verdict,
  [data-status='update'] .verdict {
    color: var(--warn);
  }
  [data-status='supported'] .mark {
    background: var(--ok);
  }
  [data-status='supported'] .verdict {
    color: var(--ok);
  }
  .system {
    font-weight: 500;
  }
  .why {
    color: var(--text-2);
    font-size: 14px;
    text-wrap: pretty;
  }
  .advice {
    display: grid;
    gap: 4px;
    padding: 10px 12px;
    border-radius: var(--radius-s);
    background: var(--surface-2);
    font-size: 14px;
  }
  .edition {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 14px;
  }
  .edition .seg button {
    height: 26px;
    font-size: 13px;
  }
  .sw > :global(.btn) {
    justify-self: start;
  }
  .manual {
    display: grid;
    gap: 8px;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .row .select {
    width: auto;
    min-width: 120px;
  }
  .exact {
    width: 110px;
  }
  .how summary {
    font-size: 13px;
    color: var(--text-2);
    cursor: pointer;
  }
  .how ul {
    list-style: none;
    margin: 8px 0 0;
    padding: 0;
    display: grid;
    gap: 6px;
  }
  .how li {
    display: grid;
    gap: 2px;
    font-size: 13px;
  }
  .how code {
    font-family: var(--mono);
    font-size: 12px;
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
  .small {
    font-size: 13px;
  }
  .exploited {
    display: grid;
    gap: 6px;
    padding: 10px 12px;
    border: 1px solid color-mix(in srgb, var(--danger) 30%, transparent);
    border-radius: var(--radius-s);
    background: var(--danger-soft);
    font-size: 14px;
  }
  .exploited.manual,
  .exploited.uncertain {
    border-color: color-mix(in srgb, var(--amber) 35%, transparent);
    background: var(--warn-soft);
  }
  .x-title {
    color: var(--danger);
  }
  .manual .x-title,
  .uncertain .x-title {
    color: var(--warn);
  }
  .cves {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 2px;
    font-size: 13px;
  }
  .cves a,
  .cves span:first-child {
    font-family: var(--mono);
  }
  .old {
    color: var(--warn);
  }
</style>
