import type {
  AuthState, Channel, Device, DeviceResponse, EventItem, Finding, Overview, Presence, Session, Settings, SystemInfo,
  InternetStatus, LineSample, NetworkState, ReportRequest, SoftwareAssessment, WlanView,
} from './types';

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    public detail = '',
    public retryAfter = 0,
  ) {
    super(code);
  }
}

let csrf = '';
let onUnauthorized: () => void = () => {};

export function setCsrf(token: string) {
  csrf = token;
}

export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await send(method, path, body);
  if (res.status === 204 || res.status === 202) return undefined as T;
  return (await res.json().catch(() => null)) as T;
}

/** A file from the server (report): the blob and the name it should get. */
async function download(path: string, body: unknown): Promise<{ blob: Blob; name: string }> {
  const res = await send('POST', path, body);
  const m = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') ?? '');
  return { blob: await res.blob(), name: m?.[1] ?? 'netzwart' };
}

async function send(method: string, path: string, body?: unknown): Promise<Response> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (method !== 'GET' && csrf) headers['X-CSRF-Token'] = csrf;
  let res: Response;
  try {
    res = await fetch(`/api/v1${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      credentials: 'same-origin',
      cache: 'no-store',
    });
  } catch {
    throw new ApiError(0, 'network');
  }
  if (!res.ok) {
    let data: unknown = null;
    if ((res.headers.get('Content-Type') ?? '').includes('application/json')) data = await res.json().catch(() => null);
    const d = (data ?? {}) as { error?: string; message?: string };
    const err = new ApiError(res.status, d.error ?? 'internal', d.message ?? '', Number(res.headers.get('Retry-After') ?? 0));
    if (res.status === 401 && !path.startsWith('/auth/')) onUnauthorized();
    throw err;
  }
  return res;
}

export const api = {
  state: () => request<AuthState>('GET', '/auth/state'),
  setup: (b: { username: string; password: string; language: string; timezone: string; networkName: string; token: string }) =>
    request<AuthState>('POST', '/auth/setup', b),
  login: (username: string, password: string, remember: boolean) =>
    request<AuthState>('POST', '/auth/login', { username, password, remember }),
  logout: () => request<void>('POST', '/auth/logout'),
  changePassword: (current: string, next: string) => request<void>('POST', '/auth/password', { current, new: next }),
  sessions: () => request<Session[]>('GET', '/auth/sessions'),
  revokeSession: (id: number) => request<void>('DELETE', `/auth/sessions/${id}`),
  revokeOtherSessions: () => request<{ revoked: number }>('POST', '/auth/sessions/revoke-others'),

  overview: () => request<Overview>('GET', '/overview'),
  devices: () => request<Device[]>('GET', '/devices'),
  device: (id: number) => request<DeviceResponse>('GET', `/devices/${id}`),
  setSoftware: (id: number, body: { manual?: { product: string; version: string }; edition?: string }) =>
    request<SoftwareAssessment>('PUT', `/devices/${id}/software`, body),
  patchDevice: (id: number, patch: Partial<Record<'name' | 'type' | 'location' | 'notes', string> & Record<'known' | 'favorite' | 'watch', boolean>>) =>
    request<Device>('PATCH', `/devices/${id}`, patch),
  deleteDevice: (id: number) => request<void>('DELETE', `/devices/${id}`),
  mergeDevice: (id: number, into: number) => request<Device>('POST', `/devices/${id}/merge`, { into }),
  presence: (id: number, range: '24h' | '7d' | '30d') => request<Presence>('GET', `/devices/${id}/presence?range=${range}`),
  strips: () => request<{ from: number; slot: number; strips: Record<string, string> }>('GET', '/presence/strips'),
  deviceEvents: (id: number) => request<EventItem[]>('GET', `/devices/${id}/events`),
  wake: (id: number) => request<void>('POST', `/devices/${id}/wake`),
  ping: (id: number) => request<{ ok: boolean; rttMs: number }>('POST', `/devices/${id}/ping`),
  portScan: (id: number) => request<{ ports: DeviceResponse['ports'] }>('POST', `/devices/${id}/portscan`),
  acknowledgeAll: () => request<{ count: number }>('POST', '/devices/acknowledge'),

  events: (opts: { before?: number; kinds?: string[]; limit?: number } = {}) => {
    const q = new URLSearchParams();
    if (opts.before) q.set('before', String(opts.before));
    if (opts.kinds?.length) q.set('kind', opts.kinds.join(','));
    if (opts.limit) q.set('limit', String(opts.limit));
    return request<EventItem[]>('GET', `/events?${q}`);
  },
  scan: () => request<void>('POST', '/scan'),

  settings: () => request<Settings>('GET', '/settings'),
  saveSettings: (s: Settings) => request<Settings>('PUT', '/settings', s),

  channels: () => request<{ channels: Channel[]; kinds: string[]; events: string[] }>('GET', '/channels'),
  createChannel: (c: Omit<Channel, 'id' | 'createdAt' | 'updatedAt'>) => request<Channel>('POST', '/channels', c),
  updateChannel: (id: number, c: Omit<Channel, 'id' | 'createdAt' | 'updatedAt'>) => request<Channel>('PUT', `/channels/${id}`, c),
  deleteChannel: (id: number) => request<void>('DELETE', `/channels/${id}`),
  testChannel: (id: number) => request<{ ok: boolean; reason?: string }>('POST', `/channels/${id}/test`),

  security: () => request<{ status: { running: boolean; lastRunAt: number; duration: number }; findings: Finding[] }>('GET', '/security'),
  runSecurity: () => request<void>('POST', '/security/run'),
  dismissFinding: (id: number, dismissed: boolean) => request<void>('POST', `/security/findings/${id}/dismiss`, { dismissed }),
  network: () => request<NetworkState>('GET', '/network'),
  wlan: () => request<WlanView>('GET', '/wlan'),
  report: (b: ReportRequest) => download('/report', b),
  acceptNetwork: (b: { ip: string } | { server: string }) => request<NetworkState>('POST', '/network/accept', b),

  internet: () => request<{ status: InternetStatus; history: { t: number; ms: number }[]; outages: { id: number; start: number; end: number; kind: 'internet' | 'router' }[]; line: LineSample | null }>('GET', '/internet'),
  system: () => request<SystemInfo>('GET', '/system'),
};
