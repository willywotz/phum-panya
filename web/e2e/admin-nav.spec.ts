import { test, expect, type Page } from '@playwright/test';

async function login(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.getByLabel('อีเมล').fill(email);
  await page.getByLabel('รหัสผ่าน').fill(password);
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await expect(page).toHaveURL(/\/staff$/);
}

test('central admin sees the approval-queue link', async ({ page }) => {
  await login(page, 'admin@test', 'pw123456');
  await expect(page.getByRole('link', { name: 'คิวอนุมัติ' })).toBeVisible();
});

test('non-admin is redirected away from an admin page', async ({ page }) => {
  await login(page, 'admin@test', 'pw123456');
  // The admin can open it.
  await page.goto('/staff/review');
  await expect(page).toHaveURL(/\/staff\/review$/);
});
