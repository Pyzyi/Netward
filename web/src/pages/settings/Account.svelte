<script lang="ts">
  import { errorText } from '../../lib/errors';
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import PasswordStrength from '../../lib/components/PasswordStrength.svelte';
  import { toast } from '../../lib/components/toast.svelte';
  import { api } from '../../lib/api';
  import { clock, logout, store, updateSettings } from '../../lib/state.svelte';
  import { relTime, t } from '../../lib/i18n.svelte';
  import type { Session } from '../../lib/types';

  let current = $state('');
  let next = $state('');
  let repeat = $state('');
  let error = $state('');
  let busy = $state(false);
  let sessions = $state<Session[]>([]);

  async function loadSessions() {
    sessions = await api.sessions();
  }
  void loadSessions();

  async function change(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    if (next !== repeat) {
      error = t('setup.mismatch');
      return;
    }
    busy = true;
    try {
      await api.changePassword(current, next);
      current = next = repeat = '';
      toast(t('settings.passwordChanged'));
      void loadSessions();
    } catch (err) {
      error = errorText(err);
    } finally {
      busy = false;
    }
  }

  async function revoke(id: number) {
    await api.revokeSession(id);
    void loadSessions();
  }

  function agent(ua: string): string {
    const browser = /Edg\//.test(ua) ? 'Edge' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : '';
    const os = /iPhone|iPad/.test(ua) ? 'iOS' : /Android/.test(ua) ? 'Android' : /Mac OS X/.test(ua) ? 'macOS' : /Windows/.test(ua) ? 'Windows' : /Linux/.test(ua) ? 'Linux' : '';
    return [browser, os].filter(Boolean).join(' · ') || ua.slice(0, 40) || '–';
  }

  let schedule = $state(store.settings!.securityScanDays);
  async function saveSchedule() {
    const err = await updateSettings((x) => (x.securityScanDays = Number(schedule)));
    toast(err ? t('err.invalid_settings', { msg: err }) : t('common.saved'), err ? 'error' : 'ok');
  }
</script>

<SettingsCard title={t('settings.password')}>
  <form class="stack" onsubmit={change}>
    <div class="field">
      <label for="cur">{t('settings.passwordCurrent')}</label>
      <input id="cur" class="input" type="password" bind:value={current} autocomplete="current-password" required />
    </div>
    <div class="field">
      <label for="new">{t('settings.passwordNew')}</label>
      <input id="new" class="input" type="password" bind:value={next} autocomplete="new-password" required minlength="10" />
      <PasswordStrength password={next} />
    </div>
    <div class="field">
      <label for="rep">{t('setup.passwordRepeat')}</label>
      <input id="rep" class="input" type="password" bind:value={repeat} autocomplete="new-password" required />
    </div>
    {#if error}<p class="error-text" role="alert">{error}</p>{/if}
    <div><button class="btn primary" type="submit" disabled={busy || !current || next.length < 10}>{t('settings.password')}</button></div>
  </form>
</SettingsCard>

<SettingsCard title={t('settings.sessions')}>
  <ul class="sessions">
    {#each sessions as s (s.id)}
      <li>
        <div class="s-text">
          <strong>{agent(s.userAgent)}</strong>
          <span class="muted small">{t('settings.sessionMeta', { t: relTime(s.lastSeenAt, clock.now), ip: s.ip })}</span>
        </div>
        {#if s.current}
          <span class="s-current">
            <span class="badge accent">{t('settings.sessionCurrent')}</span>
            <button class="btn small" type="button" onclick={logout}>{t('nav.logout')}</button>
          </span>
        {:else}
          <button class="btn small" type="button" onclick={() => revoke(s.id)}>{t('settings.sessionRevoke')}</button>
        {/if}
      </li>
    {/each}
  </ul>
</SettingsCard>

<SettingsCard title={t('settings.securitySchedule')}>
  <div class="field narrow">
    <select class="select" bind:value={schedule} onchange={saveSchedule} aria-label={t('settings.securitySchedule')}>
      <option value={0}>{t('settings.schedule.off')}</option>
      <option value={1}>{t('settings.schedule.daily')}</option>
      <option value={7}>{t('settings.schedule.weekly')}</option>
    </select>
  </div>
</SettingsCard>

<style>
  .s-current {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .sessions {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 8px;
  }
  .sessions li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  .s-text {
    display: grid;
    gap: 2px;
  }
  .small {
    font-size: 12.5px;
  }
  .narrow {
    max-width: 280px;
  }
</style>
