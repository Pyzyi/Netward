import type { Device, WlanAccessPoint, WlanClient, WlanHint } from './types';
import { displayName } from './devices';
import { i18n, number, t } from './i18n.svelte';

/** "2,4 GHz" / "2.4 GHz". */
export function bandLabel(b: string | undefined): string {
  if (!b) return '';
  return t('wlan.band', { b: i18n.lang === 'de' ? b.replace('.', ',') : b });
}

export type SignalLevel = 'good' | 'ok' | 'weak' | 'very_weak' | 'unknown';

/** Signal quality from dBm (mesh list) or percent (fallback list). */
export function signalLevel(c: { signalDbm?: number; signalPct?: number }): SignalLevel {
  if (c.signalDbm) {
    if (c.signalDbm >= -65) return 'good';
    if (c.signalDbm >= -75) return 'ok';
    if (c.signalDbm >= -82) return 'weak';
    return 'very_weak';
  }
  if (c.signalPct) return c.signalPct >= 60 ? 'good' : c.signalPct >= 35 ? 'ok' : 'weak';
  return 'unknown';
}

export function signalText(c: { signalDbm?: number; signalPct?: number }): string {
  if (c.signalDbm) return t('wlan.dbm', { v: c.signalDbm });
  if (c.signalPct) return t('wlan.pct', { v: c.signalPct });
  return '';
}

export function rateText(c: WlanClient): string {
  const r = Math.max(c.rateRx ?? 0, c.rateTx ?? 0);
  if (!r) return '';
  return c.maxRate ? t('wlan.rateOf', { v: number(r), max: number(c.maxRate) }) : t('wlan.rate', { v: number(r) });
}

export function apName(ap: WlanAccessPoint, devices: Map<number, Device>): string {
  const d = ap.deviceId ? devices.get(ap.deviceId) : undefined;
  return d?.name || ap.name || (ap.master ? 'FRITZ!Box' : t('wlan.repeater'));
}

export function clientName(c: WlanClient, devices: Map<number, Device>): string {
  const d = c.deviceId ? devices.get(c.deviceId) : undefined;
  return d ? displayName(d) : t('wlan.unknownDevice', { mac: c.mac });
}

/** One sentence per hint, naming the device and access point. */
export function hintText(h: WlanHint, c: WlanClient | null, ap: WlanAccessPoint, devices: Map<number, Device>): string {
  const vars = { name: c ? clientName(c, devices) : '', ap: apName(ap, devices), dbm: c ? signalText(c) : '' };
  if (h === 'band24' && !ap.master) return t('wlan.hint.band24Repeater', vars);
  if (h === 'backhaul_weak') {
    const b = ap.backhaul;
    const parts = [b?.signalDbm ? t('wlan.dbm', { v: b.signalDbm }) : '', b?.rate ? t('wlan.rate', { v: number(b.rate) }) : ''].filter(Boolean);
    return t('wlan.hint.backhaul_weak', { ...vars, detail: parts.join(', ') });
  }
  return t(`wlan.hint.${h}`, vars);
}

export function hintTodo(h: WlanHint): string {
  return t(`wlan.todo.${h === 'very_weak' ? 'weak' : h === 'weak_repeater' ? 'backhaul_weak' : h}`);
}
