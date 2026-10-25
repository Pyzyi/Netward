import type { Component } from 'svelte';
import Router from '@lucide/svelte/icons/router';
import Wifi from '@lucide/svelte/icons/wifi';
import Network from '@lucide/svelte/icons/network';
import Server from '@lucide/svelte/icons/server';
import HardDrive from '@lucide/svelte/icons/hard-drive';
import Monitor from '@lucide/svelte/icons/monitor';
import Laptop from '@lucide/svelte/icons/laptop';
import Smartphone from '@lucide/svelte/icons/smartphone';
import Tablet from '@lucide/svelte/icons/tablet';
import Watch from '@lucide/svelte/icons/watch';
import Tv from '@lucide/svelte/icons/tv';
import Cast from '@lucide/svelte/icons/cast';
import Speaker from '@lucide/svelte/icons/speaker';
import Gamepad from '@lucide/svelte/icons/gamepad-2';
import Printer from '@lucide/svelte/icons/printer';
import Cctv from '@lucide/svelte/icons/cctv';
import House from '@lucide/svelte/icons/house';
import Lightbulb from '@lucide/svelte/icons/lightbulb';
import Plug from '@lucide/svelte/icons/plug';
import Cpu from '@lucide/svelte/icons/cpu';
import Box from '@lucide/svelte/icons/box';
import Unknown from '@lucide/svelte/icons/circle-question-mark';
import { t } from './i18n.svelte';
import type { Device, DeviceType } from './types';

export const deviceTypes: DeviceType[] = [
  'router', 'access_point', 'switch', 'server', 'nas', 'computer', 'laptop', 'phone', 'tablet', 'watch', 'tv',
  'streaming', 'speaker', 'game_console', 'printer', 'camera', 'smart_home', 'light', 'plug', 'iot', 'virtual', 'unknown',
];

export const typeIcons: Record<DeviceType, Component> = {
  router: Router, access_point: Wifi, switch: Network, server: Server, nas: HardDrive, computer: Monitor,
  laptop: Laptop, phone: Smartphone, tablet: Tablet, watch: Watch, tv: Tv, streaming: Cast, speaker: Speaker,
  game_console: Gamepad, printer: Printer, camera: Cctv, smart_home: House, light: Lightbulb, plug: Plug,
  iot: Cpu, virtual: Box, unknown: Unknown,
};

export type Category = 'net' | 'compute' | 'mobile' | 'media' | 'home' | 'peripheral' | 'unknown';

const categories: Record<DeviceType, Category> = {
  router: 'net', access_point: 'net', switch: 'net',
  server: 'compute', nas: 'compute', computer: 'compute', laptop: 'compute', virtual: 'compute',
  phone: 'mobile', tablet: 'mobile', watch: 'mobile',
  tv: 'media', streaming: 'media', speaker: 'media', game_console: 'media',
  smart_home: 'home', light: 'home', plug: 'home', iot: 'home',
  printer: 'peripheral', camera: 'peripheral',
  unknown: 'unknown',
};

export function category(type: DeviceType): Category {
  return categories[type] ?? 'unknown';
}

export function typeLabel(type: string): string {
  return t(`type.${type}`);
}

/** The name shown everywhere: user name, discovered name or a description. */
export function displayName(d: Device): string {
  if (d.name) return d.name;
  if (d.hostname) return d.hostname;
  const label = d.type === 'unknown' ? t('type.device') : typeLabel(d.type);
  if (d.vendor) return `${d.vendor} ${label}`;
  return `${label} …${d.mac.slice(-5)}`;
}

/** Secondary line: vendor and model, or what is known. */
export function subtitle(d: Device): string {
  const parts: string[] = [];
  if (d.vendor && !displayName(d).startsWith(d.vendor)) parts.push(d.vendor);
  if (d.model) parts.push(d.model);
  else if (d.type !== 'unknown' && !displayName(d).includes(typeLabel(d.type))) parts.push(typeLabel(d.type));
  if (!parts.length && d.macKind === 'random') parts.push(t('devices.private'));
  return parts.join(' · ');
}

/** Sort key for IPv4 addresses. */
export function ipKey(ip: string): number {
  const p = ip.split('.').map(Number);
  if (p.length !== 4 || p.some((n) => Number.isNaN(n))) return Number.MAX_SAFE_INTEGER;
  return ((p[0] * 256 + p[1]) * 256 + p[2]) * 256 + p[3];
}

export function matches(d: Device, q: string): boolean {
  if (!q) return true;
  const hay = [displayName(d), d.hostname, d.ip, d.mac, d.vendor, d.model, d.location, typeLabel(d.type), d.notes]
    .join(' ')
    .toLowerCase();
  return q
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
    .every((part) => hay.includes(part));
}
