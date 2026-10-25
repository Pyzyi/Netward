<script lang="ts">
  import type { Snippet } from 'svelte';
  import { t } from '../i18n.svelte';

  let {
    title = '',
    lead = '',
    dirty = false,
    busy = false,
    error = '',
    onsave,
    children,
    extra,
  }: {
    title?: string; lead?: string; dirty?: boolean; busy?: boolean; error?: string;
    onsave?: () => void; children: Snippet; extra?: Snippet;
  } = $props();
</script>

<section class="block">
  {#if title}
    <div class="block-head">
      <div class="titles">
        <h3 class="block-title">{title}</h3>
        {#if lead}<p class="hint">{lead}</p>{/if}
      </div>
      {#if extra}{@render extra()}{/if}
    </div>
  {/if}
  <div class="body">{@render children()}</div>
  {#if onsave}
    <footer>
      {#if error}<p class="error-text" role="alert">{error}</p>{/if}
      <span class="spacer"></span>
      <button class="btn primary" type="button" disabled={!dirty || busy} onclick={onsave}>{t('common.save')}</button>
    </footer>
  {/if}
</section>

<style>
  .block {
    border-top: 1px solid var(--text);
  }
  .block-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    padding: 12px 0 4px;
  }
  .titles {
    display: grid;
    gap: 4px;
    max-width: 64ch;
  }
  .block-title {
    font-size: 17px;
    font-weight: 700;
    letter-spacing: -0.015em;
  }
  .body {
    display: grid;
    gap: 18px;
    padding: 14px 0 4px;
  }
  footer {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 16px 0 0;
  }
</style>
