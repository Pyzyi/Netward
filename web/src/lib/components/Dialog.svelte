<script lang="ts">
  import Icon from './Icon.svelte';
  import type { Snippet } from 'svelte';
  import { t } from '../i18n.svelte';

  let {
    open = $bindable(false),
    title,
    width = 480,
    children,
    footer,
  }: { open?: boolean; title: string; width?: number; children: Snippet; footer?: Snippet } = $props();

  let el: HTMLDialogElement | undefined = $state();

  $effect(() => {
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  });
</script>

<dialog
  bind:this={el}
  style:max-width="{width}px"
  onclose={() => (open = false)}
  onclick={(e) => {
    if (e.target === el) open = false;
  }}
  aria-labelledby="dlg-title"
>
  {#if open}
    <div class="inner">
      <header>
        <h2 id="dlg-title">{title}</h2>
        <button class="btn ghost icon small" type="button" aria-label={t('common.close')} onclick={() => (open = false)}>
          <Icon name="x" size={18} />
        </button>
      </header>
      <div class="content">{@render children()}</div>
      {#if footer}<footer>{@render footer()}</footer>{/if}
    </div>
  {/if}
</dialog>

<style>
  dialog {
    width: calc(100% - 32px);
    padding: 0;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    border-top: 3px solid var(--text);
    background: var(--surface);
    color: var(--text);
    box-shadow: var(--shadow-pop);
  }
  .inner {
    display: grid;
    max-height: calc(100dvh - 48px);
    grid-template-rows: auto 1fr auto;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 16px 16px 8px 20px;
  }
  .content {
    padding: 4px 20px 20px;
    overflow: auto;
  }
  footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 14px 20px;
    border-top: 1px solid var(--border);
  }
</style>
