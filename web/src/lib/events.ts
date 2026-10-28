import type { EventItem } from './types';
import { duration, has, t } from './i18n.svelte';

/** Human sentence for an activity log entry. */
export function eventText(e: EventItem, deviceName?: string): string {
  const d = e.data ?? {};
  const name = deviceName ?? (typeof d.name === 'string' && d.name ? d.name : t('type.device'));
  const vars: Record<string, string | number> = {};
  for (const [k, v] of Object.entries(d)) if (typeof v === 'string' || typeof v === 'number') vars[k] = v;
  vars.name = name; // the live device name wins over the name stored with the event
  if (e.kind === 'internet_up' && typeof d.durationMs === 'number') vars.duration = duration(d.durationMs);
  if (d.check === 'tripwire' && typeof d.service === 'string' && has(`tripwire.svc.${d.service}`)) vars.service = t(`tripwire.svc.${d.service}`);
  let key = `event.${e.kind}`;
  // Software findings have their own sentences ("… bekommt keine Updates mehr").
  if (e.kind === 'security_finding' && typeof d.check === 'string' && has(`${key}.${d.check}${d.count ? '.count' : ''}`)) {
    key = `${key}.${d.check}${d.count ? '.count' : ''}`;
  }
  const text = t(key, vars);
  return text === key ? e.kind : text;
}

export type EventGroup = 'devices' | 'presence' | 'internet' | 'security' | 'system';

export function eventGroup(kind: string): EventGroup {
  if (kind === 'device_online' || kind === 'device_offline') return 'presence';
  if (kind.startsWith('internet')) return 'internet';
  if (groupKinds.security.includes(kind)) return 'security';
  if (groupKinds.devices.includes(kind)) return 'devices';
  return 'system';
}

export const groupKinds: Record<EventGroup, string[]> = {
  devices: ['device_new', 'ip_changed', 'port_new', 'device_removed', 'device_merged', 'scan_baseline'],
  presence: ['device_online', 'device_offline'],
  internet: ['internet_down', 'internet_up'],
  security: ['security_finding', 'ip_conflict', 'gateway_changed', 'dhcp_server_new', 'identity_changed', 'device_flood', 'auth_failed'],
  system: ['auth_setup', 'auth_login', 'auth_password', 'config_settings', 'config_channel', 'config_network'],
};

/** Kinds worth showing on the overview: everything except routine sign-ins and settings changes. */
export const overviewKinds = [
  ...groupKinds.devices,
  ...groupKinds.presence,
  ...groupKinds.internet,
  ...groupKinds.security,
];

export type EventRun = { e: EventItem; count: number; firstTs: number };

/** Collapses consecutive repeats (same kind, same device, same text) into one row. */
export function collapse(items: EventItem[], text: (e: EventItem) => string): EventRun[] {
  const out: EventRun[] = [];
  for (const e of items) {
    const last = out.at(-1);
    if (last && last.e.kind === e.kind && last.e.deviceId === e.deviceId && text(last.e) === text(e)) {
      last.count++;
      last.firstTs = e.ts;
    } else {
      out.push({ e, count: 1, firstTs: e.ts });
    }
  }
  return out;
}
