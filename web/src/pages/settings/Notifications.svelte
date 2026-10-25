<script lang="ts">
  import Icon from '../../lib/components/Icon.svelte';
  import SettingsCard from '../../lib/components/SettingsCard.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import Dialog from '../../lib/components/Dialog.svelte';
  import ChannelEditor from './ChannelEditor.svelte';
  import { toast } from '../../lib/components/toast.svelte';
  import { api } from '../../lib/api';
  import { store, updateSettings } from '../../lib/state.svelte';
  import { t } from '../../lib/i18n.svelte';
  import type { Channel } from '../../lib/types';

  let channels = $state<Channel[]>([]);
  let kinds = $state<string[]>([]);
  let events = $state<string[]>([]);
  let editorOpen = $state(false);
  let editing = $state<Channel | null>(null);
  let deleting = $state<Channel | null>(null);
  let deleteOpen = $state(false);
  let testing = $state<number | null>(null);
  let loaded = $state(false);

  async function load() {
    const r = await api.channels();
    channels = r.channels;
    kinds = r.kinds;
    events = r.events;
    loaded = true;
  }
  void load();

  const s = store.settings!;
  let notifyNew = $state(s.notifyNewDevice);
  let notifyRandom = $state(s.notifyRandomMac);

  async function saveToggles() {
    const err = await updateSettings((x) => {
      x.notifyNewDevice = notifyNew;
      x.notifyRandomMac = notifyRandom;
    });
    toast(err ? t('err.invalid_settings', { msg: err }) : t('common.saved'), err ? 'error' : 'ok');
  }

  async function test(c: Channel) {
    testing = c.id;
    try {
      const r = await api.testChannel(c.id);
      toast(r.ok ? t('settings.channelTestOk') : t('settings.channelTestFail', { reason: r.reason ?? '' }), r.ok ? 'ok' : 'error', 6000);
    } finally {
      testing = null;
    }
  }

  async function toggle(c: Channel, enabled: boolean) {
    const updated = await api.updateChannel(c.id, { name: c.name, kind: c.kind, config: c.config, events: c.events, enabled });
    channels = channels.map((x) => (x.id === c.id ? updated : x));
  }

  async function remove() {
    if (!deleting) return;
    await api.deleteChannel(deleting.id);
    channels = channels.filter((x) => x.id !== deleting!.id);
    deleteOpen = false;
  }
</script>

{#if loaded && notifyNew && !channels.some((c) => c.enabled)}
  <div class="notice warn missing" role="status">
    <span>{t('settings.channelsMissing')}</span>
  </div>
{/if}

<SettingsCard title={t('settings.whenNotify')}>
  <Toggle bind:checked={notifyNew} label={t('settings.notifyNew')} onchange={saveToggles} />
  <Toggle bind:checked={notifyRandom} label={t('settings.notifyRandom')} hint={t('settings.notifyRandomHint')} disabled={!notifyNew} onchange={saveToggles} />
</SettingsCard>

<SettingsCard title={t('settings.channels')}>
  {#snippet extra()}
    <button class="btn small" type="button" onclick={() => { editing = null; editorOpen = true; }}><Icon name="plus" size={15} /> {t('settings.channelAdd')}</button>
  {/snippet}
  {#if channels.length === 0}
    <div class="empty"><Icon name="bell" size={20} /><p class="muted">{t('settings.channelsEmpty')}</p></div>
  {:else}
    <ul class="channels">
      {#each channels as c (c.id)}
        <li>
          <div class="ch-text">
            <strong>{c.name}</strong>
            <span class="muted small">{t(`channel.${c.kind}`)} · {c.events.map((e) => t(`event.kind.${e}`)).join(', ') || '–'}</span>
          </div>
          <div class="ch-actions">
            <button class="btn small" type="button" onclick={() => test(c)} disabled={testing === c.id || !c.enabled}><Icon name="paper-plane-tilt" size={14} /> {t('settings.channelTest')}</button>
            <button class="btn small icon" type="button" aria-label={t('common.edit')} onclick={() => { editing = c; editorOpen = true; }}><Icon name="pencil-simple" size={14} /></button>
            <button class="btn small icon danger" type="button" aria-label={t('common.delete')} onclick={() => { deleting = c; deleteOpen = true; }}><Icon name="trash" size={14} /></button>
            <button type="button" role="switch" aria-checked={c.enabled} aria-label={t('settings.channelEnabled')} class="mini-switch" onclick={() => toggle(c, !c.enabled)}><span></span></button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</SettingsCard>

<ChannelEditor bind:open={editorOpen} channel={editing} {kinds} {events}
  onsaved={(c) => (channels = editing ? channels.map((x) => (x.id === c.id ? c : x)) : [...channels, c])} />

<Dialog bind:open={deleteOpen} title={t('settings.channelDelete', { name: deleting?.name ?? '' })}>
  <p class="muted">{deleting ? t(`channel.${deleting.kind}`) : ''}</p>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (deleteOpen = false)}>{t('common.cancel')}</button>
    <button class="btn danger solid" type="button" onclick={remove}>{t('common.delete')}</button>
  {/snippet}
</Dialog>

<style>
  .missing {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
  }
  .empty {
    display: flex;
    gap: 12px;
    align-items: center;
    color: var(--text-3);
  }
  .channels {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 8px;
  }
  .channels li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    flex-wrap: wrap;
  }
  .ch-text {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .small {
    font-size: 13px;
  }
  .ch-actions {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .mini-switch {
    position: relative;
    width: 34px;
    height: 20px;
    margin-left: 4px;
    border: 0;
    border-radius: 999px;
    background: var(--border-strong);
    cursor: pointer;
  }
  .mini-switch[aria-checked='true'] {
    background: var(--accent);
  }
  .mini-switch span {
    position: absolute;
    top: 3px;
    left: 3px;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: #fff;
    transition: transform 150ms var(--ease);
  }
  .mini-switch[aria-checked='true'] span {
    transform: translateX(14px);
  }
</style>
