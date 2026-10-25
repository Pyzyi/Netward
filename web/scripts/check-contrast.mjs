// Verifies WCAG 2.2 contrast of the design tokens in src/app.css for light and dark themes.
import { readFileSync } from 'node:fs';

const css = readFileSync(new URL('../src/app.css', import.meta.url), 'utf8');

function block(selector) {
  const i = css.indexOf(selector);
  if (i < 0) throw new Error(`selector not found: ${selector}`);
  const start = css.indexOf('{', i);
  let depth = 0;
  for (let j = start; j < css.length; j++) {
    if (css[j] === '{') depth++;
    if (css[j] === '}' && --depth === 0) return css.slice(start + 1, j);
  }
  throw new Error('unterminated block');
}

function tokens(text) {
  const out = {};
  for (const m of text.matchAll(/--([\w-]+):\s*(#[0-9a-fA-F]{6})\b/g)) out[m[1]] = m[2];
  return out;
}

const light = tokens(block(':root {'));
const dark = { ...light, ...tokens(block(":root[data-theme='dark']")) };

function lum(hex) {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
  const l = c.map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return 0.2126 * l[0] + 0.7152 * l[1] + 0.0722 * l[2];
}
function ratio(a, b) {
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
}

// [foreground, background, minimum ratio]
const pairs = [
  ['text', 'bg', 4.5], ['text', 'surface', 4.5], ['text', 'surface-2', 4.5],
  ['text-2', 'bg', 4.5], ['text-2', 'surface', 4.5], ['text-2', 'surface-2', 4.5],
  ['accent', 'surface', 4.5], ['accent', 'bg', 4.5], ['accent-text', 'accent', 4.5],
  ['ok', 'surface', 4.5], ['ok', 'bg', 4.5], ['ok', 'ok-soft', 4.5],
  ['warn', 'surface', 4.5], ['warn', 'bg', 4.5], ['warn', 'warn-soft', 4.5],
  ['danger', 'surface', 4.5], ['danger', 'danger-soft', 4.5], ['accent', 'accent-soft', 4.5],
  ['text-3', 'bg', 4.5], ['text-3', 'surface', 4.5], ['text-3', 'surface-2', 4.5],
  ['control-border', 'surface', 3.0], ['control-border', 'bg', 3.0], // WCAG 1.4.11 non-text contrast
  ['off', 'surface', 2.0], // status dots are always accompanied by text
];

let failed = 0;
for (const [name, theme] of [['light', light], ['dark', dark]]) {
  for (const [fg, bg, min] of pairs) {
    const r = ratio(theme[fg], theme[bg]);
    const ok = r >= min;
    if (!ok) failed++;
    console.log(`${ok ? 'ok  ' : 'FAIL'} ${name.padEnd(5)} ${fg.padEnd(12)} on ${bg.padEnd(12)} ${r.toFixed(2)} (min ${min})`);
  }
}
if (failed) {
  console.error(`${failed} contrast checks failed`);
  process.exit(1);
}
