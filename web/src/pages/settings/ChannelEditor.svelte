<script lang="ts">
  import { errorText } from '../../lib/errors';
  import Dialog from '../../lib/components/Dialog.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { api } from '../../lib/api';
  import { toast } from '../../lib/components/toast.svelte';
  import { t } from '../../lib/i18n.svelte';
  import type { Channel, ChannelConfig, ChannelKind } from '../../lib/types';

  let {
    open = $bindable(false),
    channel,
    kinds,
    events,
    onsaved,
  }: { open?: boolean; channel: Channel | null; kinds: string[]; events: string[]; onsaved: (c: Channel) => void } = $props();

  const MASK = '••••••••';
  let name = $state('');
  let kind = $state<ChannelKind>('ntfy');
  let cfg = $state<ChannelConfig>({});
  let subs = $state<string[]>([]);
  let enabled = $state(true);
  let busy = $state(false);
  let error = $state('');
  let savedId = $state<number | null>(null); // set once "save & test" stored a new channel

  $effect(() => {
    if (!open) return;
    name = channel?.name ?? '';
    kind = channel?.kind ?? 'ntfy';
    // Masked secrets come back as the mask; show them as empty placeholders.
    const c = { ...(channel?.config ?? {}) } as Record<string, unknown>;
    for (const k of Object.keys(c)) if (c[k] === MASK) c[k] = '';
    cfg = c as ChannelConfig;
    subs = channel?.events ?? ['device_new', 'ip_conflict', 'internet_up', 'security_finding', 'device_flood'];
    enabled = channel?.enabled ?? true;
    error = '';
    savedId = null;
  });

  // A message from the previous service does not apply to the new one.
  $effect(() => {
    void kind;
    error = '';
  });

  type Field = { key: keyof ChannelConfig; label: string; secret?: boolean; type?: string; placeholder?: string };
  const fields: Record<ChannelKind, Field[]> = {
    ntfy: [
      { key: 'server', label: 'channel.field.serverNtfy', placeholder: 'https://ntfy.sh' },
      { key: 'topic', label: 'channel.field.topic', placeholder: 'mein-heimnetz-a8f3' },
      { key: 'token', label: 'channel.field.token', secret: true },
    ],
    gotify: [
      { key: 'server', label: 'channel.field.server', placeholder: 'http://192.168.1.10:8080' },
      { key: 'token', label: 'channel.field.appToken', secret: true },
    ],
    pushover: [
      { key: 'user', label: 'channel.field.userKey', secret: true },
      { key: 'token', label: 'channel.field.appToken', secret: true },
    ],
    telegram: [
      { key: 'token', label: 'channel.field.botToken', secret: true, placeholder: '123456:ABC…' },
      { key: 'chatId', label: 'channel.field.chatId' },
    ],
    discord: [{ key: 'url', label: 'channel.field.webhookUrl', secret: true, placeholder: 'https://discord.com/api/webhooks/…' }],
    slack: [{ key: 'url', label: 'channel.field.webhookUrl', secret: true, placeholder: 'https://hooks.slack.com/services/…' }],
    webhook: [
      { key: 'url', label: 'channel.field.url', secret: true, placeholder: 'https://…' },
      { key: 'headerName', label: 'channel.field.headerName', placeholder: 'Authorization' },
      { key: 'headerValue', label: 'channel.field.headerValue', secret: true },
    ],
    email: [
      { key: 'host', label: 'channel.field.host', placeholder: 'smtp.example.org' },
      { key: 'port', label: 'channel.field.port', type: 'number', placeholder: '587' },
      { key: 'username', label: 'channel.field.username' },
      { key: 'password', label: 'channel.field.password', secret: true },
      { key: 'from', label: 'channel.field.from', placeholder: 'netzwart@example.org' },
      { key: 'to', label: 'channel.field.to' },
    ],
  };

  const isSecretSet = (key: string) =>
    channel !== null && channel.kind === kind && (channel.config as Record<string, unknown>)[key] === MASK;

  async function save(thenTest = false) {
    busy = true;
    error = '';
    const body = { name: name.trim() || t(`channel.${kind}`), kind, config: { ...cfg }, events: subs, enabled };
    if (body.config.port !== undefined) body.config.port = Number(body.config.port) || undefined;
    try {
      const id = savedId ?? channel?.id;
      const saved = id ? await api.updateChannel(id, body) : await api.createChannel(body);
      onsaved(saved);
      if (thenTest) {
        const r = await api.testChannel(saved.id);
        if (!r.ok) {
          // Keep the dialog open so the settings can be corrected right away.
          savedId = saved.id;
          error = t('settings.channelTestFail', { reason: r.reason ?? '' });
          return;
        }
        toast(t('settings.channelTestOk'), 'ok', 6000);
      }
      open = false;
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog bind:open title={channel ? channel.name : t('settings.channelAdd')} width={540}>
  <form class="stack" onsubmit={(e) => { e.preventDefault(); void save(); }} id="chform">
    <div class="two">
      <div class="field">
        <label for="ck">{t('settings.channelKind')}</label>
        <select id="ck" class="select" bind:value={kind} disabled={channel !== null}>
          {#each kinds as k (k)}<option value={k}>{t(`channel.${k}`)}</option>{/each}
        </select>
      </div>
      <div class="field">
        <label for="cn">{t('settings.channelName')}</label>
        <input id="cn" class="input" bind:value={name} maxlength="64" placeholder={t(`channel.${kind}`)} />
      </div>
    </div>
    {#each fields[kind] as f (f.key)}
      <div class="field">
        <label for="f-{f.key}">{t(f.label)}</label>
        {#if f.key === 'port' && kind === 'email'}
          <div class="two">
            <input id="f-port" class="input" type="number" min="1" max="65535" bind:value={cfg.port} placeholder="587" />
            <select class="select" bind:value={cfg.security} aria-label={t('channel.field.security')}>
              <option value="starttls">STARTTLS</option>
              <option value="tls">TLS</option>
              <option value="none">{t('common.off')}</option>
            </select>
          </div>
        {:else}
          <input
            id="f-{f.key}"
            class="input"
            class:mono={f.secret}
            type={f.secret ? 'password' : f.type ?? 'text'}
            autocomplete="off"
            bind:value={cfg[f.key] as string}
            placeholder={f.secret && isSecretSet(f.key) ? t('settings.secretKept') : f.placeholder ? `${t('common.eg')} ${f.placeholder}` : ''}
          />
        {/if}
      </div>
    {/each}
    <details class="help">
      <summary>{t('channel.helpTitle')}</summary>
      <p>{t(`channel.help.${kind}`)}</p>
    </details>
    <p class="hint">{t('settings.localhostHint')}</p>
    <fieldset>
      <legend class="label">{t('settings.channelEvents')}</legend>
      <div class="events">
        {#each events as ev (ev)}
          <label class="check">
            <input type="checkbox" checked={subs.includes(ev)} onchange={(e) => (subs = e.currentTarget.checked ? [...subs, ev] : subs.filter((x) => x !== ev))} />
            {t(`event.kind.${ev}`)}
          </label>
        {/each}
      </div>
    </fieldset>
    <Toggle bind:checked={enabled} label={t('settings.channelEnabled')} />
    {#if error}<p class="error-text" role="alert">{error}</p>{/if}
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t('common.cancel')}</button>
    <button class="btn" type="button" disabled={busy || !enabled} onclick={() => save(true)}>{t('settings.channelSaveTest')}</button>
    <button class="btn primary" type="submit" form="chform" disabled={busy}>{t('common.save')}</button>
  {/snippet}
</Dialog>

<style>
  .help {
    font-size: 14px;
    color: var(--text-2);
  }
  .help summary {
    cursor: pointer;
    font-weight: 600;
    color: var(--text);
    padding: 4px 0;
  }
  .help p {
    margin: 6px 0 0;
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  fieldset {
    border: 0;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 8px;
  }
  .events {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 16px;
  }
  @media (max-width: 520px) {
    .two,
    .events {
      grid-template-columns: 1fr;
    }
  }
</style>
