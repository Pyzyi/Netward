// Wi-Fi map (docs/10, F4): measuring in the browser, preparing the floor
// plan image, colours and advice sentences.
import { ApiError, writeHeaders } from './api';
import { i18n, number, t } from './i18n.svelte';
import type { WifiHint } from './types';

export type Phase = 'latency' | 'download' | 'upload';
export interface Measurement {
  downMbps: number;
  upMbps: number;
  latencyMs: number;
  jitterMs: number;
}

const base = '/api/v1/wifimap';
const median = (v: number[]) => {
  const s = [...v].sort((a, b) => a - b);
  return s.length % 2 ? s[(s.length - 1) / 2] : (s[s.length / 2 - 1] + s[s.length / 2]) / 2;
};

async function check(res: Response) {
  if (!res.ok) throw new ApiError(res.status, res.status === 429 ? 'busy' : 'internal');
  return res;
}

/** Latency: ten small requests after one warm-up; jitter is the mean change between them. */
async function latency(signal: AbortSignal): Promise<{ latencyMs: number; jitterMs: number }> {
  const times: number[] = [];
  for (let i = 0; i < 11; i++) {
    const t0 = performance.now();
    await check(await fetch(`${base}/ping`, { cache: 'no-store', credentials: 'same-origin', signal }));
    if (i > 0) times.push(performance.now() - t0);
  }
  let jitter = 0;
  for (let i = 1; i < times.length; i++) jitter += Math.abs(times[i] - times[i - 1]);
  return { latencyMs: median(times), jitterMs: jitter / (times.length - 1) };
}

/** Download: read a stream for about three seconds; the first half second (slow start) is not counted. */
async function download(signal: AbortSignal, progress: (f: number) => void): Promise<number> {
  const ctl = new AbortController();
  const stop = () => ctl.abort();
  signal.addEventListener('abort', stop);
  try {
    const res = await check(await fetch(`${base}/download?bytes=${64 << 20}`, { cache: 'no-store', credentials: 'same-origin', signal: ctl.signal }));
    const reader = res.body!.getReader();
    const t0 = performance.now();
    let from = 0;
    let bytes = 0;
    let total = 0;
    let at = t0;
    for (;;) {
      const { done, value } = await reader.read();
      const now = performance.now();
      if (done) break;
      total += value.byteLength;
      if (now - t0 < 500) {
        from = now;
      } else {
        bytes += value.byteLength;
        at = now;
      }
      progress(Math.min(1, (now - t0) / 3500));
      if (now - t0 > 3500) break;
    }
    const end = performance.now();
    void reader.cancel().catch(() => {});
    const secs = (at - (from || t0)) / 1000;
    if (bytes > 0 && secs > 0) return (bytes * 8) / secs / 1e6;
    // Finished within the warm-up (a very fast link): count everything.
    return end > t0 ? (total * 8) / ((end - t0) / 1000) / 1e6 : 0;
  } finally {
    signal.removeEventListener('abort', stop);
    ctl.abort();
  }
}

let noise: Uint8Array<ArrayBuffer> | undefined;
function payload(size: number): Blob {
  if (!noise) {
    noise = new Uint8Array(1 << 20);
    for (let i = 0; i < noise.length; i += 65536) crypto.getRandomValues(noise.subarray(i, i + 65536));
  }
  const parts: BlobPart[] = [];
  for (let left = size; left > 0; left -= noise.length) parts.push(noise.subarray(0, Math.min(left, noise.length)));
  return new Blob(parts, { type: 'application/octet-stream' });
}

/** Upload: growing requests until about three seconds are measured. */
async function upload(signal: AbortSignal, progress: (f: number) => void): Promise<number> {
  let size = 256 << 10;
  let bytes = 0;
  let ms = 0;
  let first = true;
  const start = performance.now();
  while (performance.now() - start < 3500) {
    const t0 = performance.now();
    await check(await fetch(`${base}/upload`, { method: 'POST', headers: writeHeaders(), body: payload(size), credentials: 'same-origin', signal }));
    const dt = performance.now() - t0;
    if (!first || performance.now() - start > 2000) {
      bytes += size;
      ms += dt;
    }
    first = false;
    progress(Math.min(1, (performance.now() - start) / 3500));
    size = Math.min(32 << 20, Math.max(256 << 10, Math.round((size * 1000) / Math.max(dt, 1))));
  }
  return ms > 0 ? (bytes * 8) / (ms / 1000) / 1e6 : 0;
}

/** Runs the whole measurement between this browser and Netzwart. */
export async function measure(signal: AbortSignal, onPhase: (p: Phase, f: number) => void): Promise<Measurement> {
  onPhase('latency', 0);
  const l = await latency(signal);
  onPhase('download', 0);
  const downMbps = await download(signal, (f) => onPhase('download', f));
  onPhase('upload', 0);
  const upMbps = await upload(signal, (f) => onPhase('upload', f));
  const r = (v: number) => Math.round(v * 10) / 10;
  return { downMbps: r(downMbps), upMbps: r(upMbps), latencyMs: r(l.latencyMs), jitterMs: r(l.jitterMs) };
}

/**
 * Shrinks a photo or drawing to at most 2400 px and re-encodes it. That
 * keeps uploads small, turns HEIC from iPhones into JPEG and drops the
 * location data that photos carry.
 */
export async function preparePlan(file: File): Promise<Blob> {
  const src = await decode(file);
  const w0 = src.width;
  const h0 = src.height;
  const scale = Math.min(1, 2400 / Math.max(w0, h0));
  const canvas = document.createElement('canvas');
  canvas.width = Math.max(16, Math.round(w0 * scale));
  canvas.height = Math.max(16, Math.round(h0 * scale));
  const ctx = canvas.getContext('2d')!;
  ctx.fillStyle = '#fff'; // transparent drawings get a white background
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.drawImage(src, 0, 0, canvas.width, canvas.height);
  if ('close' in src) src.close();
  const type = file.type === 'image/png' ? 'image/png' : 'image/jpeg';
  const blob = await new Promise<Blob | null>((ok) => canvas.toBlob(ok, type, 0.85));
  if (!blob) throw new Error('encode');
  if (blob.size > 7.5 * 1024 * 1024 && type === 'image/png') {
    const jpg = await new Promise<Blob | null>((ok) => canvas.toBlob(ok, 'image/jpeg', 0.85));
    if (jpg) return jpg;
  }
  return blob;
}

// createImageBitmap needs no URL; the fallback uses a data: URL because
// the CSP allows no blob: images.
async function decode(file: File): Promise<ImageBitmap | HTMLImageElement> {
  try {
    return await createImageBitmap(file, { imageOrientation: 'from-image' });
  } catch {
    const url = await new Promise<string>((ok, fail) => {
      const r = new FileReader();
      r.onload = () => ok(String(r.result));
      r.onerror = () => fail(r.error);
      r.readAsDataURL(file);
    });
    const img = new Image();
    img.src = url;
    await img.decode();
    return img;
  }
}

// Heatmap colours: share of the best spot. Same in light and dark mode,
// the map is drawn on the plan image.
const stops: [number, [number, number, number]][] = [
  [0, [196, 52, 44]],
  [0.4, [226, 155, 18]],
  [0.75, [31, 166, 106]],
];
export function rgb(share: number): [number, number, number] {
  const s = Math.max(0, Math.min(1, share));
  for (let i = 1; i < stops.length; i++) {
    const [p1, c1] = stops[i];
    const [p0, c0] = stops[i - 1];
    if (s <= p1) {
      const f = (s - p0) / (p1 - p0);
      return [0, 1, 2].map((k) => Math.round(c0[k] + (c1[k] - c0[k]) * f)) as [number, number, number];
    }
  }
  return stops[stops.length - 1][1];
}
export const level = (share: number) => (share < 0.4 ? 'weak' : share < 0.75 ? 'fair' : 'good');

/** "im Flur", "in der Küche": German needs the article; other names stay quoted. */
function inRoom(room: string): string {
  if (!/^[\p{L}-]+$/u.test(room)) return `bei „${room}“`;
  const fem = /(küche|diele|garage|kammer|terrasse|werkstatt|galerie|loggia|veranda|toilette|halle|stube|ecke|etage)$/i;
  return (fem.test(room) ? 'in der ' : 'im ') + room;
}
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

export const mbps = (v: number) => number(v >= 100 ? Math.round(v) : Math.round(v * 10) / 10);

/** One sentence of advice. */
export function hintText(h: WifiHint): string {
  const de = i18n.lang === 'de';
  const room = h.room || t('wifimap.unnamed');
  const vars = {
    room,
    In: h.room ? cap(inRoom(h.room)) : t('wifimap.here'),
    via: h.via ?? '',
    at: h.via ? (de ? inRoom(h.via) : h.via) : '',
    best: h.best || t('wifimap.bestSpot'),
    down: mbps(h.downMbps),
    latency: Math.round(h.latencyMs ?? 0),
    jitter: Math.round(h.jitterMs ?? 0),
  };
  if (h.kind === 'good') return t('wifimap.hint.good', vars);
  if (h.kind === 'unstable') return t('wifimap.hint.unstable', vars);
  const key = h.via ? 'wifimap.hint.weak' : h.best ? 'wifimap.hint.weakBetween' : 'wifimap.hint.weakRouter';
  return t(key, vars) + (h.band24 ? ' ' + t('wifimap.hint.band24') : '');
}
