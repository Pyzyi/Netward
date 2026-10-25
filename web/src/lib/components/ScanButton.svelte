<script lang="ts">
  import Icon from './Icon.svelte';
  import { errorText } from '../errors';
  import { api } from '../api';
  import { store } from '../state.svelte';
  import { t } from '../i18n.svelte';
  import { toast } from './toast.svelte';

  // onInk: light button for the dark stage of the overview.
  let { onInk = false }: { onInk?: boolean } = $props();
  let requested = $state(false);
  const scanning = $derived(store.overview?.scanner.scanning || requested);
  const demo = $derived(store.overview?.scanner.mode === 'demo');

  $effect(() => {
    if (store.overview?.scanner.scanning) requested = false;
  });

  async function scan() {
    requested = true;
    try {
      await api.scan();
    } catch (e) {
      requested = false;
      toast(errorText(e), 'error');
    }
    setTimeout(() => (requested = false), 8000);
  }
</script>

<button class="btn primary" class:on-ink={onInk} type="button" onclick={scan} disabled={scanning || demo} title={demo ? t('err.demo_mode') : undefined}>
  <span class:spin={scanning} class="ico"><Icon name="arrows-clockwise" size={16} /></span>
  {scanning ? t('overview.scanning') : t('overview.scanNow')}
</button>

<style>
  .ico {
    display: inline-grid;
  }
  .on-ink {
    background: var(--on-ink);
    border-color: var(--on-ink);
    color: var(--ink);
  }
  .on-ink:hover:not(:disabled) {
    background: #fff;
    border-color: #fff;
  }
  .on-ink:disabled {
    background: transparent;
    border: 1px dashed var(--on-ink-2);
    color: var(--on-ink-2);
  }
</style>
