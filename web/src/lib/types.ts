export type DeviceType =
  | 'router' | 'access_point' | 'switch' | 'server' | 'nas' | 'computer' | 'laptop' | 'phone' | 'tablet'
  | 'watch' | 'tv' | 'streaming' | 'speaker' | 'game_console' | 'printer' | 'camera' | 'smart_home'
  | 'light' | 'plug' | 'iot' | 'virtual' | 'unknown';

export interface Device {
  id: number;
  mac: string;
  ip: string;
  name: string;
  hostname: string;
  vendor: string;
  vendorOfficial: string;
  model: string;
  os: string;
  type: DeviceType;
  detectedType: DeviceType;
  typeConfidence: number;
  typeOverride: string;
  location: string;
  notes: string;
  known: boolean;
  favorite: boolean;
  watch: boolean;
  macKind: 'universal' | 'random' | 'virtual';
  proxy: boolean;
  viaProxy: boolean;
  online: boolean;
  isGateway: boolean;
  isSelf: boolean;
  interface: string;
  dhcpFingerprint: string;
  dhcpVendor: string;
  firstSeen: number;
  lastSeen: number;
  enrichedAt: number;
  portsScannedAt: number;
  new: boolean;
}

export interface Port {
  port: number;
  proto: string;
  service: string;
  firstSeen: number;
  lastSeen: number;
}

export interface Address {
  ip: string;
  interface: string;
  firstSeen: number;
  lastSeen: number;
}

export interface DeviceDetails {
  names: Record<string, string>;
  services: string[];
  instances: Record<string, string>;
  upnpTypes: string[];
  maker: string;
  model: string;
}

export interface DeviceResponse {
  device: Device;
  details: DeviceDetails;
  addresses: Address[];
  ports: Port[];
}

export interface PresenceSpan { start: number; end: number }
export interface Gap { start: number; end: number }
export interface Presence { from: number; to: number; spans: PresenceSpan[]; gaps: Gap[]; onlinePct: number }

export interface EventItem {
  id: number;
  ts: number;
  kind: string;
  severity: 'info' | 'warning' | 'critical';
  deviceId?: number;
  data: Record<string, unknown>;
}

export interface IfaceStatus {
  name: string;
  mac: string;
  addr: string;
  prefix: string;
  hosts: number;
  active: boolean;
  default: boolean;
  skipped?: 'large' | 'too_large' | 'fallback_limit';
}

export interface ScannerStatus {
  mode: 'raw' | 'fallback' | 'demo';
  scanning: boolean;
  lastScanAt: number;
  lastScanDuration: number;
  nextScanAt: number;
  interfaces: IfaceStatus[] | null;
  gateways: string[] | null;
  error?: string;
}

export interface InternetStatus {
  state: 'unknown' | 'online' | 'degraded' | 'offline' | 'lan_down' | 'disabled';
  latencyMs: number;
  checkedAt: number;
  since: number;
  gateway: string;
  gatewayOk: boolean;
}

export interface Overview {
  networkName: string;
  counts: { total: number; online: number; offline: number; new: number; unreviewed: number };
  scanner: ScannerStatus;
  internet: InternetStatus;
  security: { critical: number; warning: number; info: number; lastRunAt: number; running: boolean };
  baseline: number;
  types: Record<string, number>;
}

export interface Finding {
  id: number;
  check: 'risky_port' | 'upnp_igd' | 'ip_conflict' | 'unknown_devices';
  deviceId?: number;
  severity: 'info' | 'warning' | 'critical';
  data: { port?: number; service?: string; ip?: string };
  firstSeen: number;
  lastSeen: number;
  dismissed: boolean;
}

export interface MQTTSettings {
  enabled: boolean;
  host: string;
  port: number;
  tls: boolean;
  username: string;
  password: string;
  discoveryPrefix: string;
  baseTopic: string;
}

export interface Settings {
  networkName: string;
  language: 'de' | 'en';
  timezone: string;
  interfaces: string[];
  scanInterval: number;
  offlineAfter: number;
  offlineAfterMobile: number;
  allowLargeSubnets: boolean;
  resolveNames: boolean;
  notifyNewDevice: boolean;
  notifyRandomMac: boolean;
  expireRandomDays: number;
  retentionDays: number;
  internetCheck: boolean;
  internetTargets: string[];
  securityScanDays: number;
  mqtt: MQTTSettings;
  fritzbox: { enabled: boolean; host: string; username: string; password: string };
}

export type ChannelKind = 'ntfy' | 'gotify' | 'pushover' | 'telegram' | 'discord' | 'slack' | 'webhook' | 'email';

export interface ChannelConfig {
  server?: string;
  topic?: string;
  token?: string;
  user?: string;
  chatId?: string;
  url?: string;
  headerName?: string;
  headerValue?: string;
  host?: string;
  port?: number;
  security?: 'starttls' | 'tls' | 'none';
  username?: string;
  password?: string;
  from?: string;
  to?: string;
  priority?: number;
}

export interface Channel {
  id: number;
  name: string;
  kind: ChannelKind;
  config: ChannelConfig;
  events: string[];
  enabled: boolean;
  createdAt: number;
  updatedAt: number;
}

export interface Session {
  id: number;
  createdAt: number;
  lastSeenAt: number;
  expiresAt: number;
  remember: boolean;
  userAgent: string;
  ip: string;
  current: boolean;
}

export interface AuthState {
  setupRequired: boolean;
  setupAllowed: boolean;
  setupReason?: 'expired' | 'not_local';
  setupTokenRequired: boolean;
  authenticated: boolean;
  username?: string;
  csrf?: string;
  version: string;
  language: 'de' | 'en';
  https: boolean;
}

export interface SystemInfo {
  version: string;
  startedAt: number;
  goVersion: string;
  uid: number;
  rawSockets: boolean;
  mode: string;
  dbSize: number;
  memory: number;
  mqtt: { state: string; error?: string; since: number } | null;
  https: boolean;
  streams: number;
}
