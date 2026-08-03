import path from 'node:path';

import { test, expect, type Page } from '@playwright/test';

import { selectByName } from './fixtures/select';

const photoFixture = path.join(__dirname, 'fixtures', 'test-photo.png');

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

// SRS UAT 1-6: admin seeds a catalog herb, a district, and a district_editor;
// the editor creates a doctor (with a photo) and a recipe with one catalog
// and one pending ingredient; the admin reconciles the pending herb; the
// editor adds a case. Every entity must appear in its list.
test('staff flow: editor creates doctor, recipe, and case; admin reconciles a pending herb', async ({
  page,
}) => {
  test.setTimeout(120_000);

  const stamp = Date.now();
  const herbName = `ขมิ้นชัน${stamp}`;
  const pendingHerbName = `สมุนไพรทดสอบ${stamp}`;
  const districtName = `อำเภอทดสอบ${stamp}`;
  const editorEmail = `editor${stamp}@test`;
  const doctorCode = `DOC${stamp}`;
  const doctorName = `หมอทดสอบ${stamp}`;
  const recipeCode = `REC${stamp}`;
  const recipeName = `ตำรับทดสอบ${stamp}`;

  // Admin: seed a catalog herb.
  await login(page, 'admin@test', 'pw123456');
  await page.goto('/staff/herbs');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('ชื่อไทย').fill(herbName);
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(herbName) })).toBeVisible();

  // Admin: create a district.
  await page.goto('/staff/districts');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('ชื่อ').fill(districtName);
  await page.getByLabel('จังหวัด').fill('E2E Province');
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(districtName) })).toBeVisible();

  // Admin: create a district_editor user for that district.
  await page.goto('/staff/users');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('ชื่อ-นามสกุล').fill('E2E Editor');
  await page.getByLabel('อีเมล').fill(editorEmail);
  await page.getByLabel('รหัสผ่าน').fill('pw123456');
  await selectByName(page, 'บทบาท', 'ผู้แก้ไขข้อมูลอำเภอ');
  await page.getByRole('combobox', { name: 'อำเภอ' }).click();
  await expect(page.getByRole('option', { name: districtName, exact: true })).toHaveCount(1);
  await page.getByRole('option', { name: districtName, exact: true }).click();
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(editorEmail) })).toBeVisible();

  // Editor: create a doctor in their own district.
  await logout(page);
  await login(page, editorEmail, 'pw123456');
  await page.goto('/staff/doctors');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('รหัส').fill(doctorCode);
  await page.getByLabel('ชื่อ-นามสกุล').fill(doctorName);
  await selectByName(page, 'สถานะ', 'ใช้งาน');
  await page.getByRole('button', { name: 'บันทึก' }).click();
  const doctorRow = page.getByRole('row', { name: new RegExp(doctorName) });
  await expect(doctorRow).toBeVisible();

  // Editor: attempt a doctor photo upload (edit mode only).
  await doctorRow.getByRole('button', { name: 'แก้ไข' }).click();
  await page.setInputFiles('#photo-upload', photoFixture);
  await expect(page.getByText(/\.jpg$/)).toBeVisible();
  await page.getByRole('button', { name: 'ยกเลิก' }).click();

  // Editor: add a recipe under that doctor with one catalog herb and one
  // pending herb ingredient.
  await page.goto('/staff/recipes');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('รหัส').fill(recipeCode);
  await page.getByLabel('ชื่อ').fill(recipeName);
  await page.getByLabel('ปีข้อมูล').fill('2024');

  // Radix renders the option listbox in a page-root portal, so the trigger
  // click is scoped to the row but the option click is scoped to the page.
  const ingredientRow1 = page.locator('fieldset').nth(0);
  await ingredientRow1.getByRole('combobox', { name: 'สมุนไพร' }).click();
  await page.getByRole('option', { name: herbName, exact: true }).click();
  await ingredientRow1.getByLabel('ปริมาณ').fill('10');
  await ingredientRow1.getByLabel('หน่วย').fill('กรัม');

  await page.getByRole('button', { name: 'เพิ่มส่วนประกอบ' }).click();
  const ingredientRow2 = page.locator('fieldset').nth(1);
  await ingredientRow2.getByRole('combobox', { name: 'สมุนไพร' }).click();
  await page.getByRole('option', { name: 'อื่นๆ (พิมพ์ชื่อ)', exact: true }).click();
  await ingredientRow2.getByLabel('ชื่อสมุนไพร').fill(pendingHerbName);
  await ingredientRow2.getByLabel('ปริมาณ').fill('5');
  await ingredientRow2.getByLabel('หน่วย').fill('กรัม');

  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(recipeName) })).toBeVisible();

  // Admin: reconcile the pending herb to the catalog herb.
  await logout(page);
  await login(page, 'admin@test', 'pw123456');
  await page.goto('/staff/herbs');
  const pendingSelect = page.getByLabel('ชื่อสมุนไพรที่รอตรวจสอบ');
  await expect(pendingSelect.locator('option', { hasText: pendingHerbName })).toHaveCount(1);
  await pendingSelect.selectOption({ label: pendingHerbName });
  await page.getByLabel('จับคู่กับสมุนไพร').selectOption({ label: herbName });
  await page.getByRole('button', { name: 'จับคู่' }).click();
  // The pending-herb select is a global list across every district editor's
  // still-unreconciled herbs, so assert only this test's herb is gone from
  // it (another spec's pending herb may legitimately still be there).
  await expect(
    page.getByLabel('ชื่อสมุนไพรที่รอตรวจสอบ').locator('option', { hasText: pendingHerbName }),
  ).toHaveCount(0);

  // Editor: add a case under the recipe.
  await logout(page);
  await login(page, editorEmail, 'pw123456');
  await page.goto('/staff/cases');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('อาการ').fill('ปวดหัว');
  // The case result select is rendered by CaseForm's own native <select>
  // (cases/page.tsx), not CrudForm, so it is untouched by this migration.
  await page.getByLabel('ผลการรักษา').selectOption({ label: 'หายขาด' });
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: /ปวดหัว/ })).toBeVisible();
});
