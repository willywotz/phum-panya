import { test, expect, type Page } from '@playwright/test';

async function login(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.getByLabel('อีเมล').fill(email);
  await page.getByLabel('รหัสผ่าน').fill(password);
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await expect(page).toHaveURL(/\/staff$/);
}

test('near-duplicate warning appears, then admin merges two herbs', async ({ page }) => {
  test.setTimeout(120_000);
  const stamp = Date.now();
  const base = `ฟ้าทะลายโจร${stamp}`;

  await login(page, 'admin@test', 'pw123456');
  await page.goto('/staff/herbs');

  // Create the canonical herb via the add-form.
  await page.getByLabel('ชื่อไทย (เพิ่ม)').fill(base);
  await page.getByRole('button', { name: 'บันทึกสมุนไพร' }).click();
  await expect(page.getByRole('row', { name: new RegExp(base) })).toBeVisible();

  // Typing a near-identical name shows the warning.
  await page.getByLabel('ชื่อไทย (เพิ่ม)').fill(base);
  await expect(page.getByText(/อาจซ้ำกับ/)).toBeVisible();

  // Create a second, distinct herb to merge into the first.
  const dup = `${base}-ซ้ำ`;
  await page.getByLabel('ชื่อไทย (เพิ่ม)').fill(dup);
  await page.getByRole('button', { name: 'บันทึกสมุนไพร' }).click();
  await expect(page.getByRole('row', { name: new RegExp(dup) })).toBeVisible();

  // Merge dup -> base.
  await page.getByRole('combobox', { name: 'สมุนไพรต้นทาง' }).click();
  await page.getByRole('option', { name: dup, exact: true }).click();
  await page.getByRole('combobox', { name: 'สมุนไพรหลัก' }).click();
  await page.getByRole('option', { name: base, exact: true }).click();
  await page.getByRole('button', { name: 'รวมสมุนไพร' }).click();
  await page.getByRole('button', { name: 'ยืนยัน' }).click();
  await expect(page.getByText(/รวมสมุนไพรแล้ว/)).toBeVisible();
});
