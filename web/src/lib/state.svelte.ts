// Global application state, kept live through the server-sent event stream.
import { SvelteMap } from 'svelte/reactivity';
import { errorText } from './errors';
import { api, ApiError, setCsrf, setUnauthorizedHandler } from './api';
import { setLang } from './i18n.svelte';
import type { AuthState, Device, EventItem, InternetStatus, Overview, ScannerStatus, Settings } from './types';

export const auth = $state<{ loaded: boolean; state: AuthState | null }>({ loaded: false, state: null });

export const store = $state<{
  devices: SvelteMap<number, Device>;
  overview: Overview | null;
  settings: Settings | null;
  recent: EventItem[];
  connected: boolean;
  loading: boolean;
  portScans: Record<number, boolean>;
  eventTick: number;
  /** 24 h heartbeat per device: 48 half-hour slots ('1' online, '0' offline, '-' unknown). */
  strips: Record<number, string>;
  stripFrom: number;
}>({
  devices: new SvelteMap(),
  overview: null,
  settings: null,
  recent: [],
  connected: true,
  loading: true,
  portScans: {},
  eventTick: 0,
  strips: {},
  stripFrom: 0,
});

const SLOT_MS = 30 * 60_000;
const SLOTS = 48;
let stripTimer: ReturnType<typeof setInterval> | undefined;
let stripsLoading = false;
export async function loadStrips() {
  // Hidden tabs and signed-out pages must not poll: every authenticated
  // request would otherwise keep the session from idling out.
  if (stripsLoading || document.hidden || !auth.state?.authenticated) return;
  stripsLoading = true;
  try {
    const r = await api.strips();
    const next: Record<number, string> = {};
    for (const [id, st] of Object.entries(r.strips)) next[Number(id)] = st;
    // A device that came online while the request was in flight is online now.
    for (const d of store.devices.values()) {
      const st = next[d.id];
      if (st && d.online && !st.endsWith('1')) next[d.id] = st.slice(0, -1) + '1';
    }
    store.strips = next;
    store.stripFrom = r.from;
  } catch {
    /* not critical */
  } finally {
    stripsLoading = false;
  }
}

function stopStrips() {
  clearInterval(stripTimer);
  stripTimer = undefined;
  store.strips = {};
  store.stripFrom = 0;
}

/** A clock for relative times, ticking every 15 seconds. */
export const clock = $state({ now: Date.now() });
setInterval(() => (clock.now = Date.now()), 15_000);

let source: EventSource | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

export async function loadAuth() {
  try {
    const st = await api.state();
    applyAuth(st);
  } catch {
    auth.state = null;
  } finally {
    auth.loaded = true;
  }
}

export function applyAuth(st: AuthState) {
  auth.state = st;
  setCsrf(st.csrf ?? '');
  if (st.language && !localStorageLang()) setLang(st.language);
}

function localStorageLang(): string | null {
  try {
    return localStorage.getItem('netzwart.lang');
  } catch {
    return null;
  }
}

setUnauthorizedHandler(() => {
  if (auth.state) auth.state = { ...auth.state, authenticated: false, csrf: '' };
  disconnect();
  stopStrips();
});

export async function loadAll() {
  store.loading = true;
  try {
    const [overview, devices, settings, recent] = await Promise.all([
      api.overview(),
      api.devices(),
      api.settings(),
      api.events({ limit: 12 }),
    ]);
    store.overview = overview;
    store.settings = settings;
    store.recent = recent;
    store.devices.clear();
    for (const d of devices ?? []) store.devices.set(d.id, d);
    store.eventTick++; // lists built from events reload after a resync
    void loadStrips();
    clearInterval(stripTimer);
    stripTimer = setInterval(loadStrips, 5 * 60_000);
  } finally {
    store.loading = false;
  }
}

export async function refreshOverview() {
  try {
    store.overview = await api.overview();
  } catch {
    /* the next stream message will retry */
  }
}

function handle(type: string, data: unknown) {
  switch (type) {
    case 'device': {
      const d = data as Device;
      store.devices.set(d.id, d);
      // Keep the current slot of the heartbeat in step with live state; once
      // the last slot lies in the past, the strips need a fresh window.
      if (store.stripFrom && Date.now() >= store.stripFrom + SLOTS * SLOT_MS) {
        void loadStrips();
        break;
      }
      const st = store.strips[d.id];
      if (st && d.online && !st.endsWith('1')) store.strips[d.id] = st.slice(0, -1) + '1';
      break;
    }
    case 'device_removed':
      store.devices.delete((data as { id: number }).id);
      break;
    case 'event':
      store.recent = [data as EventItem, ...store.recent].slice(0, 12);
      store.eventTick++;
      break;
    case 'scanner':
      if (store.overview) store.overview.scanner = data as ScannerStatus;
      break;
    case 'internet':
      if (store.overview) store.overview.internet = data as InternetStatus;
      break;
    case 'security':
      void refreshOverview();
      break;
    case 'settings':
      store.settings = { ...(store.settings ?? {}), ...(data as Settings) };
      if (store.overview) store.overview.networkName = (data as Settings).networkName;
      break;
    case 'portscan': {
      const p = data as { id: number; running: boolean };
      store.portScans = { ...store.portScans, [p.id]: p.running };
      break;
    }
    case 'resync':
      void loadAll();
      break;
  }
}

const streamTypes = ['device', 'device_removed', 'event', 'scanner', 'internet', 'security', 'settings', 'portscan', 'mqtt', 'resync'];

/** Opens the live stream. It is closed while the tab is hidden so several
 * open tabs never exhaust the browser's six connections per host. */
export function connect() {
  if (source || document.hidden) return;
  source = new EventSource('/api/v1/stream');
  source.addEventListener('hello', () => {
    store.connected = true;
  });
  for (const type of streamTypes) {
    source.addEventListener(type, (e) => {
      try {
        handle(type, JSON.parse((e as MessageEvent).data));
      } catch {
        /* ignore malformed message */
      }
    });
  }
  source.onerror = () => {
    store.connected = false;
    disconnect();
    clearTimeout(reconnectTimer);
    reconnectTimer = setTimeout(async () => {
      try {
        const st = await api.state();
        applyAuth(st);
        if (st.authenticated) {
          await loadAll();
          connect();
        }
      } catch (e) {
        if (!(e instanceof ApiError) || e.status === 0) reconnectTimer = setTimeout(() => connect(), 5000);
      }
    }, 3000);
  };
}

export function disconnect() {
  source?.close();
  source = null;
}

document.addEventListener('visibilitychange', () => {
  if (!auth.state?.authenticated) return;
  if (document.hidden) {
    disconnect();
  } else {
    void loadAll().then(connect);
  }
});

export async function logout() {
  try {
    await api.logout();
  } finally {
    disconnect();
    stopStrips();
    store.devices.clear();
    store.overview = null;
    if (auth.state) auth.state = { ...auth.state, authenticated: false, csrf: '' };
  }
}

/** Applies fn to a copy of the settings, saves them and returns the error message, if any. */
export async function updateSettings(fn: (s: Settings) => void): Promise<string> {
  if (!store.settings) return 'not loaded';
  const copy = $state.snapshot(store.settings) as Settings;
  fn(copy);
  try {
    store.settings = await api.saveSettings(copy);
    if (store.overview) store.overview.networkName = store.settings.networkName;
    return '';
  } catch (e) {
    return errorText(e);
  }
}
