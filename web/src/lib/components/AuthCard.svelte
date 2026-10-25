<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import Logo from './Logo.svelte';
  import { i18n, setLang, t } from '../i18n.svelte';

  let { children }: { children: Snippet } = $props();

  // A still Netzring as the identity of the sign-in screen (decorative).
  const dots = Array.from({ length: 26 }, (_, i) => {
    const a = (i / 26) * 2 * Math.PI - Math.PI / 2;
    return { x: 160 + 118 * Math.cos(a), y: 160 + 118 * Math.sin(a), state: i === 7 ? 'new' : i % 6 === 4 ? 'off' : 'on' };
  });
</script>

<div class="auth">
  <aside class="side">
    <span class="brand"><Logo size={30} tile={false} /> Netzwart</span>
    <div class="ring-wrap" aria-hidden="true">
    <svg class="ring" viewBox="0 0 320 320">
      <circle cx="160" cy="160" r="118" class="orbit" />
      {#each dots as d, i (i)}
        {#if d.state !== 'off'}<line x1={160 + (d.x - 160) * 0.42} y1={160 + (d.y - 160) * 0.42} x2={d.x} y2={d.y} class="spoke" />{/if}
      {/each}
      {#each dots as d, i (i)}
        <circle cx={d.x} cy={d.y} r="5.5" class={d.state} />
      {/each}
    </svg>
    <span class="hub"><Icon name="duo:router" size={32} /></span>
    </div>
    <ul class="facts">
      <li>{t('auth.fact1')}</li>
      <li>{t('auth.fact2')}</li>
      <li>{t('auth.fact3')}</li>
    </ul>
  </aside>
  <main class="form-side">
    <div class="lang seg" role="group" aria-label="Language">
      <button type="button" aria-pressed={i18n.lang === 'de'} onclick={() => setLang('de')}>DE</button>
      <button type="button" aria-pressed={i18n.lang === 'en'} onclick={() => setLang('en')}>EN</button>
    </div>
    <div class="panel">{@render children()}</div>
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
    font-weight: 800;
    font-size: 20px;
    letter-spacing: -0.02em;
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
  .spoke {
    stroke: var(--on-ink-2);
    opacity: 0.22;
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
    border-radius: 12px;
    background: var(--ink-2);
    border: 1px solid var(--ink-line);
    color: var(--on-ink);
  }
  .facts {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 8px;
    font-family: var(--mono);
    font-size: 13px;
    color: var(--on-ink-2);
  }
  .facts li::before {
    content: '— ';
    color: var(--signal);
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
    height: 30px;
    padding: 0 8px;
  }
  .panel {
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
