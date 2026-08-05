import { test, expect, type Page } from '@playwright/test';

async function login(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.getByLabel('อีเมล').fill(email);
  await page.getByLabel('รหัสผ่าน').fill(password);
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await expect(page).toHaveURL(/\/staff$/);
}

test('admin locks then unlocks a data year', async ({ page }) => {
  test.setTimeout(60_000);
  // A year unlikely to collide with other specs' data.
  const year = 2400 + (Date.now() % 150);
  await login(page, 'admin@test', 'pw123456');
  await page.goto('/staff/year-locks');
  await page.getByLabel('ปีข้อมูล').fill(String(year));
  await page.getByRole('button', { name: 'ล็อกปี' }).click();
  const row = page.getByRole('row', { name: new RegExp(String(year)) });
  await expect(row).toBeVisible();
  await row.getByRole('button', { name: 'ปลดล็อก' }).click();
  await page.getByRole('button', { name: 'ยืนยัน' }).click();
  await expect(page.getByRole('row', { name: new RegExp(String(year)) })).toHaveCount(0);
});
