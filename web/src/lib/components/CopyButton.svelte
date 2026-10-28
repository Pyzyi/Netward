<script lang="ts">
  import Icon from './Icon.svelte';
  import { t } from '../i18n.svelte';

  let { value }: { value: string } = $props();
  let copied = $state(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      // Clipboard API needs a secure context; fall back to a hidden textarea.
      const ta = document.createElement('textarea');
      ta.value = value;
      ta.setAttribute('readonly', '');
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.append(ta);
      ta.select();
      document.execCommand('copy');
      ta.remove();
    }
    copied = true;
    setTimeout(() => (copied = false), 1500);
  }
</script>

<button class="btn ghost icon small" type="button" onclick={copy} aria-label={copied ? t('common.copied') : t('common.copy')} title={copied ? t('common.copied') : t('common.copy')}>
  {#if copied}<Icon name="check" size={15} />{:else}<Icon name="copy" size={15} />{/if}
</button>
