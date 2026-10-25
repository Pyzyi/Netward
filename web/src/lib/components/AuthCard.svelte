<script lang="ts">
  import type { Snippet } from 'svelte';
  import Logo from './Logo.svelte';
  import { i18n, setLang } from '../i18n.svelte';

  let { children }: { children: Snippet } = $props();
</script>

<div class="auth">
  <div class="top">
    <span class="brand"><Logo size={30} /> Netzwart</span>
    <div class="seg" role="group" aria-label="Language">
      <button type="button" aria-pressed={i18n.lang === 'de'} onclick={() => setLang('de')}>DE</button>
      <button type="button" aria-pressed={i18n.lang === 'en'} onclick={() => setLang('en')}>EN</button>
    </div>
  </div>
  <div class="card panel">{@render children()}</div>
</div>

<style>
  .auth {
    min-height: 100dvh;
    display: grid;
    align-content: center;
    justify-items: center;
    gap: 22px;
    padding: 32px 16px;
  }
  .top {
    width: min(420px, 100%);
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    font-weight: 650;
    font-size: 17px;
  }
  .panel {
    width: min(420px, 100%);
    padding: 28px;
  }
  @media (max-width: 480px) {
    .panel {
      padding: 22px 18px;
    }
  }
</style>
