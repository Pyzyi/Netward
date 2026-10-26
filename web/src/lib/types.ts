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
  /** Short software verdict; missing when nothing is known. */
  software?: { status: SoftwareStatus; product: string; name: string };
}

export type SoftwareStatus = 'supported' | 'aging' | 'outdated' | 'update' | 'unknown';

export interface SoftwareObservation {
  source: string;
  product: string;
  version: string;
  evidence: string;
  seenAt: number;
  updateAvailable?: boolean;
}

/** What Netzwart knows about the software of a device (internal/software). */
export interface SoftwareAssessment {
  status: SoftwareStatus;
  product: string;
  name: string;
  version: string;
  since?: string;
  until?: string;
  maxOs?: string;
  note?: string;
  edition?: string;
  conflict?: boolean;
  lastSeen?: number;
  source?: string;
  evidence?: string;
  observations: SoftwareObservation[];
  dataAsOf: string;
  exploited?: {
    verdict: 'affected' | 'uncertain' | 'none';
    fixed?: string;
    cves?: { id: string; name: string; added: string; url?: string }[];
    version: string;
    manual?: boolean;
    dataAsOf: string;
  };
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
  software: SoftwareAssessment;
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
  software: Record<SoftwareStatus, number>;
}

export interface Finding {
  id: number;
  check: 'risky_port' | 'upnp_igd' | 'ip_conflict' | 'unknown_devices' | 'software_outdated' | 'software_update' | 'gateway_mac' | 'dhcp_rogue' | 'known_exploited';
  deviceId?: number;
  severity: 'info' | 'warning' | 'critical';
  data: { port?: number; service?: string; ip?: string; product?: string; status?: string; kind?: 'conflict' | 'changed'; server?: string; fixed?: string; manual?: string; uncertain?: string };
  firstSeen: number;
  lastSeen: number;
  dismissed: boolean;
}

/** Who answers for the router address and who hands out addresses. */
export interface NetworkState {
  gateways: {
    ip: string;
    mac: string;
    vendor?: string;
    since: number;
    change?: { mac: string; vendor?: string; at: number; oldActive: boolean };
    conflict?: boolean;
  }[];
  dhcpServers: {
    serverId: string;
    mac: string;
    vendor?: string;
    router?: string;
    dns?: string[];
    firstSeen: number;
    lastSeen: number;
    accepted: boolean;
  }[];
  probe?: { at: number; answers: number };
}

/** Connection data from the FRITZ!Box (docs/09, F5); rates in kbit/s. */
export interface LineSample {
  t: number;
  access: 'dsl' | 'cable' | 'fiber' | 'ethernet' | 'mobile' | '';
  up: boolean;
  down: number;
  upstream: number;
  downMax?: number;
  upMax?: number;
}

/** Provider report (docs/09, F5): contract speeds in Mbit/s, 0 = not given. */
export interface ReportContract {
  maxDown: number;
  normalDown: number;
  minDown: number;
  maxUp: number;
  normalUp: number;
  minUp: number;
}
export interface ReportRequest {
  from: string;
  to: string;
  format: 'pdf' | 'csv';
  contract: ReportContract;
  holder: string;
  customer: string;
  address: string;
}

/** Wi-Fi diagnosis from the FRITZ!Box (docs/09, F3). */
export type WlanHint = 'weak' | 'very_weak' | 'band24' | 'backhaul_weak' | 'weak_repeater';
export interface WlanClient {
  mac: string;
  apMac: string;
  band: '2.4' | '5' | '6' | '';
  channel?: number;
  signalDbm?: number;
  signalPct?: number;
  rateRx?: number;
  rateTx?: number;
  maxRate?: number;
  can5ghz?: boolean;
  deviceId?: number;
  hints: WlanHint[];
}
export interface WlanAccessPoint {
  mac: string;
  name: string;
  model?: string;
  master: boolean;
  bands?: string[];
  backhaul?: { type: 'wlan' | 'lan'; band?: string; signalDbm?: number; rate?: number };
  deviceId?: number;
  hints: WlanHint[];
  clients: WlanClient[];
}
export interface WlanView {
  at: number;
  source?: 'mesh' | 'wlanlist';
  aps: WlanAccessPoint[];
  fritzbox: 'disabled' | 'connecting' | 'connected' | 'error';
  status?: '' | 'mesh' | 'wlanlist' | 'auth' | 'error';
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
  dhcpCheck: boolean;
  updateCheck: boolean;
  mqtt: MQTTSettings;
  fritzbox: FritzBoxSettings;
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

export interface FritzBoxSettings {
  enabled: boolean;
  host: string;
  username: string;
  password: string;
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
  fritzbox: { state: string; error?: string; hosts: number; lastSync: number } | null;
  https: boolean;
  streams: number;
  softwareDataAsOf: string;
  update: UpdateStatus | null;
}

export interface UpdateStatus {
  enabled: boolean;
  current: string;
  latest?: string;
  url?: string;
  available: boolean;
  checkedAt?: number;
  error?: string;
}
