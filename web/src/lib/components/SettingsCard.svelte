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
      {#if dirty || busy}
        <button class="btn primary" type="button" disabled={busy} onclick={onsave}>{t('common.save')}</button>
      {:else}
        <span class="unchanged">{t('common.noChanges')}</span>
      {/if}
      {#if error}<p class="error-text" role="alert">{error}</p>{/if}
    </footer>
  {/if}
</section>

<style>
  .block {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .block-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    padding: 16px 20px 0;
  }
  .titles {
    display: grid;
    gap: 2px;
    max-width: 64ch;
  }
  .block-title {
    font-size: 15px;
    font-weight: 600;
  }
  .body {
    display: grid;
    gap: 18px;
    padding: 16px 20px 20px;
  }
  footer {
    display: flex;
    align-items: center;
    gap: 14px;
    min-height: 52px;
    padding: 10px 20px;
    border-top: 1px solid var(--border);
  }
  .unchanged {
    font-size: 13px;
    color: var(--text-2);
  }
  @media (max-width: 640px) {
    .block-head {
      padding: 14px 16px 0;
    }
    .body {
      padding: 14px 16px 16px;
    }
    footer {
      padding: 10px 16px;
    }
  }
</style>
