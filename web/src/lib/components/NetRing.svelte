<script lang="ts">
  import Icon from './Icon.svelte';
  import { category, displayName, typeIcons, type Category } from '../devices';
  import { navigate } from '../router.svelte';
  import { t } from '../i18n.svelte';
  import type { Device } from '../types';

  // The network as a ring around its router: every device is a point, grouped
  // by category. Online points are filled and linked to the centre, offline
  // points are hollow, new ones carry an amber halo.
  let { devices, size = 380, labels = true }: { devices: Device[]; size?: number; labels?: boolean } = $props();

  const order: Category[] = ['net', 'compute', 'mobile', 'media', 'home', 'peripheral', 'unknown'];
  const c = $derived(size / 2);
  const inner = $derived(size * 0.2); // spokes start outside the centre label
  const gateway = $derived(devices.find((d) => d.isGateway));

  type Point = { d: Device; x: number; y: number; r: number };
  type Label = { name: string; count: number; x: number; y: number; anchor: string; arc: string };

  const layout = $derived.by(() => {
    const others = devices
      .filter((d) => !d.isGateway)
      .sort((a, b) => order.indexOf(category(a.type)) - order.indexOf(category(b.type)) || displayName(a).localeCompare(displayName(b)));
    // Rings from the inside out; the number of points per ring follows its circumference.
    // Without labels the ring can use the room they would take.
    const outer = labels ? 0.445 : 0.47;
    const radii = [size * (labels ? 0.36 : 0.4), size * outer].filter((_, i) => i === 0 || others.length > 40);
    const spacing = others.length > 120 ? 9 : 15;
    const capacity = radii.map((r) => Math.floor((2 * Math.PI * r) / spacing));
    const groups = order.map((cat) => others.filter((d) => category(d.type) === cat)).filter((g) => g.length);
    const gap = groups.length > 1 ? 2.6 : 0; // empty slots between categories
    const total = others.length + gap * groups.length;
    const points: Point[] = [];
    const groupLabels: Label[] = [];
    let slot = 0;
    for (const g of groups) {
      const startSlot = slot;
      for (const d of g) {
        // Single ring unless it is full; then alternate rings to spread the load.
        const ring = radii.length > 1 && others.length > capacity[0] ? (points.length % 2) : 0;
        const r = radii[ring];
        const a = (slot / total) * 2 * Math.PI - Math.PI / 2;
        points.push({ d, x: c + r * Math.cos(a), y: c + r * Math.sin(a), r });
        slot += 1;
      }
      const ang = (k: number) => (k / total) * 2 * Math.PI - Math.PI / 2;
      // A group of one gets a short arc, not a stub.
      const pad = g.length === 1 ? 0.9 : 0.35;
      const a0 = ang(startSlot - pad);
      const a1 = ang(slot - 1 + pad);
      const mid = (a0 + a1) / 2;
      const ar = radii[radii.length - 1] + (labels ? 14 : 10); // arc just outside the points
      const pt = (a: number, r: number) => `${(c + r * Math.cos(a)).toFixed(1)},${(c + r * Math.sin(a)).toFixed(1)}`;
      const arc = `M${pt(a0, ar)}A${ar},${ar} 0 ${a1 - a0 > Math.PI ? 1 : 0} 1 ${pt(a1, ar)}`;
      const lr = ar + 16;
      groupLabels.push({
        name: t(`category.${category(g[0].type)}`),
        // The router in the centre belongs to its category too.
        count: g.length + (gateway && category(g[0].type) === category(gateway.type) ? 1 : 0),
        x: c + lr * Math.cos(mid),
        y: c + lr * Math.sin(mid) + 4,
        anchor: Math.abs(Math.cos(mid)) < 0.3 ? 'middle' : Math.cos(mid) > 0 ? 'start' : 'end',
        arc,
      });
      slot += gap;
    }
    // Dots and their click targets shrink when the ring gets crowded, so
    // neighbours never overlap (neighbours on one ring are `rings` slots apart).
    const rings = radii.length > 1 && others.length > capacity[0] ? 2 : 1;
    const pitch = ((2 * Math.PI * radii[0]) / Math.max(1, total)) * rings;
    const dot = Math.max(2, Math.min(5.5, pitch / 2.6));
    const hit = Math.max(dot + 1, Math.min(11, pitch / 2));
    return { points, labels: groupLabels, radii, dot, hit };
  });

  let hover = $state<Device | null>(null);
  const focus = $derived(hover ?? gateway ?? null);
  const online = $derived(devices.filter((d) => d.online).length);
</script>

<div class="ring" style:width="{size}px" style:height="{size}px">
  <svg
    width={size}
    height={size}
    viewBox="0 0 {size} {size}"
    role="img"
    aria-label={t('overview.ringLabel', { online, total: devices.length })}
  >
    {#each layout.radii as r (r)}
      <circle class="orbit" cx={c} cy={c} {r} />
    {/each}
    {#each layout.points as p (p.d.id)}
      {#if p.d.online}
        <line class="spoke" class:hot={hover?.id === p.d.id} x1={c + ((p.x - c) * inner) / p.r} y1={c + ((p.y - c) * inner) / p.r}
          x2={c + ((p.x - c) * (p.r - 8)) / p.r} y2={c + ((p.y - c) * (p.r - 8)) / p.r} />
      {/if}
    {/each}
    {#each layout.labels as l (l.name)}
      <path class="arc" d={l.arc} />
      {#if labels}<text class="cat" x={l.x} y={l.y} text-anchor={l.anchor}>{l.name} <tspan class="n">{l.count}</tspan></text>{/if}
    {/each}
    {#each layout.points as p (p.d.id)}
      <g
        class="pt"
        class:on={p.d.online}
        class:fresh={p.d.new}
        role="presentation"
        onmouseenter={() => (hover = p.d)}
        onmouseleave={() => (hover = null)}
        onclick={() => navigate(`/devices/${p.d.id}`)}
      >
        <circle class="hit" cx={p.x} cy={p.y} r={layout.hit} />
        {#if p.d.new}<circle class="halo" cx={p.x} cy={p.y} r={layout.dot * 2} />{/if}
        <circle class="mark" cx={p.x} cy={p.y} r={hover?.id === p.d.id ? layout.dot + 2 : layout.dot} />
      </g>
    {/each}
  </svg>
  <div class="core">
    <span class="hub"><Icon name={focus ? typeIcons[focus.type] : 'duo:router'} size={30} /></span>
    {#if focus}
      <span class="core-name">{displayName(focus)}</span>
      <span class="core-ip">{focus.ip}</span>
    {/if}
  </div>
</div>
{#if !labels}
  <!-- Small screens: the categories as a list under the ring, in ring order. -->
  <ul class="cats" aria-hidden="true">
    {#each layout.labels as l (l.name)}<li>{l.name} <span class="num">{l.count}</span></li>{/each}
  </ul>
{/if}

<style>
  .ring {
    position: relative;
    flex: none;
  }
  svg {
    display: block;
    overflow: visible;
  }
  .orbit {
    fill: none;
    stroke: var(--ink-line);
    stroke-width: 1;
  }
  .spoke {
    stroke: var(--on-ink-2);
    stroke-width: 1;
    opacity: 0.18;
    transition: opacity 120ms var(--ease);
  }
  .spoke.hot {
    opacity: 0.9;
    stroke: var(--signal);
  }
  .arc {
    fill: none;
    stroke: var(--on-ink-2);
    stroke-width: 2;
    stroke-linecap: round;
    opacity: 0.35;
  }
  .cat {
    font-size: 12px;
    font-weight: 500;
    fill: var(--on-ink);
  }
  .cat .n {
    font-weight: 400;
    fill: var(--on-ink-2);
  }
  .pt {
    cursor: pointer;
  }
  .hit {
    fill: transparent;
  }
  .mark {
    fill: var(--ink);
    stroke: var(--on-ink-2);
    stroke-width: 1.5;
    transition: r 120ms var(--ease);
  }
  .pt.on .mark {
    fill: var(--signal);
    stroke: var(--signal);
  }
  .pt.fresh .mark {
    fill: var(--amber);
    stroke: var(--amber);
  }
  .halo {
    fill: none;
    stroke: var(--amber);
    stroke-width: 1.5;
    opacity: 0.45;
    transform-box: fill-box;
    transform-origin: center;
    animation: halo 2.4s var(--ease) infinite;
  }
  @keyframes halo {
    50% {
      transform: scale(1.25);
      opacity: 0.15;
    }
  }
  .cats {
    list-style: none;
    margin: 4px 0 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 4px 14px;
    font-size: 12px;
    color: var(--on-ink);
  }
  .cats span {
    color: var(--on-ink-2);
  }
  /* The router icon sits exactly on the centre of the ring, where the spokes
     meet; name and address hang below it. */
  .core {
    position: absolute;
    left: 50%;
    top: 50%;
    width: 46%;
    transform: translate(-50%, -30px);
    display: grid;
    justify-items: center;
    gap: 6px;
    text-align: center;
    pointer-events: none;
  }
  .hub {
    display: grid;
    place-items: center;
    width: 60px;
    height: 60px;
    border-radius: 50%;
    background: var(--ink-2);
    border: 1px solid var(--ink-line);
    color: var(--on-ink);
  }
  /* On small rings the lower spokes pass behind the text. */
  .core-name,
  .core-ip {
    padding: 0 6px;
    border-radius: 4px;
    background: var(--ink);
  }
  .core-name {
    max-width: 100%;
    box-sizing: border-box;
    font-weight: 600;
    color: var(--on-ink);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .core-ip {
    font-family: var(--mono);
    font-size: 12px;
    color: var(--on-ink-2);
  }
</style>
