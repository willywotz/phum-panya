import { test, expect } from '@playwright/test';

test('language toggle switches UI labels', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('button', { name: 'เข้าสู่ระบบ' })).toBeVisible();
  await page.getByRole('button', { name: 'EN' }).click();
  await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible();
});
