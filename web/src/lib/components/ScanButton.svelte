<script lang="ts">
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { api, ApiError } from '../api';
  import { store } from '../state.svelte';
  import { t } from '../i18n.svelte';
  import { toast } from './toast.svelte';

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
      toast(t(`err.${e instanceof ApiError ? e.code : 'network'}`), 'error');
    }
    setTimeout(() => (requested = false), 8000);
  }
</script>

<button class="btn primary" type="button" onclick={scan} disabled={scanning || demo}>
  <span class:spin={scanning} class="ico"><RefreshCw size={16} /></span>
  {scanning ? t('overview.scanning') : t('overview.scanNow')}
</button>

<style>
  .ico {
    display: inline-grid;
  }
</style>
