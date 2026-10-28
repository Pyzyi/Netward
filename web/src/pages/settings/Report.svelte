<script lang="ts">
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import Icon from '../../lib/components/Icon.svelte';
  import { toast } from '../../lib/components/toast.svelte';
  import { api } from '../../lib/api';
  import { errorText } from '../../lib/errors';
  import { i18n, t } from '../../lib/i18n.svelte';
  import type { ReportContract } from '../../lib/types';

  // Evidence for the provider (docs/09, F5). The contract speeds are
  // remembered in this browser for next time; name, customer number and
  // address are only sent for the PDF and never stored anywhere.
  const day = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const yesterday = new Date(Date.now() - 86_400_000);
  let from = $state(day(new Date(Date.now() - 30 * 86_400_000)));
  let to = $state(day(yesterday));
  const maxTo = day(yesterday);
  // A report covers at most a year (the server refuses longer periods).
  const minFrom = $derived(day(new Date(new Date(`${to}T12:00:00`).getTime() - 365 * 86_400_000)));

  const empty: ReportContract = { maxDown: 0, normalDown: 0, minDown: 0, maxUp: 0, normalUp: 0, minUp: 0 };
  function loadContract(): ReportContract {
    try {
      return { ...empty, ...JSON.parse(localStorage.getItem('netward.contract') ?? '{}') };
    } catch {
      return { ...empty };
    }
  }
  let contract = $state<ReportContract>(loadContract());
  let holder = $state('');
  let customer = $state('');
  let address = $state('');
  let busy = $state<'' | 'pdf' | 'csv'>('');

  async function create(format: 'pdf' | 'csv') {
    busy = format;
    const c = Object.fromEntries(Object.entries(contract).map(([k, v]) => [k, Number(v) || 0])) as unknown as ReportContract;
    try {
      localStorage.setItem('netward.contract', JSON.stringify(c));
    } catch {
      /* private mode: just not remembered */
    }
    try {
      const { blob, name } = await api.report({ from, to, format, lang: i18n.lang, contract: c, holder, customer, address });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = name;
      document.body.append(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 10_000);
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      busy = '';
    }
  }

  const fields = [
    ['maxDown', 'report.max'],
    ['normalDown', 'report.normal'],
    ['minDown', 'report.min'],
  ] as const;
</script>

<SettingsCard title={t('report.title')} lead={t('report.lead')}>
  <div class="two">
    <div class="field">
      <label for="rf">{t('report.from')}</label>
      <input id="rf" class="input" type="date" bind:value={from} min={minFrom} max={to} />
    </div>
    <div class="field">
      <label for="rt">{t('report.to')}</label>
      <input id="rt" class="input" type="date" bind:value={to} min={from} max={maxTo} />
    </div>
  </div>

  <fieldset>
    <legend>{t('report.contract')}</legend>
    <p class="hint">{t('report.contractHint')}</p>
    <div class="grid" role="group">
      <span></span>
      {#each fields as [, label] (label)}<span class="col">{t(label)}</span>{/each}
      <span class="rowlabel">{t('report.download')}</span>
      {#each fields as [k, label] (k)}
        <input class="input num" type="number" min="0" max="100000" step="1" inputmode="decimal" bind:value={contract[k]} aria-label="{t('report.download')} {t(label)}" />
      {/each}
      <span class="rowlabel">{t('report.upload')}</span>
      {#each fields as [k, label] (k)}
        {@const up = k.replace('Down', 'Up') as keyof ReportContract}
        <input class="input num" type="number" min="0" max="100000" step="1" inputmode="decimal" bind:value={contract[up]} aria-label="{t('report.upload')} {t(label)}" />
      {/each}
    </div>
  </fieldset>

  <details>
    <summary>{t('report.personal')}</summary>
    <p class="hint">{t('report.personalHint')}</p>
    <div class="two">
      <div class="field">
        <label for="rh">{t('report.holder')}</label>
        <input id="rh" class="input" maxlength="100" autocomplete="name" bind:value={holder} />
      </div>
      <div class="field">
        <label for="rc">{t('report.customer')}</label>
        <input id="rc" class="input" maxlength="64" autocomplete="off" bind:value={customer} />
      </div>
    </div>
    <div class="field">
      <label for="ra">{t('report.address')}</label>
      <input id="ra" class="input" maxlength="200" autocomplete="street-address" bind:value={address} />
    </div>
  </details>

  <div class="actions">
    <button class="btn primary" type="button" disabled={busy !== ''} onclick={() => create('pdf')}>
      <Icon name="download-simple" size={16} /> {busy === 'pdf' ? t('report.creating') : t('report.pdf')}
    </button>
    <button class="btn" type="button" disabled={busy !== ''} onclick={() => create('csv')}>{t('report.csv')}</button>
  </div>
  <p class="notice info small">{t('report.bnetza')}</p>
</SettingsCard>

<style>
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
  }
  fieldset {
    border: 0;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 6px;
  }
  legend {
    font-weight: 600;
    padding: 0;
  }
  .grid {
    display: grid;
    grid-template-columns: auto repeat(3, minmax(0, 1fr));
    gap: 6px 10px;
    align-items: center;
    max-width: 520px;
  }
  .col {
    font-size: 12px;
    color: var(--text-2);
  }
  .rowlabel {
    font-size: 14px;
  }
  .num {
    text-align: right;
  }
  details summary {
    cursor: pointer;
    font-weight: 600;
  }
  details[open] {
    display: grid;
    gap: 10px;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
  }
  .small {
    font-size: 13px;
  }
  @media (max-width: 560px) {
    .two {
      grid-template-columns: 1fr;
    }
  }
</style>
