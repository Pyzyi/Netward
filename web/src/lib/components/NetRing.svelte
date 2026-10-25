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
  type Label = { text: string; x: number; y: number; anchor: string };

  const layout = $derived.by(() => {
    const others = devices
      .filter((d) => !d.isGateway)
      .sort((a, b) => order.indexOf(category(a.type)) - order.indexOf(category(b.type)) || displayName(a).localeCompare(displayName(b)));
    // Rings from the inside out; the number of points per ring follows its circumference.
    const radii = [size * 0.36, size * 0.445].filter((_, i) => i === 0 || others.length > 40);
    const spacing = others.length > 120 ? 9 : 15;
    const capacity = radii.map((r) => Math.floor((2 * Math.PI * r) / spacing));
    const groups = order.map((cat) => others.filter((d) => category(d.type) === cat)).filter((g) => g.length);
    const gap = groups.length > 1 ? 1.4 : 0; // empty slots between categories
    const total = others.length + gap * groups.length;
    const points: Point[] = [];
    const labels: Label[] = [];
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
      const mid = ((startSlot + slot - 1) / 2 / total) * 2 * Math.PI - Math.PI / 2;
      const lr = radii[radii.length - 1] + 26;
      const lx = c + lr * Math.cos(mid);
      const ly = c + lr * Math.sin(mid);
      labels.push({
        text: `${t(`category.${category(g[0].type)}`)} ${g.length}`,
        x: lx,
        y: ly + 4,
        anchor: Math.abs(Math.cos(mid)) < 0.3 ? 'middle' : Math.cos(mid) > 0 ? 'start' : 'end',
      });
      slot += gap;
    }
    return { points, labels, radii };
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
        <line class="spoke" class:hot={hover?.id === p.d.id} x1={c + ((p.x - c) * inner) / p.r} y1={c + ((p.y - c) * inner) / p.r} x2={p.x} y2={p.y} />
      {/if}
    {/each}
    {#each labels ? layout.labels : [] as l (l.text)}
      <text class="cat" x={l.x} y={l.y} text-anchor={l.anchor}>{l.text}</text>
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
        <circle class="hit" cx={p.x} cy={p.y} r="11" />
        {#if p.d.new}<circle class="halo" cx={p.x} cy={p.y} r="11" />{/if}
        <circle class="dot" cx={p.x} cy={p.y} r={hover?.id === p.d.id ? 7.5 : 5.5} />
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
    opacity: 0.22;
    transition: opacity 120ms var(--ease);
  }
  .spoke.hot {
    opacity: 0.9;
    stroke: var(--signal);
  }
  .cat {
    font-family: var(--mono);
    font-size: 10px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    fill: var(--on-ink-2);
  }
  .pt {
    cursor: pointer;
  }
  .hit {
    fill: transparent;
  }
  .dot {
    fill: var(--ink);
    stroke: var(--on-ink-2);
    stroke-width: 1.5;
    transition: r 120ms var(--ease);
  }
  .pt.on .dot {
    fill: var(--signal);
    stroke: var(--signal);
  }
  .pt.fresh .dot {
    fill: var(--amber);
    stroke: var(--amber);
  }
  .halo {
    fill: none;
    stroke: var(--amber);
    stroke-width: 1.5;
    transform-box: fill-box;
    transform-origin: center;
    animation: halo 2.4s var(--ease) infinite;
  }
  @keyframes halo {
    from {
      transform: scale(0.55);
      opacity: 0.9;
    }
    to {
      transform: scale(1.5);
      opacity: 0;
    }
  }
  .core {
    position: absolute;
    left: 50%;
    top: 50%;
    width: 46%;
    transform: translate(-50%, -50%);
    display: grid;
    justify-items: center;
    gap: 6px;
    text-align: center;
    pointer-events: none;
  }
  .hub {
    display: grid;
    place-items: center;
    width: 56px;
    height: 56px;
    border-radius: 12px;
    background: var(--ink-2);
    border: 1px solid var(--ink-line);
    color: var(--on-ink);
  }
  .core-name {
    max-width: 100%;
    font-weight: 650;
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
