<script lang="ts">
  import AuthCard from '../lib/components/AuthCard.svelte';
  import { api, ApiError } from '../lib/api';
  import { applyAuth } from '../lib/state.svelte';
  import { t } from '../lib/i18n.svelte';

  let username = $state('');
  let password = $state('');
  let remember = $state(true);
  let busy = $state(false);
  let error = $state('');
  let showHelp = $state(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = '';
    try {
      const st = await api.login(username.trim(), password, remember);
      applyAuth({ ...st, setupRequired: false, setupAllowed: false, setupTokenRequired: false });
    } catch (err) {
      const code = err instanceof ApiError ? err.code : 'network';
      error = t(`err.${code}`, { s: err instanceof ApiError ? err.retryAfter : 0 });
      password = '';
    } finally {
      busy = false;
    }
  }
</script>

<AuthCard>
  <form onsubmit={submit} class="stack">
    <div class="head">
      <h1>{t('login.title')}</h1>
      <p class="muted">{t('login.lead')}</p>
    </div>
    <div class="field">
      <label for="u">{t('setup.username')}</label>
      <!-- svelte-ignore a11y_autofocus -->
      <input id="u" class="input" bind:value={username} autocomplete="username" required autofocus />
    </div>
    <div class="field">
      <label for="p">{t('setup.password')}</label>
      <input id="p" class="input" type="password" bind:value={password} autocomplete="current-password" required />
    </div>
    <label class="check"><input type="checkbox" bind:checked={remember} /> {t('login.remember')}</label>
    {#if error}<p class="error-text" role="alert">{error}</p>{/if}
    <button class="btn primary" type="submit" disabled={busy || !username || !password}>{t('login.submit')}</button>
    <button class="linkish" type="button" onclick={() => (showHelp = !showHelp)} aria-expanded={showHelp}>{t('login.forgot')}</button>
    {#if showHelp}<p class="hint">{t('login.forgotHelp')}</p>{/if}
  </form>
</AuthCard>

<style>
  .head {
    display: grid;
    gap: 6px;
    margin-bottom: 4px;
  }
  h1 {
    font-size: 28px;
  }
  .linkish {
    justify-self: start;
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent);
    cursor: pointer;
    font-size: 13px;
  }
</style>
