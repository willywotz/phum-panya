import { test, expect, type Page } from '@playwright/test';

import { selectByName } from './fixtures/select';

async function login(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.getByLabel('อีเมล').fill(email);
  await page.getByLabel('รหัสผ่าน').fill(password);
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await expect(page).toHaveURL(/\/staff$/);
}

async function logout(page: Page) {
  await page.goto('/staff');
  await page.getByRole('button', { name: 'ออกจากระบบ' }).click();
  await expect(page).toHaveURL(/\/login$/);
}

// Issue #12: Districts and Users are central-admin-only management screens.
// A district_editor must not see their nav links, and a direct URL hit must
// bounce to /staff instead of rendering a broken/403 page.
test('district_editor cannot see or reach the Districts/Users admin screens', async ({
  page,
}) => {
  test.setTimeout(60_000);

  const stamp = Date.now();
  const districtName = `อำเภอนำทาง${stamp}`;
  const editorEmail = `nav-editor${stamp}@test`;

  await login(page, 'admin@test', 'pw123456');
  await page.goto('/staff/districts');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('ชื่อ').fill(districtName);
  await page.getByLabel('จังหวัด').fill('E2E Province');
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(districtName) })).toBeVisible();

  await page.goto('/staff/users');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('ชื่อ-นามสกุล').fill('E2E Nav Editor');
  await page.getByLabel('อีเมล').fill(editorEmail);
  await page.getByLabel('รหัสผ่าน').fill('pw123456');
  await selectByName(page, 'บทบาท', 'ผู้แก้ไขข้อมูลอำเภอ');
  await page.getByRole('combobox', { name: 'อำเภอ' }).click();
  await expect(page.getByRole('option', { name: districtName, exact: true })).toHaveCount(1);
  await page.getByRole('option', { name: districtName, exact: true }).click();
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(editorEmail) })).toBeVisible();

  await logout(page);
  await login(page, editorEmail, 'pw123456');

  await expect(page.getByRole('link', { name: 'อำเภอ' })).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'ผู้ใช้งาน' })).toHaveCount(0);

  await page.goto('/staff/users');
  await expect(page).toHaveURL(/\/staff$/);
});
