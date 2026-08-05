import path from 'node:path';

import { test, expect, type Page } from '@playwright/test';

const template = path.join(__dirname, 'fixtures', 'import-template.xlsx');

async function login(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.getByLabel('อีเมล').fill(email);
  await page.getByLabel('รหัสผ่าน').fill(password);
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await expect(page).toHaveURL(/\/staff$/);
}

test('admin dry-runs, commits, then undoes an import', async ({ page }) => {
  test.setTimeout(120_000);
  await login(page, 'admin@test', 'pw123456');
  await page.goto('/staff/imports');
  await page.getByLabel('ไฟล์ Excel').setInputFiles(template);
  await page.getByRole('button', { name: 'ตรวจสอบ' }).click();
  await expect(page.getByText(/หมอใหม่/)).toBeVisible();
  await page.getByRole('button', { name: 'ยืนยันนำเข้า' }).click();
  await expect(page.getByRole('button', { name: 'ยกเลิกชุดนำเข้า' })).toBeVisible();
  await page.getByRole('button', { name: 'ยกเลิกชุดนำเข้า' }).click();
  await page.getByRole('button', { name: 'ยืนยัน' }).click();
  await expect(page.getByText(/ยกเลิกแล้ว/)).toBeVisible();
});
