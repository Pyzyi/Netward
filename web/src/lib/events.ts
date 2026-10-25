import type { EventItem } from './types';
import { duration, t } from './i18n.svelte';

/** Human sentence for an activity log entry. */
export function eventText(e: EventItem, deviceName?: string): string {
  const d = e.data ?? {};
  const name = deviceName ?? (typeof d.name === 'string' && d.name ? d.name : t('type.device'));
  const vars: Record<string, string | number> = {};
  for (const [k, v] of Object.entries(d)) if (typeof v === 'string' || typeof v === 'number') vars[k] = v;
  vars.name = name; // the live device name wins over the name stored with the event
  if (e.kind === 'internet_up' && typeof d.durationMs === 'number') vars.duration = duration(d.durationMs);
  const key = `event.${e.kind}`;
  const text = t(key, vars);
  return text === key ? e.kind : text;
}

export type EventGroup = 'devices' | 'presence' | 'internet' | 'security' | 'system';

export function eventGroup(kind: string): EventGroup {
  if (kind === 'device_online' || kind === 'device_offline') return 'presence';
  if (kind.startsWith('internet')) return 'internet';
  if (kind === 'security_finding' || kind === 'ip_conflict' || kind === 'device_flood' || kind === 'auth_failed') return 'security';
  if (kind.startsWith('device') || kind === 'ip_changed' || kind === 'scan_baseline') return 'devices';
  return 'system';
}

export const groupKinds: Record<EventGroup, string[]> = {
  devices: ['device_new', 'ip_changed', 'device_removed', 'device_merged', 'scan_baseline'],
  presence: ['device_online', 'device_offline'],
  internet: ['internet_down', 'internet_up'],
  security: ['security_finding', 'ip_conflict', 'device_flood', 'auth_failed'],
  system: ['auth_setup', 'auth_login', 'auth_password', 'config_settings', 'config_channel'],
};
