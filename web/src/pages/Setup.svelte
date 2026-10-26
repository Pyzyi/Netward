<script lang="ts">
  import Icon from '../lib/components/Icon.svelte';
  import AuthCard from '../lib/components/AuthCard.svelte';
  import PasswordStrength from '../lib/components/PasswordStrength.svelte';
  import { api, ApiError } from '../lib/api';
  import { applyAuth, auth, loadAuth } from '../lib/state.svelte';
  import { toast } from '../lib/components/toast.svelte';
  import { i18n, t } from '../lib/i18n.svelte';

  let step = $state(1);
  let username = $state('admin');
  let password = $state('');
  let repeat = $state('');
  let token = $state('');
  let networkName = $state('');
  let busy = $state(false);
  let error = $state('');
  let pwError = $state('');
  let showPw = $state(false);
  let tried = $state(false); // after a submit, an empty repeat field counts as a mismatch

  const st = $derived(auth.state);
  const mismatch = $derived((tried || repeat.length > 0) && repeat !== password);

  function next(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    pwError = '';
    tried = true;
    if (password.length < 10) {
      pwError = t('err.password_too_short');
      document.getElementById('p')?.focus();
      return;
    }
    if (password !== repeat) {
      document.getElementById('r')?.focus(); // the field shows the mismatch
      return;
    }
    step = 2;
  }

  async function finish(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = '';
    try {
      const res = await api.setup({
        username: username.trim(),
        password,
        token,
        language: i18n.lang,
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone ?? 'UTC',
        networkName: networkName.trim(),
      });
      applyAuth({ ...res, setupRequired: false, setupAllowed: false, setupTokenRequired: false });
    } catch (err) {
      const code = err instanceof ApiError ? err.code : 'network';
      if (code === 'already_setup') {
        // Someone finished setup in the meantime: show the login instead.
        toast(t('err.already_setup'), 'info', 6000);
        await loadAuth();
        return;
      }
      error = t(`err.${code}`, { s: err instanceof ApiError ? err.retryAfter : 0 });
      if (code.startsWith('password') || code === 'invalid_username' || code === 'setup_token') step = 1;
    } finally {
      busy = false;
    }
  }
</script>

<AuthCard>
  {#if st && !st.setupAllowed}
    <div class="blocked">
      <span class="lock"><Icon name="lock-simple" size={22} /></span>
      <h1>{t('setup.blockedTitle')}</h1>
      <p class="muted">{st.setupReason === 'expired' ? t('setup.expired') : t('setup.notLocal')}</p>
    </div>
  {:else if step === 1}
    <form onsubmit={next} class="stack" novalidate>
      <div class="head">
        <span class="step">{t('setup.step', { n: 1 })}</span>
        <h1>{t('setup.title')}</h1>
        <p class="muted">{t('setup.lead')}</p>
      </div>
      <div class="field">
        <label for="u">{t('setup.username')}</label>
        <input id="u" class="input" bind:value={username} autocomplete="username" required minlength="2" maxlength="32" />
      </div>
      <div class="field">
        <label for="p">{t('setup.password')}</label>
        <input id="p" class="input" type={showPw ? 'text' : 'password'} bind:value={password} autocomplete="new-password" required minlength="10" maxlength="128"
          aria-invalid={pwError ? 'true' : undefined} aria-describedby="p-hint{pwError ? ' p-err' : ''}" oninput={() => (pwError = '')} />
        <PasswordStrength {password} />
        {#if pwError}<p class="error-text" id="p-err">{pwError}</p>{/if}
        <p class="hint" id="p-hint">{t('setup.passwordHint')}</p>
      </div>
      <div class="field">
        <label for="r">{t('setup.passwordRepeat')}</label>
        <input id="r" class="input" type={showPw ? 'text' : 'password'} bind:value={repeat} autocomplete="new-password" aria-invalid={mismatch}
          aria-describedby={mismatch ? 'r-err' : undefined} required />
        {#if mismatch}<p class="error-text" id="r-err">{t('setup.mismatch')}</p>{/if}
      </div>
      <label class="check"><input type="checkbox" bind:checked={showPw} /> {t('setup.showPassword')}</label>
      {#if st?.setupTokenRequired}
        <div class="field">
          <label for="tok">{t('setup.token')}</label>
          <input id="tok" class="input mono" bind:value={token} autocomplete="off" required />
          <p class="hint">{t('setup.tokenHint')}</p>
        </div>
      {/if}
      {#if error}<p class="error-text" role="alert">{error}</p>{/if}
      <button class="btn primary" type="submit">{t('setup.next')}</button>
    </form>
  {:else}
    <form onsubmit={finish} class="stack">
      <div class="head">
        <span class="step">{t('setup.step', { n: 2 })}</span>
        <h1>{t('setup.networkTitle')}</h1>
        <p class="muted">{t('setup.networkLead')}</p>
      </div>
      <div class="field">
        <label for="n">{t('setup.networkName')}</label>
        <input id="n" class="input" bind:value={networkName} placeholder={t('setup.networkPlaceholder')} maxlength="64" />
      </div>

      {#if error}<p class="error-text" role="alert">{error}</p>{/if}
      <div class="row">
        <button class="btn" type="button" onclick={() => (step = 1)} disabled={busy}>{t('common.back')}</button>
        <button class="btn primary grow" type="submit" disabled={busy}>{t('setup.finish')}</button>
      </div>
    </form>
  {/if}
</AuthCard>

<style>
  .head {
    display: grid;
    gap: 6px;
    margin-bottom: 4px;
  }
  .step {
    font-family: var(--mono);
    font-size: 11px;
    font-weight: 500;
    color: var(--text-3);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  h1 {
    font-size: 28px;
  }
  .grow {
    flex: 1;
  }
  .blocked {
    display: grid;
    gap: 10px;
    justify-items: start;
  }
  .lock {
    display: grid;
    place-items: center;
    width: 44px;
    height: 44px;
    border-radius: 50%;
    background: var(--warn-soft);
    color: var(--warn);
  }
</style>
