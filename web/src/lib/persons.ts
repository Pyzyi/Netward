import type { Device, Person, PersonStatus } from './types';
import { shortDay, t, time } from './i18n.svelte';

/** At home while at least one presence device is online (live from the device list). */
export function isHome(p: Person, devices: Map<number, Device>): boolean {
  return p.devices.some((d) => d.presence && devices.get(d.deviceId)?.online);
}

function when(ms: number, now: number): string {
  // "Gestern"/"Yesterday" stands alone elsewhere; inside a sentence it is lower case.
  const day = shortDay(ms, now);
  const inline = day === t('activity.yesterday') ? day.toLocaleLowerCase() : day;
  return day ? `${inline}, ${time(ms)}` : time(ms);
}

/** "zu Hause seit 07:12", "unterwegs seit gestern, 18:40". */
export function presenceText(p: PersonStatus, home: boolean, now: number): string {
  if (p.devices.every((d) => !d.presence)) return t('persons.noPresence');
  // The live state changed since the status was loaded: no "since" yet.
  if (p.home !== home || !p.since) return t(home ? 'persons.home' : 'persons.away');
  return t(home ? 'persons.homeSince' : 'persons.awaySince', { t: when(p.since, now) });
}

export function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => [...w][0]?.toUpperCase() ?? '')
    .join('');
}
