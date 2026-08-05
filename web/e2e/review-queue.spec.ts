import { test, expect, type Page } from '@playwright/test';

async function login(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.getByLabel('อีเมล').fill(email);
  await page.getByLabel('รหัสผ่าน').fill(password);
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await expect(page).toHaveURL(/\/staff$/);
}

test('admin approves a pending doctor from the queue', async ({ page, request }) => {
  test.setTimeout(120_000);
  const stamp = Date.now();
  const districtName = `รีวิวอำเภอ${stamp}`;
  const editorEmail = `rev-editor${stamp}@test`;
  const doctorName = `หมอรอรีวิว${stamp}`;
  const doctorName2 = `หมอรอรีวิวสอง${stamp}`;

  // Admin seeds a district + district_editor.
  await login(page, 'admin@test', 'pw123456');
  await page.goto('/staff/districts');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('ชื่อ').fill(districtName);
  await page.getByLabel('จังหวัด').fill('E2E');
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(districtName) })).toBeVisible();

  await page.goto('/staff/users');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('ชื่อ-นามสกุล').fill('Rev Editor');
  await page.getByLabel('อีเมล').fill(editorEmail);
  await page.getByLabel('รหัสผ่าน').fill('pw123456');
  await page.getByRole('combobox', { name: 'บทบาท' }).click();
  await page.getByRole('option', { name: 'ผู้แก้ไขข้อมูลอำเภอ' }).click();
  await page.getByRole('combobox', { name: 'อำเภอ' }).click();
  await page.getByRole('option', { name: districtName, exact: true }).click();
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(editorEmail) })).toBeVisible();

  // Editor creates a doctor -> lands in the pending queue.
  await page.goto('/staff');
  await page.getByRole('button', { name: 'ออกจากระบบ' }).click();
  await login(page, editorEmail, 'pw123456');
  await page.goto('/staff/doctors');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('รหัส').fill(`RVDOC${stamp}`);
  await page.getByLabel('ชื่อ-นามสกุล').fill(doctorName);
  // 'status' is a required select; mirror staff-flow.spec.ts.
  await page.getByRole('combobox', { name: 'สถานะ' }).click();
  await page.getByRole('option', { name: 'ใช้งาน', exact: true }).click();
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(doctorName) })).toBeVisible();

  // A second pending doctor, to check the reject dialog does not leak state across rows.
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('รหัส').fill(`RVDOC2${stamp}`);
  await page.getByLabel('ชื่อ-นามสกุล').fill(doctorName2);
  await page.getByRole('combobox', { name: 'สถานะ' }).click();
  await page.getByRole('option', { name: 'ใช้งาน', exact: true }).click();
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(doctorName2) })).toBeVisible();

  await page.goto('/staff');
  await page.getByRole('button', { name: 'ออกจากระบบ' }).click();

  // Admin approves it from the queue.
  await login(page, 'admin@test', 'pw123456');
  await page.goto('/staff/review');
  const row = page.getByRole('row', { name: new RegExp(doctorName) });
  const row2 = page.getByRole('row', { name: new RegExp(doctorName2) });
  await expect(row).toBeVisible();
  await expect(row2).toBeVisible();

  // Type a reject reason in row 1, cancel without submitting, then open row 2's
  // dialog: the reason must not have leaked across rows.
  await row.getByRole('button', { name: 'ไม่อนุมัติ' }).click();
  await page.getByRole('textbox', { name: 'เหตุผลที่ไม่อนุมัติ' }).fill('เหตุผลของแถวที่ 1');
  await page.getByRole('button', { name: 'ยกเลิก' }).click();
  await row2.getByRole('button', { name: 'ไม่อนุมัติ' }).click();
  await expect(page.getByRole('textbox', { name: 'เหตุผลที่ไม่อนุมัติ' })).toHaveValue('');
  await page.getByRole('button', { name: 'ยกเลิก' }).click();

  await row.getByRole('button', { name: 'อนุมัติ', exact: true }).click();
  await expect(page.getByRole('row', { name: new RegExp(doctorName) })).toHaveCount(0);
});
