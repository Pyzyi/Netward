import { test, expect, type Page } from '@playwright/test';

const PASSWORD = 'Sonnenblume-im-Garten7';

// Collects console errors and CSP violations so every test can assert none happened.
function watch(page: Page) {
  const problems: string[] = [];
  page.on('console', (m) => {
    if (m.type() === 'error') problems.push(m.text());
  });
  page.on('pageerror', (e) => problems.push(e.message));
  return problems;
}

async function login(page: Page) {
  await page.goto('/');
  const state = await page.request.get('/api/v1/auth/state').then((r) => r.json());
  if (state.setupRequired) {
    await page.getByLabel('Benutzername').fill('admin');
    await page.getByLabel('Passwort', { exact: true }).fill(PASSWORD);
    await page.getByLabel('Passwort wiederholen').fill(PASSWORD);
    await page.getByRole('button', { name: 'Weiter' }).click();
    await page.getByLabel('Name des Netzwerks').fill('Zuhause');
    await page.getByRole('button', { name: 'Einrichtung abschließen' }).click();
  } else if (!state.authenticated) {
    await page.getByLabel('Benutzername').fill('admin');
    await page.getByLabel('Passwort').fill(PASSWORD);
    await page.getByRole('button', { name: 'Anmelden' }).click();
  }
  // The app shell (with its main landmark) only renders once signed in.
  await expect(page.locator('main#main')).toBeVisible();
}

test.describe.configure({ mode: 'serial' });

test('setup or login, overview shows live counts', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await expect(page.getByText('Braucht Aufmerksamkeit')).toBeVisible();
  await expect(page.getByText('Letzte Aktivität')).toBeVisible();
  expect(problems).toEqual([]);
});

test('devices: search, open detail, rename, close', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/devices');
  await page.getByRole('searchbox').fill('drucker');
  const row = page.locator('a.main', { hasText: 'Drucker' }).first();
  await expect(row).toBeVisible();
  await row.click();
  const name = page.getByLabel('Namen vergeben');
  await expect(name).toBeVisible();
  await name.fill('Laserdrucker Büro');
  await name.press('Enter');
  await expect(page.locator('a.main', { hasText: 'Laserdrucker Büro' })).toBeVisible();
  // restore
  await name.fill('Drucker');
  await name.press('Enter');
  await page.keyboard.press('Escape');
  await expect(page).toHaveURL(/\/devices(\?.*)?$/);
  expect(problems).toEqual([]);
});

// Review 2 (UX H1): names stay readable in the phone layout.
test('device names are readable at every width', async ({ page }) => {
  await login(page);
  await page.goto('/devices');
  const name = page.locator('a.main .names').first();
  await expect(name).toBeVisible();
  const box = await name.boundingBox();
  expect(box!.width).toBeGreaterThan(100);
});

// Review 2 (UX H4): keyboard focus moves into the detail panel and back.
test('keyboard: open detail, escape returns focus to the row', async ({ page }) => {
  await login(page);
  await page.goto('/devices');
  const row = page.locator('a.main').nth(2);
  await row.focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('aside.detail h2')).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(page.locator('aside.detail')).toHaveCount(0);
  await expect(page.locator('a.main').nth(2)).toBeFocused();
});

// 0.1: software and support.
test('software: outdated devices, explanation and manual version', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/devices?filter=software');
  const pc = page.locator('a.main', { hasText: 'Gaming-PC' });
  await expect(pc).toBeVisible();
  await expect(pc.getByText('Veraltet')).toBeVisible();
  await expect(page.locator('a.main', { hasText: 'Drucker' })).toHaveCount(0);
  await pc.click();
  const detail = page.locator('aside.detail');
  await expect(detail.getByText('Keine Sicherheitsupdates mehr seit 14. Oktober 2025')).toBeVisible();
  await expect(detail.getByText('Windows 10 22H2')).toBeVisible();
  await expect(detail.getByText('Was kannst du tun?')).toBeVisible();
  await page.keyboard.press('Escape');

  // iPhones hide their version: the user can enter it, and remove it again.
  await page.goto('/devices');
  await page.locator('a.main', { hasText: "Max' iPhone" }).click();
  await expect(detail.getByText('iPhones und iPads verraten ihre iOS-Version nicht')).toBeVisible();
  await detail.getByRole('button', { name: 'Systemversion angeben' }).click();
  await detail.getByLabel('Hauptversion').selectOption('18');
  await detail.getByRole('button', { name: 'Speichern' }).click();
  await expect(detail.getByText('das Ende ist absehbar')).toBeVisible();
  await detail.getByRole('button', { name: 'Angabe ändern' }).click();
  await detail.getByRole('button', { name: 'Angabe löschen' }).click();
  await expect(detail.getByText('Software unbekannt')).toBeVisible();
  expect(problems).toEqual([]);
});

test('settings: saved values become the new baseline', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/settings/general');
  const name = page.locator('#nn');
  const card = page.locator('.block').filter({ has: name });
  await name.fill('Zuhause 2');
  await card.getByRole('button', { name: 'Speichern' }).click();
  await expect(card.getByRole('button', { name: 'Speichern' })).toHaveCount(0);
  // Going back to the old value is a change again and must be savable.
  await name.fill('Zuhause');
  await card.getByRole('button', { name: 'Speichern' }).click();
  await expect(card.getByRole('button', { name: 'Speichern' })).toHaveCount(0);
  expect(problems).toEqual([]);
});

test('security, activity and settings pages render', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  for (const [path, heading] of [
    ['/security', 'Sicherheit'],
    ['/activity', 'Aktivität'],
    ['/settings/notifications', 'Einstellungen'],
  ]) {
    await page.goto(path);
    await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible();
  }
  expect(problems).toEqual([]);
});

test('second DHCP server is explained with its device', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/security');
  const item = page.locator('li.finding', { hasText: 'Ein weiteres Gerät verteilt Adressen (192.168.178.80)' });
  await expect(item).toBeVisible();
  await expect(item.getByText('Raspberry Pi')).toBeVisible();
  await expect(item.getByText('Router 192.168.178.1 · DNS 192.168.178.80')).toBeVisible();
  // Demo mode is read-only.
  await expect(item.getByRole('button', { name: 'Diesen Server kenne ich' })).toBeDisabled();
  await page.goto('/activity');
  await page.getByRole('button', { name: 'Sicherheit' }).click(); // logins of other tests fill the first page
  await expect(page.getByText('Adressverteiler (DHCP-Server) entdeckt: 192.168.178.80')).toBeVisible();
  expect(problems).toEqual([]);
});

test('Wi-Fi diagnosis names the problem and the device', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/devices');
  await page.getByRole('link', { name: 'WLAN-Diagnose' }).click();
  await expect(page.getByRole('heading', { level: 1, name: 'WLAN' })).toBeVisible();
  await expect(page.getByText('Laptop Kinder hängt im 2,4-GHz-Band am Repeater „Repeater OG“, obwohl es 5 GHz kann.')).toBeVisible();
  await expect(page.getByText(/Der Repeater „Repeater OG“ ist selbst nur schwach angebunden/)).toBeVisible();
  await page.getByRole('link', { name: 'Laptop Kinder' }).click();
  const detail = page.locator('section', { has: page.getByRole('heading', { name: 'WLAN' }) });
  await expect(detail.getByText('Repeater OG', { exact: true })).toBeVisible();
  await expect(detail.locator('dd', { hasText: 'Schwaches Signal' })).toBeVisible();
  expect(problems).toEqual([]);
});

test('provider report downloads as PDF and CSV', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/settings/internet');
  await expect(page.getByText('DSL · synchronisiert mit')).toBeVisible();
  await page.getByLabel('Download Minimal').fill('70');
  const [pdf] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Bericht als PDF' }).click()]);
  expect(pdf.suggestedFilename()).toMatch(/^netzwart-nachweis-\d{4}-\d{2}-\d{2}-\d{4}-\d{2}-\d{2}\.pdf$/);
  const head = (await import('node:fs')).readFileSync(await pdf.path()).subarray(0, 8).toString();
  expect(head).toBe('%PDF-1.4');
  const [csv] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Rohdaten (CSV)' }).click()]);
  expect(csv.suggestedFilename()).toMatch(/\.csv$/);
  expect(problems).toEqual([]);
});

test('known exploited vulnerabilities: detected version and manual entry', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/devices/7'); // MacBook-Air, macOS 26.6 from AirPlay
  await expect(page.getByText('Update dringend: Lücken werden bereits ausgenutzt')).toBeVisible();
  await expect(page.getByText(/^CVE-\d{4}-\d+$/).first()).toBeVisible();
  await expect(page.getByText(/fehlt ein Eintrag, heißt das nicht, dass das Gerät sicher ist/)).toBeVisible();
  await page.goto('/devices/6'); // Annas iPhone, entered by hand
  await expect(page.getByText(/Laut deiner Angabe \(26\.6\.2\)/)).toBeVisible();
  expect(problems).toEqual([]);
});

test('persons: who is at home, consent for time per day', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/');
  const home = page.locator('section', { has: page.getByRole('heading', { name: 'Wer ist zu Hause' }) });
  await expect(home.getByText('Max', { exact: true })).toBeVisible();
  await expect(home.getByText(/^zu Hause seit /).first()).toBeVisible();
  await home.getByRole('link', { name: 'Verwalten' }).click();
  await expect(page.getByRole('heading', { level: 1, name: 'Personen' })).toBeVisible();
  // Anna agreed: hours per day are shown. Max did not: switching on asks first.
  await expect(page.getByText('Stunden je Tag, in denen mindestens ein Anwesenheitsgerät verbunden war.')).toBeVisible();
  const max = page.locator('section.person', { has: page.getByRole('heading', { name: /Max/ }) });
  await max.getByRole('switch', { name: 'Zeit im Heimnetz je Tag zeigen' }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByText('Max weiß Bescheid und ist einverstanden.')).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Einschalten' })).toBeDisabled();
  await dialog.getByRole('button', { name: 'Abbrechen' }).click();
  await expect(max.getByRole('switch', { name: 'Zeit im Heimnetz je Tag zeigen' })).toHaveAttribute('aria-checked', 'false');
  expect(problems).toEqual([]);
});

test('identity change of a known device is explained with two answers', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/security');
  const item = page.locator('li.finding', { hasText: 'Laptop Kinder meldet sich plötzlich als Linux-Gerät' });
  await expect(item).toBeVisible();
  await expect(item.getByRole('button', { name: 'Neu eingerichtet' })).toBeDisabled(); // demo mode
  await expect(item.getByRole('button', { name: 'Kenne ich nicht' })).toBeVisible();
  expect(problems).toEqual([]);
});

test('no horizontal overflow on any page', async ({ page }) => {
  await login(page);
  for (const path of ['/', '/devices', '/devices/5', '/activity', '/security', '/wlan', '/settings/scanning', '/settings/internet', '/persons']) {
    await page.goto(path);
    await page.waitForTimeout(400);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, `horizontal overflow on ${path}`).toBeLessThanOrEqual(0);
  }
});

test('security headers and API protections', async ({ request, baseURL }) => {
  const res = await request.get('/');
  expect(res.headers()['content-security-policy']).toContain("script-src 'self'");
  expect(res.headers()['x-frame-options']).toBe('DENY');
  const unauth = await request.get('/api/v1/devices', { headers: { cookie: '' } });
  expect(unauth.status()).toBe(401);
  const foreign = await request.post('/api/v1/auth/login', {
    headers: { Origin: 'http://evil.example', 'Content-Type': 'application/json' },
    data: { username: 'admin', password: PASSWORD },
  });
  expect(foreign.status()).toBe(403);
  expect(baseURL).toBeTruthy();
});

test('device history: milestones, purchase and warranty', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/devices');
  await page.locator('a.main', { hasText: 'FRITZ!Box' }).first().click();
  const detail = page.locator('aside.detail');
  const life = detail.locator('section', { has: page.getByRole('heading', { name: 'Lebenslauf' }) });
  await expect(life.getByText('Software aktualisiert: 8.00 → 8.20')).toBeVisible();
  await expect(life.getByText('47 min nicht erreichbar')).toBeVisible();
  await expect(life.getByText(/^Garantie endet · in /)).toBeVisible();
  await expect(detail.getByLabel('Seriennummer')).toHaveValue('A123.456.78.901.234');
  expect(problems).toEqual([]);
});

test('LAN attack detection: router, DHCP and DNS at a glance', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/security');
  const ids = page.locator('section.ids');
  await expect(ids.getByRole('heading', { name: 'Angriffserkennung im LAN' })).toBeVisible();
  await expect(ids.getByText('192.168.178.80 antwortet korrekt und blockiert dns.google (Filter wie Pi-hole).')).toBeVisible();
  await expect(ids.getByText(/192\.168\.178\.1 gehört seit .* zu AVM/)).toBeVisible();
  expect(problems).toEqual([]);
});

test('tripwire: alarm, status and settings', async ({ page }) => {
  const problems = watch(page);
  await login(page);
  await page.goto('/security');
  await expect(page.getByRole('heading', { name: /ESP_9E3BD1 hat den Stolperdraht ausgelöst/ })).toBeVisible();
  const ids = page.locator('section.ids');
  await expect(ids.getByText(/Köder auf 2323, 3389, 5555, 5900, 8081\. Zuletzt ausgelöst .* von 192\.168\.178\.90 \(Telnet\)\./)).toBeVisible();
  await page.goto('/activity');
  await expect(page.getByText('ESP_9E3BD1 (192.168.178.90) hat den Stolperdraht ausgelöst: Telnet auf Port 2323').first()).toBeVisible();
  await page.goto('/settings/scanning');
  const ports = page.getByLabel('Ports', { exact: true });
  await expect(ports).toHaveValue('2323, 3389, 5555, 5900, 8081');
  await ports.fill('23, 3389');
  await page.getByRole('button', { name: 'Speichern' }).last().click();
  await expect(page.getByText('Bitte nur Zahlen von 1024 bis 65535 eintragen, jede nur einmal.')).toBeVisible();
  await ports.fill('2323, 3389, 5555, 5900, 8081');
  expect(problems).toEqual([]);
});

test('Wi-Fi map: hints, measuring, own floor plan', async ({ page }) => {
  test.setTimeout(60_000);
  const problems = watch(page);
  await login(page);
  await page.goto('/wlan');
  await page.getByRole('link', { name: 'WLAN-Karte' }).click();
  await expect(page.getByRole('heading', { name: 'WLAN-Karte', level: 1 })).toBeVisible();
  await expect(page.getByText('Im Kinderzimmer nur 38 Mbit/s, ein Verstärker im Flur würde helfen.')).toBeVisible();
  await expect(page.getByText(/Im Schlafzimmer schwankt die Verbindung/)).toBeVisible();

  // Measure at a spot (demo mode accepts measurements from this machine).
  await page.locator('.map').click({ position: { x: 40, y: 40 } });
  await page.getByLabel('Raum').fill('Testecke');
  await page.getByRole('button', { name: 'Hier messen' }).click();
  await expect(page.getByText(/^Testecke: .* Mbit\/s Download/)).toBeVisible({ timeout: 30_000 });
  await page.getByRole('button', { name: 'Messpunkt Testecke löschen' }).click();
  await expect(page.getByRole('button', { name: 'Messpunkt Testecke löschen' })).toHaveCount(0);

  // Upload a second floor plan, then delete it again.
  const png = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAIAAAD8GO2jAAAAKklEQVR42u3NMQEAAAjDMMC/52ECvlRA00nqs3sHAgQIECBAgAABAgQIEGcBpwYBX4bJfoAAAAAASUVORK5CYII=',
    'base64',
  );
  await page.getByLabel('Name').last().fill('Keller');
  await page.locator('input[type=file]').last().setInputFiles({ name: 'keller.png', mimeType: 'image/png', buffer: png });
  await expect(page.getByRole('heading', { name: 'Keller' })).toBeVisible();
  await page.getByRole('button', { name: 'Löschen' }).first().click();
  await page.getByRole('dialog').getByRole('button', { name: 'Löschen' }).click();
  await expect(page.getByRole('heading', { name: 'Keller' })).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Wohnung' })).toBeVisible();
  expect(problems).toEqual([]);
});
