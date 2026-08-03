import AxeBuilder from '@axe-core/playwright';
import { test, expect, type Page } from '@playwright/test';

async function login(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.getByLabel('อีเมล').fill(email);
  await page.getByLabel('รหัสผ่าน').fill(password);
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await expect(page).toHaveURL(/\/staff$/);
}

// SRS UAT 10: a district_editor can export their scoped data as CSV/Excel.
test('district editor sees a scoped CSV export link on the doctors page', async ({
  page,
  request,
}) => {
  const stamp = Date.now();
  const districtName = `ExportDistrict${stamp}`;
  const editorEmail = `export-editor${stamp}@test`;

  const loginRes = await request.post('/api/login', {
    data: { email: 'admin@test', password: 'pw123456' },
  });
  expect(loginRes.ok()).toBeTruthy();

  const districtRes = await request.post('/api/districts', {
    data: { name: districtName, province: 'Test' },
  });
  const { ID: districtId } = await districtRes.json();

  await request.post('/api/users', {
    data: {
      full_name: 'Export Editor',
      email: editorEmail,
      password: 'pw123456',
      role: 'district_editor',
      district_id: districtId,
    },
  });
  await request.post('/api/logout');

  await login(page, editorEmail, 'pw123456');
  await page.goto('/staff/doctors');

  const exportCsv = page.getByRole('link', { name: 'ส่งออก CSV' });
  await expect(exportCsv).toHaveAttribute('href', /\/api\/export\/doctors\.csv/);

  const downloadPromise = page.waitForEvent('download');
  await exportCsv.click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe('doctors.csv');
});

test('storage meter shows used bytes for central_admin', async ({ page }) => {
  await login(page, 'admin@test', 'pw123456');
  await page.goto('/staff');
  await expect(page.getByText(/\d+(\.\d+)? MB/)).toBeVisible();
});

// NFR-A11Y-1 (best-effort): no critical/serious axe violations on public pages.
test('public home and doctor detail pages have no critical/serious a11y violations', async ({
  page,
  request,
}) => {
  const stamp = Date.now();
  await request.post('/api/login', {
    data: { email: 'admin@test', password: 'pw123456' },
  });
  const districtRes = await request.post('/api/districts', {
    data: { name: `A11yDistrict${stamp}`, province: 'Test' },
  });
  const { ID: districtId } = await districtRes.json();
  const doctorRes = await request.post('/api/doctors', {
    data: {
      code: `A11Y${stamp}`,
      full_name: `A11y Doctor ${stamp}`,
      district_id: districtId,
      status: 'active',
      consent_obtained: true,
    },
  });
  const { ID: doctorId } = await doctorRes.json();
  await request.post('/api/logout');

  await page.goto('/');
  const homeViolations = (await new AxeBuilder({ page }).analyze()).violations.filter(
    (v) => v.impact === 'critical' || v.impact === 'serious',
  );
  expect(homeViolations).toEqual([]);

  await page.goto(`/doctor?id=${doctorId}`);
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  const doctorViolations = (await new AxeBuilder({ page }).analyze()).violations.filter(
    (v) => v.impact === 'critical' || v.impact === 'serious',
  );
  expect(doctorViolations).toEqual([]);
});
