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

// Full SRS §6.1 UAT walkthrough, run as one ordered narrative against the
// real built binary. Step 11 (backup zip contains the db and media, and a
// restore from it succeeds) is proven authoritatively by the Go integration
// test internal/backup/restore_test.go; this spec only exercises the
// POST /api/backup/run endpoint that step 11 also requires.
test('SRS §6.1 UAT: editor lifecycle, consent gate, public discovery, export scope, backup, i18n', async ({
  page,
  request,
}) => {
  test.setTimeout(180_000);

  const stamp = Date.now();
  const herbName = `UATHerb${stamp}`;
  const pendingHerbName = `UATPendingHerb${stamp}`;
  const districtName = `UATDistrict${stamp}`;
  const otherDistrictName = `UATOtherDistrict${stamp}`;
  const editorEmail = `uat-editor${stamp}@test`;
  const doctorCode = `UATDOC${stamp}`;
  const doctorName = `UATDoctor${stamp}`;
  const doctorPhone = `080${String(stamp).slice(-7)}`;
  const otherDoctorName = `UATOtherDoctor${stamp}`;
  const recipeCode = `UATREC${stamp}`;
  const recipeName = `UATRecipe${stamp}`;

  // --- Step 1: central admin creates a district editor and assigns a
  // district. Also seeds a catalog herb (needed for step 4) and a second
  // district + consented doctor (needed to prove step 10's export is
  // district-scoped, not just phone-scoped).
  await login(page, 'admin@test', 'pw123456');

  await page.goto('/staff/herbs');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('ชื่อไทย').fill(herbName);
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(herbName) })).toBeVisible();

  await page.goto('/staff/districts');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('ชื่อ').fill(districtName);
  await page.getByLabel('จังหวัด').fill('UAT Province');
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(districtName) })).toBeVisible();

  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('ชื่อ').fill(otherDistrictName);
  await page.getByLabel('จังหวัด').fill('UAT Other Province');
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(otherDistrictName) })).toBeVisible();

  await page.goto('/staff/users');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('ชื่อ-นามสกุล').fill('UAT Editor');
  await page.getByLabel('อีเมล').fill(editorEmail);
  await page.getByLabel('รหัสผ่าน').fill('pw123456');
  await selectByName(page, 'บทบาท', 'ผู้แก้ไขข้อมูลอำเภอ');
  await page.getByRole('combobox', { name: 'อำเภอ' }).click();
  await expect(page.getByRole('option', { name: districtName, exact: true })).toHaveCount(1);
  await page.getByRole('option', { name: districtName, exact: true }).click();
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(editorEmail) })).toBeVisible();

  // A consented doctor in the OTHER district, owned by admin: proves step
  // 10's export excludes doctors outside the editor's own district.
  await page.goto('/staff/doctors');
  await selectByName(page, 'อำเภอ', otherDistrictName);
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('รหัส').fill(`OTH${stamp}`);
  await page.getByLabel('ชื่อ-นามสกุล').fill(otherDoctorName);
  await selectByName(page, 'สถานะ', 'ใช้งาน');
  await page.getByLabel('ได้รับความยินยอม').check();
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: new RegExp(otherDoctorName) })).toBeVisible();

  // --- Step 2: the editor logs in and creates a Doctor with a photo.
  await logout(page);
  await login(page, editorEmail, 'pw123456');
  await page.goto('/staff/doctors');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('รหัส').fill(doctorCode);
  await page.getByLabel('ชื่อ-นามสกุล').fill(doctorName);
  await page.getByLabel('เบอร์โทรศัพท์').fill(doctorPhone);
  await selectByName(page, 'สถานะ', 'ใช้งาน');
  await page.getByRole('button', { name: 'บันทึก' }).click();
  const doctorRow = page.getByRole('row', { name: new RegExp(doctorName) });
  await expect(doctorRow).toBeVisible();

  await doctorRow.getByRole('button', { name: 'แก้ไข' }).click();
  await page.setInputFiles('#photo-upload', photoFixture);
  await expect(page.getByText(/\.jpg$/)).toBeVisible();
  await expect(page.getByLabel('ได้รับความยินยอม')).not.toBeChecked();
  await page.getByRole('button', { name: 'ยกเลิก' }).click();

  // --- Step 3: the Doctor is not public until consent is ticked.
  const beforeConsent = await request.get(
    `/api/public/doctors?q=${encodeURIComponent(doctorName)}`,
  );
  expect(await beforeConsent.json()).toEqual([]);

  await doctorRow.getByRole('button', { name: 'แก้ไข' }).click();
  await page.getByLabel('ได้รับความยินยอม').check();
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(doctorRow).toBeVisible();

  const afterConsent = await request.get(
    `/api/public/doctors?q=${encodeURIComponent(doctorName)}`,
  );
  const afterConsentDoctors: Array<{ id: number; full_name: string }> =
    await afterConsent.json();
  expect(afterConsentDoctors.map((d) => d.full_name)).toContain(doctorName);
  const doctorId = afterConsentDoctors.find((d) => d.full_name === doctorName)!.id;

  // --- Step 4: the editor adds a Recipe with one catalog herb and one
  // pending herb; it saves.
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

  // --- Step 5: the central admin reconciles the pending herb.
  await logout(page);
  await login(page, 'admin@test', 'pw123456');
  await page.goto('/staff/herbs');
  await selectByName(page, 'ชื่อสมุนไพรที่รอตรวจสอบ', pendingHerbName);
  await selectByName(page, 'จับคู่กับสมุนไพร', herbName);
  await page.getByRole('button', { name: 'จับคู่' }).click();
  // The pending-herb select is a global list across every district editor's
  // still-unreconciled herbs, so assert only this test's herb is gone from
  // it (another spec's pending herb may legitimately still be there, or the
  // reconcile panel may unmount entirely once no pending herbs remain).
  await page.waitForLoadState('networkidle');
  const pendingTrigger = page.getByRole('combobox', { name: 'ชื่อสมุนไพรที่รอตรวจสอบ' });
  if ((await pendingTrigger.count()) > 0) {
    await pendingTrigger.click();
    await expect(page.getByRole('option', { name: pendingHerbName, exact: true })).toHaveCount(0);
    await page.keyboard.press('Escape');
  }

  // --- Step 6: the editor adds an anonymous Case linked to the Recipe.
  await logout(page);
  await login(page, editorEmail, 'pw123456');
  await page.goto('/staff/cases');
  await page.getByRole('button', { name: 'เพิ่ม' }).click();
  await page.getByLabel('อาการ').fill('ปวดหัว');
  await selectByName(page, 'ผลการรักษา', 'หายขาด');
  await page.getByRole('button', { name: 'บันทึก' }).click();
  await expect(page.getByRole('row', { name: /ปวดหัว/ })).toBeVisible();
  await logout(page);

  // --- Step 7: the public finds the Doctor by keyword and by district
  // filter.
  await page.goto('/doctors');
  await page.getByLabel('ค้นหา').fill(doctorName);
  await expect(page.getByRole('link', { name: new RegExp(doctorName) })).toBeVisible();

  await page.getByLabel('ค้นหา').fill('');
  await selectByName(page, 'อำเภอ', districtName);
  await expect(page.getByRole('link', { name: new RegExp(doctorName) })).toBeVisible();

  // --- Step 8: the public filters recipes by one herb.
  await page.goto('/recipes');
  await selectByName(page, 'สมุนไพร', herbName);
  await expect(page.getByText(recipeName)).toBeVisible();

  // --- Step 9: the public prints/saves the healer page as PDF (public
  // fields only — no phone).
  await page.goto(`/doctor?id=${doctorId}`);
  await expect(page.getByRole('heading', { name: doctorName })).toBeVisible();
  await expect(page.getByText(doctorPhone)).toHaveCount(0);

  await page.emulateMedia({ media: 'print' });
  await expect(page.locator('nav.no-print')).toBeHidden();
  await expect(page.getByRole('button', { name: 'พิมพ์' })).toBeHidden();
  await expect(page.getByRole('heading', { name: doctorName })).toBeVisible();
  await expect(page.getByText(doctorPhone)).toHaveCount(0);
  await page.emulateMedia({ media: 'screen' });

  // --- Step 10: a district editor exports own-district data; the file has
  // no private fields, and no doctor from another district.
  await login(page, editorEmail, 'pw123456');
  await page.goto('/staff/doctors');
  const exportCsv = page.getByRole('link', { name: 'ส่งออก CSV' });
  await expect(exportCsv).toHaveAttribute('href', /\/api\/export\/doctors\.csv/);

  const csvRes = await page.request.get('/api/export/doctors.csv');
  expect(csvRes.ok()).toBeTruthy();
  const csvBody = await csvRes.text();
  const [csvHeader] = csvBody.split('\n');
  expect(csvHeader.toLowerCase()).not.toContain('phone');
  expect(csvBody).toContain(doctorName);
  expect(csvBody).not.toContain(otherDoctorName);
  expect(csvBody).not.toContain(doctorPhone);
  await logout(page);

  // --- Step 11: the nightly backup zip contains the database and images.
  // The zip's contents and restorability are proven authoritatively by
  // internal/backup/restore_test.go; here we only confirm the admin-only
  // endpoint that produces it works end to end.
  const adminLogin = await request.post('/api/login', {
    data: { email: 'admin@test', password: 'pw123456' },
  });
  expect(adminLogin.ok()).toBeTruthy();
  const backupRes = await request.post('/api/backup/run');
  expect(backupRes.ok()).toBeTruthy();
  const backupBody: { zip: string } = await backupRes.json();
  expect(backupBody.zip).toMatch(/backup-.*\.zip$/);
  await request.post('/api/logout');

  // --- Step 12: the UI switches Thai/English labels; record content stays
  // as typed (the doctor's own name is never translated).
  await page.goto(`/doctor?id=${doctorId}`);
  await expect(page.getByRole('heading', { name: doctorName })).toBeVisible();
  await page.getByRole('button', { name: 'EN' }).click();
  await expect(page.getByRole('button', { name: 'Print' })).toBeVisible();
  await expect(page.getByRole('heading', { name: doctorName })).toBeVisible();
});
