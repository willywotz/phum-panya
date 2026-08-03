import { test, expect } from '@playwright/test';

test('landing page shows hero and three browse links', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();

  // Scoped to <main> — the public nav also has links with the same names.
  const main = page.getByRole('main');
  await expect(main.getByRole('link', { name: 'หมอพื้นบ้าน' })).toHaveAttribute('href', '/doctors');
  await expect(main.getByRole('link', { name: 'ตำรับยา' })).toHaveAttribute('href', '/recipes');
  await expect(main.getByRole('link', { name: 'สมุนไพร' })).toHaveAttribute('href', '/herbs');
});
