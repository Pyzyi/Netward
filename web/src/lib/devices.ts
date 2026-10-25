import { t } from './i18n.svelte';
import type { Device, DeviceType } from './types';

export const deviceTypes: DeviceType[] = [
  'router', 'access_point', 'switch', 'server', 'nas', 'computer', 'laptop', 'phone', 'tablet', 'watch', 'tv',
  'streaming', 'speaker', 'game_console', 'printer', 'camera', 'smart_home', 'light', 'plug', 'iot', 'virtual', 'unknown',
];

/** Phosphor duotone glyph per device type (see lib/icons.ts). */
export const typeIcons: Record<DeviceType, string> = {
  router: 'duo:router', access_point: 'duo:wifi-high', switch: 'duo:network', server: 'duo:hard-drives', nas: 'duo:database',
  computer: 'duo:desktop-tower', laptop: 'duo:laptop', phone: 'duo:device-mobile', tablet: 'duo:device-tablet',
  watch: 'duo:watch', tv: 'duo:television-simple', streaming: 'duo:screencast', speaker: 'duo:speaker-hifi',
  game_console: 'duo:game-controller', printer: 'duo:printer', camera: 'duo:security-camera', smart_home: 'duo:house-line',
  light: 'duo:lightbulb', plug: 'duo:plug', iot: 'duo:cpu', virtual: 'duo:cube', unknown: 'duo:question',
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

// Apple reports hardware identifiers over mDNS ("iPhone16,1"); people know
// the marketing names. Unknown identifiers fall back to the product family.
const appleModels: Record<string, string> = {
  'iPhone14,2': 'iPhone 13 Pro', 'iPhone14,3': 'iPhone 13 Pro Max', 'iPhone14,4': 'iPhone 13 mini', 'iPhone14,5': 'iPhone 13',
  'iPhone14,6': 'iPhone SE (3. Gen.)', 'iPhone14,7': 'iPhone 14', 'iPhone14,8': 'iPhone 14 Plus',
  'iPhone15,2': 'iPhone 14 Pro', 'iPhone15,3': 'iPhone 14 Pro Max', 'iPhone15,4': 'iPhone 15', 'iPhone15,5': 'iPhone 15 Plus',
  'iPhone16,1': 'iPhone 15 Pro', 'iPhone16,2': 'iPhone 15 Pro Max',
  'iPhone17,1': 'iPhone 16 Pro', 'iPhone17,2': 'iPhone 16 Pro Max', 'iPhone17,3': 'iPhone 16', 'iPhone17,4': 'iPhone 16 Plus',
  'iPhone17,5': 'iPhone 16e',
  'AppleTV5,3': 'Apple TV HD', 'AppleTV6,2': 'Apple TV 4K', 'AppleTV11,1': 'Apple TV 4K (2. Gen.)', 'AppleTV14,1': 'Apple TV 4K (3. Gen.)',
  'AudioAccessory1,1': 'HomePod', 'AudioAccessory5,1': 'HomePod mini', 'AudioAccessory6,1': 'HomePod (2. Gen.)',
  'Mac14,2': 'MacBook Air (M2)', 'Mac14,15': 'MacBook Air 15" (M2)', 'Mac15,12': 'MacBook Air (M3)', 'Mac15,13': 'MacBook Air 15" (M3)',
  'Mac14,3': 'Mac mini (M2)', 'Mac16,10': 'Mac mini (M4)', 'Mac16,11': 'Mac mini (M4 Pro)',
};
const appleFamilies: [RegExp, string][] = [
  [/^iPhone\d/, 'iPhone'], [/^iPad\d/, 'iPad'], [/^AppleTV\d/, 'Apple TV'], [/^AudioAccessory\d/, 'HomePod'],
  [/^Watch\d/, 'Apple Watch'], [/^(Mac|MacBook\w*|iMac\w*)\d/, 'Mac'],
];

/** Readable model name, e.g. "iPhone 15 Pro" for "iPhone16,1". */
export function modelName(model: string): string {
  if (!model) return '';
  if (appleModels[model]) return appleModels[model];
  for (const [re, family] of appleFamilies) if (re.test(model) && /^\w+\d+,\d+$/.test(model)) return family;
  return model;
}

/** Vendor, inferred from an Apple model identifier when the MAC is private. */
export function vendorName(d: Device): string {
  if (d.vendor) return d.vendor;
  return appleFamilies.some(([re]) => re.test(d.model)) ? 'Apple' : '';
}

/** The name shown everywhere: user name, discovered name or a description. */
export function displayName(d: Device): string {
  if (d.name) return d.name;
  if (d.hostname) return d.hostname;
  const label = d.type === 'unknown' ? t('type.device') : typeLabel(d.type);
  if (modelName(d.model) && modelName(d.model) !== d.model) return modelName(d.model);
  if (vendorName(d)) return `${vendorName(d)} ${label}`;
  return `${label} …${d.mac.slice(-5)}`;
}

/** Secondary line: vendor and model, or what is known. */
export function subtitle(d: Device): string {
  const parts: string[] = [];
  const vendor = vendorName(d);
  const model = modelName(d.model);
  if (vendor && !displayName(d).startsWith(vendor)) parts.push(vendor);
  if (model && !displayName(d).includes(model)) parts.push(model);
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
  const hay = [displayName(d), d.hostname, d.ip, d.mac, vendorName(d), d.model, modelName(d.model), d.location, typeLabel(d.type), d.notes]
    .join(' ')
    .toLowerCase();
  return q
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
    .every((part) => hay.includes(part));
}
