import { test, expect } from '@playwright/test';

test('theme toggle switches between light and dark', async ({ page }) => {
  await page.goto('/');
  const html = page.locator('html');
  await expect(html).not.toHaveClass(/dark/);

  await page.getByRole('button', { name: 'ธีม' }).click();
  await expect(html).toHaveClass(/dark/);

  await page.getByRole('button', { name: 'ธีม' }).click();
  await expect(html).not.toHaveClass(/dark/);
});
