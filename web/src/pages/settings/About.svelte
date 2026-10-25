<script lang="ts">
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import Logo from '../../lib/components/Logo.svelte';
  import { api } from '../../lib/api';
  import { clock } from '../../lib/state.svelte';
  import { bytes, duration, t } from '../../lib/i18n.svelte';
  import type { SystemInfo } from '../../lib/types';

  let info = $state<SystemInfo | null>(null);
  void api.system().then((i) => (info = i));
</script>

<SettingsCard>
  <div class="brand"><Logo size={40} /><div><strong>Netzwart</strong><p class="muted">{info?.version ?? ''}</p></div></div>
  {#if info}
    <dl class="dl">
      <dt>{t('settings.about.version')}</dt><dd class="mono">{info.version} ({info.goVersion})</dd>
      <dt>{t('settings.about.mode')}</dt><dd>{t(`settings.mode.${info.mode}`)}</dd>
      <dt>{t('settings.about.user')}</dt><dd class="mono">UID {info.uid}</dd>
      <dt>{t('settings.about.uptime')}</dt><dd>{duration(clock.now - info.startedAt)}</dd>
      <dt>{t('settings.about.db')}</dt><dd>{bytes(info.dbSize)}</dd>
      <dt>{t('settings.about.memory')}</dt><dd>{bytes(info.memory)}</dd>
    </dl>
  {/if}
  <p class="hint">{t('settings.about.license')}</p>
</SettingsCard>

<style>
  .brand {
    display: flex;
    align-items: center;
    gap: 14px;
  }
</style>
