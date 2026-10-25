<script lang="ts">
  import { t } from '../i18n.svelte';
  let { password }: { password: string } = $props();

  // Length dominates; character variety and repetition adjust the score.
  const score = $derived.by(() => {
    const p = password;
    if (p.length < 10) return 0;
    let s = p.length >= 20 ? 3 : p.length >= 14 ? 2 : 1;
    const classes = [/[a-z]/, /[A-Z]/, /[0-9]/, /[^A-Za-z0-9]/].filter((r) => r.test(p)).length;
    if (classes >= 3) s++;
    if (new Set(p).size < p.length / 3) s--;
    return Math.max(1, Math.min(4, s));
  });
</script>

<div class="strength" aria-live="polite">
  <div class="bars" aria-hidden="true">
    {#each [1, 2, 3, 4] as i (i)}
      <span class:fill={score >= i} class="s{score}"></span>
    {/each}
  </div>
  <span class="hint">{t('strength.label')}: {t(`strength.${score}`)}</span>
</div>

<style>
  .strength {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .bars {
    display: grid;
    grid-template-columns: repeat(4, 28px);
    gap: 4px;
  }
  .bars span {
    height: 4px;
    border-radius: 2px;
    background: var(--surface-3);
  }
  .fill.s1 {
    background: var(--danger);
  }
  .fill.s2 {
    background: var(--warn);
  }
  .fill.s3,
  .fill.s4 {
    background: var(--ok);
  }
</style>
