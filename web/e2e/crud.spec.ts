import { test, expect } from '@playwright/test';

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login');
  await page.getByLabel('อีเมล').fill('admin@test');
  await page.getByLabel('รหัสผ่าน').fill('pw123456');
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await expect(page).toHaveURL(/\/staff$/);
}

test('crud table adds, edits, and deletes a district', async ({ page }) => {
  const name = `E2E District ${Date.now()}`;
  const updatedName = `${name} Updated`;
  const province = 'E2E Province';

  await login(page);
  await page.goto('/staff/districts');

  // Add.
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await expect(page.getByLabel('ชื่อ')).toBeFocused();
  await page.getByLabel('ชื่อ').fill(name);
  await page.getByLabel('จังหวัด').fill(province);
  await page.getByRole('button', { name: 'บันทึก' }).click();

  const row = page.getByRole('row', { name: new RegExp(name) });
  await expect(row).toBeVisible();
  await expect(row.getByRole('cell', { name: province })).toBeVisible();

  // Edit.
  await row.getByRole('button', { name: 'แก้ไข' }).click();
  await page.getByLabel('ชื่อ').fill(updatedName);
  await page.getByRole('button', { name: 'บันทึก' }).click();

  const updatedRow = page.getByRole('row', { name: new RegExp(updatedName) });
  await expect(updatedRow).toBeVisible();
  await expect(page.getByRole('cell', { name, exact: true })).toHaveCount(0);

  // Delete, via the confirmation dialog.
  await updatedRow.getByRole('button', { name: 'ลบ' }).click();
  const dialog = page.getByRole('alertdialog');
  const describedBy = await dialog.getAttribute('aria-describedby');
  expect(describedBy).toBeTruthy();
  await expect(page.locator(`#${describedBy}`)).toHaveText('ยืนยันการลบ?');
  await dialog.getByRole('button', { name: 'ใช่' }).click();
  await expect(page.getByRole('row', { name: new RegExp(updatedName) })).toHaveCount(0);
});
