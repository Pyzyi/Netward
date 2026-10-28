<script lang="ts">
  import Icon from '../lib/components/Icon.svelte';
  import Dialog from '../lib/components/Dialog.svelte';
  import { toast } from '../lib/components/toast.svelte';
  import { api, ApiError } from '../lib/api';
  import { errorText } from '../lib/errors';
  import { clock, store } from '../lib/state.svelte';
  import { link, navigate, router } from '../lib/router.svelte';
  import { displayName } from '../lib/devices';
  import { number, relTime, t } from '../lib/i18n.svelte';
  import { hintText, level, mbps, measure, preparePlan, rgb, type Phase } from '../lib/wifimap';
  import type { FloorPlan, WifiMapView, WifiPoint } from '../lib/types';

  let plans = $state<FloorPlan[] | null>(null);
  let view = $state<WifiMapView>({ points: [], hints: [], refMbps: 0 });
  let failed = $state(false);
  const planId = $derived(Number(router.query.get('plan')) || plans?.[0]?.id || 0);
  const plan = $derived(plans?.find((p) => p.id === planId));

  async function loadPlans() {
    try {
      plans = await api.floorPlans();
      failed = false;
    } catch {
      failed = true;
    }
  }
  void loadPlans();
  $effect(() => {
    const id = plan?.id;
    if (!id) return;
    api.wifiPoints(id).then((v) => (view = v), (e) => toast(errorText(e), 'error'));
  });

  // Upload
  let uploadName = $state('');
  let uploading = $state(false);
  let fileInput: HTMLInputElement | undefined = $state();
  async function upload(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    uploading = true;
    try {
      const img = await preparePlan(file);
      const p = await api.addFloorPlan(uploadName.trim() || t('wifimap.defaultName'), img);
      uploadName = '';
      await loadPlans();
      navigate(`/wlan/map?plan=${p.id}`, { replace: true });
    } catch (err) {
      toast(err instanceof ApiError ? errorText(err) : t('wifimap.badImage'), 'error');
    } finally {
      uploading = false;
      if (fileInput) fileInput.value = '';
    }
  }

  // Placing and measuring
  let pending = $state<{ x: number; y: number } | null>(null);
  let room = $state('');
  let phase = $state<Phase | null>(null);
  let progress = $state(0);
  let last = $state<WifiPoint | null>(null);
  let abort: AbortController | null = null;
  const rooms = $derived([...new Set(view.points.map((p) => p.room).filter(Boolean))].sort());

  function place(e: MouseEvent) {
    if (phase) return;
    const box = (e.currentTarget as HTMLElement).getBoundingClientRect();
    pending = { x: Math.min(1, Math.max(0, (e.clientX - box.left) / box.width)), y: Math.min(1, Math.max(0, (e.clientY - box.top) / box.height)) };
    last = null;
  }
  async function run() {
    if (!pending || !plan) return;
    abort = new AbortController();
    try {
      const m = await measure(abort.signal, (p, f) => {
        phase = p;
        progress = f;
      });
      const before = new Set(view.points.map((p) => p.id));
      view = await api.addWifiPoint(plan.id, { ...pending, room: room.trim(), ...m });
      last = view.points.find((p) => !before.has(p.id)) ?? null;
      pending = null;
      void loadPlans();
    } catch (e) {
      if (!abort.signal.aborted) toast(e instanceof ApiError && e.code === 'busy' ? t('wifimap.busy') : errorText(e), 'error');
      if (e instanceof ApiError && e.code === 'measure_local') pending = null;
    } finally {
      phase = null;
      abort = null;
    }
  }
  function cancel() {
    abort?.abort();
    pending = null;
  }
  async function removePoint(p: WifiPoint) {
    if (!plan) return;
    try {
      view = await api.deleteWifiPoint(plan.id, p.id);
      if (last?.id === p.id) last = null;
      void loadPlans();
    } catch (e) {
      toast(errorText(e), 'error');
    }
  }

  // Rename / delete
  let renameOpen = $state(false);
  let newName = $state('');
  let deleteOpen = $state(false);
  async function rename() {
    if (!plan) return;
    try {
      await api.renameFloorPlan(plan.id, newName.trim());
      renameOpen = false;
      await loadPlans();
    } catch (e) {
      toast(errorText(e), 'error');
    }
  }
  async function remove() {
    if (!plan) return;
    try {
      await api.deleteFloorPlan(plan.id);
      deleteOpen = false;
      view = { points: [], hints: [], refMbps: 0 };
      await loadPlans();
      navigate('/wlan/map', { replace: true });
    } catch (e) {
      toast(errorText(e), 'error');
    }
  }

  // Heatmap: inverse distance weighting of each point's share of the best
  // speed, faded out away from the measurements. Drawn small; the browser
  // scales it up smoothly.
  const best = $derived(view.refMbps);
  const share = (p: WifiPoint) => (best > 0 ? p.downMbps / best : 0);
  let canvas: HTMLCanvasElement | undefined = $state();
  $effect(() => {
    if (!canvas || !plan) return;
    const W = 160;
    const H = Math.max(1, Math.round((W * plan.height) / plan.width));
    canvas.width = W;
    canvas.height = H;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    ctx.clearRect(0, 0, W, H);
    const pts = view.points.map((p) => ({ x: p.x * W, y: p.y * H, s: Math.min(1, share(p)) }));
    if (!pts.length) return;
    const reach = 0.28 * Math.max(W, H);
    const img = ctx.createImageData(W, H);
    for (let y = 0; y < H; y++) {
      for (let x = 0; x < W; x++) {
        let sw = 0;
        let sv = 0;
        let dmin = Infinity;
        for (const p of pts) {
          const d = Math.hypot(x - p.x, y - p.y);
          dmin = Math.min(dmin, d);
          const w = 1 / (d * d + 1);
          sw += w;
          sv += w * p.s;
        }
        const a = Math.max(0, 1 - dmin / reach);
        if (a <= 0) continue;
        const [r, g, b] = rgb(sv / sw);
        const i = (y * W + x) * 4;
        img.data[i] = r;
        img.data[i + 1] = g;
        img.data[i + 2] = b;
        img.data[i + 3] = Math.round(150 * a);
      }
    }
    ctx.putImageData(img, 0, 0);
  });

  const deviceName = (id?: number) => {
    const d = id ? store.devices.get(id) : undefined;
    return d ? displayName(d) : '';
  };
</script>

<div class="wrap">
  <header class="page-head">
    <div>
      <a class="crumb" href="/wlan" use:link><Icon name="caret-left" size={14} /> {t('wlan.title')}</a>
      <h1>{t('wifimap.title')}</h1>
      <p class="lead">{t('wifimap.lead')}</p>
    </div>
    {#if plans && plans.length > 1}
      <select class="select" aria-label={t('wifimap.choose')} value={planId}
        onchange={(e) => navigate(`/wlan/map?plan=${e.currentTarget.value}`, { replace: true })}>
        {#each plans as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
      </select>
    {/if}
  </header>

  {#if !plans}
    {#if failed}<p class="notice danger">{t('common.error')}</p>{:else}<div class="skeleton" style:height="240px"></div>{/if}
  {:else if !plan}
    <section class="section start">
      <h2>{t('wifimap.start.title')}</h2>
      <ol class="steps">
        <li>{t('wifimap.start.step1')}</li>
        <li>{t('wifimap.start.step2')}</li>
        <li>{t('wifimap.start.step3')}</li>
      </ol>
      {@render uploader()}
      <p class="hint">{t('wifimap.note')}</p>
    </section>
  {:else}
    <div class="layout">
      <section class="section map-panel" aria-labelledby="plan-title">
        <header class="map-head">
          <h2 id="plan-title">{plan.name}</h2>
          <div class="row-btns">
            <button class="btn small ghost" type="button" onclick={() => { newName = plan.name; renameOpen = true; }}>
              <Icon name="pencil-simple" size={14} /> {t('wifimap.rename')}</button>
            <button class="btn small ghost" type="button" onclick={() => (deleteOpen = true)}>
              <Icon name="trash" size={14} /> {t('wifimap.delete')}</button>
          </div>
        </header>
        <p class="hint">{pending ? t('wifimap.placed') : t('wifimap.tapHint')}</p>
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
        <div class="map" style:aspect-ratio="{plan.width} / {plan.height}" onclick={place}>
          <img src="/api/v1/wifimap/plans/{plan.id}/image?v={plan.createdAt}" alt={t('wifimap.planAlt', { name: plan.name })} draggable="false" />
          <canvas bind:this={canvas} aria-hidden="true"></canvas>
          {#each view.points as p (p.id)}
            <span class="pt {level(share(p))}" class:new={last?.id === p.id} style:left="{p.x * 100}%" style:top="{p.y * 100}%"
              title="{p.room || t('wifimap.unnamed')}: {mbps(p.downMbps)} Mbit/s">{mbps(p.downMbps)}</span>
          {/each}
          {#if pending}
            <span class="pin" style:left="{pending.x * 100}%" style:top="{pending.y * 100}%" aria-hidden="true"></span>
          {/if}
        </div>
        {#if view.points.length}
          <div class="legend" aria-hidden="true">
            <span class="scale"></span>
            <span class="faint small">{t('wifimap.legend', { best: mbps(best) })}</span>
          </div>
        {/if}

        {#if pending}
          <div class="measure" role="group" aria-label={t('wifimap.measureHere')}>
            <div class="field">
              <label for="room">{t('wifimap.room')}</label>
              <input id="room" class="input" list="rooms" maxlength="40" autocomplete="off" placeholder={t('wifimap.roomPlaceholder')}
                bind:value={room} disabled={!!phase} />
              <datalist id="rooms">{#each rooms as r (r)}<option value={r}></option>{/each}</datalist>
            </div>
            {#if phase}
              <div class="progress" role="status">
                <span>{t(`wifimap.phase.${phase}`)}</span>
                <span class="bar"><span style:width="{Math.round(((phase === 'latency' ? 0 : phase === 'download' ? 1 : 2) + progress) / 3 * 100)}%"></span></span>
              </div>
            {/if}
            <div class="row-btns">
              <button class="btn primary" type="button" onclick={run} disabled={!!phase}>
                <Icon name="gauge" size={16} /> {t('wifimap.measureHere')}</button>
              <button class="btn" type="button" onclick={cancel}>{t('common.cancel')}</button>
            </div>
          </div>
        {:else if last}
          <p class="result" role="status">
            {t('wifimap.result', { room: last.room || t('wifimap.unnamed'), down: mbps(last.downMbps), up: mbps(last.upMbps), latency: Math.round(last.latencyMs) })}
          </p>
        {/if}
      </section>

      <div class="side">
        <section class="section" aria-labelledby="hints-title">
          <h2 id="hints-title">{t('wifimap.hints')}</h2>
          {#if view.points.length < 2}
            <p class="muted">{t('wifimap.hintsEmpty')}</p>
          {:else}
            <ul class="hints">
              {#each view.hints as h, i (i)}
                <li class={h.kind === 'good' ? 'good' : h.severe ? 'bad' : 'weak'}>{hintText(h)}</li>
              {/each}
            </ul>
          {/if}
          <p class="hint">{t('wifimap.note')}</p>
        </section>

        {#if view.points.length}
          <section class="section" aria-labelledby="pts-title">
            <h2 id="pts-title"><span class="count-label">{t('wifimap.points')} <span class="count">{view.points.length}</span></span></h2>
            <ul class="rows pts">
              {#each [...view.points].reverse() as p (p.id)}
                <li>
                  <span class="dot {level(share(p)) === 'good' ? 'ok' : level(share(p)) === 'fair' ? 'warn' : 'danger'}" aria-hidden="true"></span>
                  <div class="pt-main">
                    <strong>{p.room || t('wifimap.unnamed')}</strong>
                    <span class="num">{t('wifimap.values', { down: mbps(p.downMbps), up: mbps(p.upMbps), latency: Math.round(p.latencyMs) })}</span>
                    <span class="faint small">
                      {[relTime(p.ts, clock.now), deviceName(p.deviceId), p.band ? `${p.band} GHz` : '', p.signalDbm ? `${p.signalDbm} dBm` : '', p.ap ?? ''].filter(Boolean).join(' · ')}
                    </span>
                  </div>
                  <button class="btn small ghost icon-only" type="button" aria-label={t('wifimap.deletePoint', { room: p.room || t('wifimap.unnamed') })}
                    onclick={() => removePoint(p)}><Icon name="trash" size={15} /></button>
                </li>
              {/each}
            </ul>
          </section>
        {/if}

        {#if plans.length < 10}
          <section class="section">
            <h2>{t('wifimap.addPlan')}</h2>
            {@render uploader()}
          </section>
        {/if}
      </div>
    </div>
  {/if}
</div>

{#snippet uploader()}
  <div class="upload">
    <div class="field">
      <label for="plan-name">{t('wifimap.planName')}</label>
      <input id="plan-name" class="input" maxlength="40" placeholder={t('wifimap.defaultName')} bind:value={uploadName} />
    </div>
    <label class="btn primary file" class:busy={uploading}>
      <Icon name="upload-simple" size={16} /> {uploading ? t('wifimap.uploading') : t('wifimap.upload')}
      <input bind:this={fileInput} type="file" accept="image/*" onchange={upload} disabled={uploading} />
    </label>
  </div>
{/snippet}

<Dialog bind:open={renameOpen} title={t('wifimap.rename')}>
  <div class="field">
    <label for="rename">{t('wifimap.planName')}</label>
    <input id="rename" class="input" maxlength="40" bind:value={newName} />
  </div>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (renameOpen = false)}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" disabled={!newName.trim()} onclick={rename}>{t('common.save')}</button>
  {/snippet}
</Dialog>

<Dialog bind:open={deleteOpen} title={t('wifimap.deleteTitle', { name: plan?.name ?? '' })}>
  <p>{t('wifimap.deleteText', { n: number(plan?.points ?? 0) })}</p>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (deleteOpen = false)}>{t('common.cancel')}</button>
    <button class="btn danger" type="button" onclick={remove}>{t('wifimap.delete')}</button>
  {/snippet}
</Dialog>

<style>
  .layout {
    display: grid;
    grid-template-columns: minmax(0, 1.6fr) minmax(280px, 1fr);
    gap: 16px;
    align-items: start;
  }
  .side {
    display: grid;
    gap: 16px;
  }
  .section h2 {
    font-size: 15px;
    margin: 0 0 8px;
  }
  .start {
    max-width: 640px;
  }
  .steps {
    margin: 0 0 16px;
    padding-left: 20px;
    display: grid;
    gap: 6px;
  }
  .map-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .map-head h2 {
    margin: 0;
  }
  .map {
    position: relative;
    width: 100%;
    margin-top: 8px;
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
    background: #fff;
    cursor: crosshair;
    touch-action: manipulation;
    user-select: none;
  }
  .map img,
  .map canvas {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
  }
  .map img {
    object-fit: fill;
  }
  .pt {
    position: absolute;
    transform: translate(-50%, -50%);
    min-width: 30px;
    height: 22px;
    padding: 0 6px;
    border-radius: 11px;
    display: inline-grid;
    place-items: center;
    font-size: 12px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    color: #fff;
    border: 2px solid #fff;
    box-shadow: 0 1px 3px rgb(0 0 0 / 0.3);
    pointer-events: none;
  }
  .pt.good {
    background: #17734a;
  }
  .pt.fair {
    background: #8f5a00;
  }
  .pt.weak {
    background: #b4322a;
  }
  .pt.new {
    outline: 3px solid var(--accent);
  }
  .pin {
    position: absolute;
    width: 18px;
    height: 18px;
    transform: translate(-50%, -50%);
    border-radius: 50%;
    background: var(--accent);
    border: 3px solid #fff;
    box-shadow: 0 0 0 6px rgb(40 86 199 / 0.25);
    pointer-events: none;
  }
  .legend {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 8px;
  }
  .scale {
    width: 96px;
    height: 8px;
    border-radius: 4px;
    background: linear-gradient(90deg, rgb(196 52 44), rgb(226 155 18) 40%, rgb(31 166 106) 75%);
  }
  .measure {
    display: grid;
    gap: 12px;
    margin-top: 14px;
    padding-top: 14px;
    border-top: 1px solid var(--border);
  }
  .progress {
    display: grid;
    gap: 6px;
    font-size: 14px;
  }
  .bar {
    height: 6px;
    border-radius: 3px;
    background: var(--surface-2, var(--border));
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    background: var(--accent);
    transition: width 0.2s;
  }
  .result {
    margin: 14px 0 0;
    padding: 10px 12px;
    border-radius: 8px;
    background: var(--ok-soft);
    color: var(--text);
  }
  .hints {
    list-style: none;
    margin: 0 0 8px;
    padding: 0;
    display: grid;
    gap: 8px;
  }
  .hints li {
    padding: 10px 12px;
    border-radius: 8px;
  }
  .hints .good {
    background: var(--ok-soft);
  }
  .hints .weak {
    background: var(--warn-soft);
  }
  .hints .bad {
    background: var(--danger-soft);
  }
  .pts li {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .pt-main {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 1px;
  }
  .upload {
    display: flex;
    gap: 12px;
    align-items: end;
    flex-wrap: wrap;
  }
  .upload .field {
    flex: 1 1 200px;
    margin: 0;
  }
  .file {
    position: relative;
    overflow: hidden;
    cursor: pointer;
  }
  .file input {
    position: absolute;
    inset: 0;
    opacity: 0;
    cursor: pointer;
  }
  .file:focus-within {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
  .file.busy {
    opacity: 0.7;
    pointer-events: none;
  }
  @media (max-width: 900px) {
    .layout {
      grid-template-columns: 1fr;
    }
  }
</style>
