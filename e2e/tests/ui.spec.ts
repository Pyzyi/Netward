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

test('no horizontal overflow on any page', async ({ page }) => {
  await login(page);
  for (const path of ['/', '/devices', '/devices/5', '/activity', '/security', '/settings/scanning']) {
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
