<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import Logo from './Logo.svelte';
  import { i18n, setLang, t } from '../i18n.svelte';

  let { children }: { children: Snippet } = $props();

  // A still Netzring as the identity of the sign-in screen (decorative):
  // the same grouping, gaps and arcs as on the overview.
  const groups = [3, 6, 5, 7, 4];
  const gap = 2.6;
  const total = groups.reduce((a, b) => a + b, 0) + gap * groups.length;
  const ang = (k: number) => (k / total) * 2 * Math.PI - Math.PI / 2;
  const pt = (a: number, r: number) => `${(160 + r * Math.cos(a)).toFixed(1)},${(160 + r * Math.sin(a)).toFixed(1)}`;
  const dots: { x: number; y: number; state: string }[] = [];
  const arcs: string[] = [];
  let slot = 0;
  groups.forEach((n) => {
    const start = slot;
    for (let i = 0; i < n; i++, slot++) {
      const a = ang(slot);
      const idx = dots.length;
      dots.push({ x: 160 + 118 * Math.cos(a), y: 160 + 118 * Math.sin(a), state: idx === 9 ? 'new' : idx % 7 === 5 ? 'off' : 'on' });
    }
    const a0 = ang(start - 0.35);
    const a1 = ang(slot - 1 + 0.35);
    arcs.push(`M${pt(a0, 131)}A131,131 0 0 1 ${pt(a1, 131)}`);
    slot += gap;
  });
</script>

<div class="auth">
  <aside class="side">
    <span class="brand"><Logo size={30} tile={false} /> Netward</span>
    <div class="ring-wrap" aria-hidden="true">
    <svg class="ring" viewBox="0 0 320 320">
      <circle cx="160" cy="160" r="118" class="orbit" />
      {#each arcs as d, i (i)}<path {d} class="arc" />{/each}
      {#each dots as d, i (i)}
        {#if d.state !== 'off'}<line x1={160 + (d.x - 160) * 0.42} y1={160 + (d.y - 160) * 0.42} x2={d.x} y2={d.y} class="spoke" />{/if}
      {/each}
      {#each dots as d, i (i)}
        <circle cx={d.x} cy={d.y} r="5.5" class={d.state} />
      {/each}
    </svg>
    <span class="hub"><Icon name="duo:router" size={32} /></span>
    </div>
    <p class="facts">{t('auth.fact1')}</p>
  </aside>
  <main class="form-side">
    <div class="lang seg" role="group" aria-label="Language">
      <button type="button" aria-pressed={i18n.lang === 'de'} onclick={() => setLang('de')}>DE</button>
      <button type="button" aria-pressed={i18n.lang === 'en'} onclick={() => setLang('en')}>EN</button>
    </div>
    <div class="form-box">{@render children()}</div>
  </main>
</div>

<style>
  .auth {
    min-height: 100dvh;
    display: grid;
    grid-template-columns: minmax(360px, 44%) minmax(0, 1fr);
  }
  .side {
    display: grid;
    grid-template-rows: auto 1fr auto;
    gap: 24px;
    padding: 32px 40px;
    background: var(--ink);
    color: var(--on-ink);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    font-weight: 600;
    font-size: 18px;
  }
  .ring-wrap {
    position: relative;
    width: min(340px, 100%);
    align-self: center;
    justify-self: center;
  }
  .ring {
    display: block;
    width: 100%;
  }
  .orbit {
    fill: none;
    stroke: var(--ink-line);
  }
  .arc {
    fill: none;
    stroke: var(--on-ink-2);
    stroke-width: 2;
    stroke-linecap: round;
    opacity: 0.35;
  }
  .spoke {
    stroke: var(--on-ink-2);
    opacity: 0.18;
  }
  .on {
    fill: var(--signal);
  }
  .off {
    fill: var(--ink);
    stroke: var(--on-ink-2);
    stroke-width: 1.5;
  }
  .new {
    fill: var(--amber);
  }
  .hub {
    position: absolute;
    left: 50%;
    top: 50%;
    transform: translate(-50%, -50%);
    display: grid;
    place-items: center;
    width: 18%;
    aspect-ratio: 1;
    border-radius: 50%;
    background: var(--ink-2);
    border: 1px solid var(--ink-line);
    color: var(--on-ink);
  }
  .facts {
    max-width: 40ch;
    font-size: 13px;
    color: var(--on-ink-2);
  }
  .form-side {
    position: relative;
    display: grid;
    place-items: center;
    padding: 64px 24px 32px;
  }
  .lang {
    position: absolute;
    top: 22px;
    right: 28px;
  }
  .lang button {
    height: 26px;
    padding: 0 9px;
  }
  .form-box {
    width: min(400px, 100%);
  }
  @media (max-width: 820px) {
    .auth {
      grid-template-columns: 1fr;
      grid-template-rows: auto 1fr;
    }
    .side {
      grid-template-rows: auto;
      padding: 18px 20px;
    }
    .ring-wrap,
    .facts {
      display: none;
    }
    .form-side {
      place-items: start center;
      padding: 56px 18px 32px;
    }
    .lang {
      top: 12px;
      right: 16px;
    }
  }
</style>
