import { test, expect } from '@playwright/test';

test('visiting /staff while logged out redirects to /login', async ({ page }) => {
  await page.goto('/staff');
  await expect(page).toHaveURL(/\/login$/);
});

test('logging in with valid credentials lands on /staff', async ({ page }) => {
  await page.goto('/login');
  await page.getByLabel('อีเมล').fill('admin@test');
  await page.getByLabel('รหัสผ่าน').fill('pw123456');
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await expect(page).toHaveURL(/\/staff$/);
  await expect(page.getByText('แดชบอร์ดเจ้าหน้าที่')).toBeVisible();
});

test('logging in with a wrong password shows an error and stays on /login', async ({ page }) => {
  await page.goto('/login');
  await page.getByLabel('อีเมล').fill('admin@test');
  await page.getByLabel('รหัสผ่าน').fill('wrong-password');
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await expect(page.getByText('อีเมลหรือรหัสผ่านไม่ถูกต้อง')).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
});
