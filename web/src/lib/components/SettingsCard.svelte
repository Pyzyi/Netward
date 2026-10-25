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

<section class="card">
  {#if title}
    <div class="card-head">
      <div class="titles">
        <h3>{title}</h3>
        {#if lead}<p class="hint">{lead}</p>{/if}
      </div>
      {#if extra}{@render extra()}{/if}
    </div>
  {/if}
  <div class="card-body body">{@render children()}</div>
  {#if onsave}
    <footer>
      {#if error}<p class="error-text" role="alert">{error}</p>{/if}
      <span class="spacer"></span>
      <button class="btn primary" type="button" disabled={!dirty || busy} onclick={onsave}>{t('common.save')}</button>
    </footer>
  {/if}
</section>

<style>
  .titles {
    display: grid;
    gap: 3px;
  }
  .body {
    display: grid;
    gap: 18px;
    padding-top: 8px;
  }
  footer {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 16px;
    border-top: 1px solid var(--border);
  }
</style>
