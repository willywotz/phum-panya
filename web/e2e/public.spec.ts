import { test, expect } from '@playwright/test';

import { selectByName } from './fixtures/select';

// SRS UAT 7-9, 12: an unauthenticated visitor finds a consented doctor by
// keyword and by district, opens the healer's ID card, filters recipes by
// herb, gets a clean print view, and can toggle the UI language without the
// doctor's own name ever being translated.
test('public visitor searches, filters by herb, and prints the healer page', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  const stamp = Date.now();
  const districtName = `PublicDistrict${stamp}`;
  const doctorName = `PublicDoctor${stamp}`;
  const herbName = `PublicHerb${stamp}`;
  const recipeName = `PublicRecipe${stamp}`;

  // Seed consented public data via the API as admin, using a request
  // context isolated from the visitor's browser page.
  const login = await request.post('/api/login', {
    data: { email: 'admin@test', password: 'pw123456' },
  });
  expect(login.ok()).toBeTruthy();

  // The staff CRUD API marshals model structs (no json tags) as PascalCase,
  // unlike the public API's explicit snake_case json tags (see lib/crud.ts).
  const districtRes = await request.post('/api/districts', {
    data: { name: districtName, province: 'Test' },
  });
  const { ID: districtId } = await districtRes.json();

  const herbRes = await request.post('/api/herbs', {
    data: { thai_name: herbName },
  });
  const { ID: herbId } = await herbRes.json();

  const doctorRes = await request.post('/api/doctors', {
    data: {
      code: `PD${stamp}`,
      full_name: doctorName,
      district_id: districtId,
      status: 'active',
      consent_obtained: true,
    },
  });
  const { ID: doctorId } = await doctorRes.json();

  // A 1x1 PNG, small enough to inline, so /media/<path> serves a real file.
  const onePixelPng = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
    'base64',
  );
  const photoRes = await request.post(`/api/doctors/${doctorId}/photo`, {
    multipart: { photo: { name: 'photo.png', mimeType: 'image/png', buffer: onePixelPng } },
  });
  expect(photoRes.ok()).toBeTruthy();

  const recipeRes = await request.post('/api/recipes', {
    data: {
      code: `PR${stamp}`,
      name: recipeName,
      doctor_id: doctorId,
      data_year: 2024,
      ingredients: [{ herb_id: herbId, amount: '1', unit: 'g' }],
    },
  });
  const { ID: recipeId } = await recipeRes.json();

  await request.post('/api/cases', {
    data: { recipe_id: recipeId, condition: 'ปวดหัว', result: 'cured', data_year: 2024 },
  });

  await request.post('/api/logout');

  // 1. /doctors: search by keyword, then filter by district.
  await page.goto('/doctors');
  await page.getByLabel('ค้นหา').fill(doctorName);
  await expect(page.getByRole('link', { name: new RegExp(doctorName) })).toBeVisible();

  // The district filter selects by the district's NAME, not its raw numeric
  // id.
  await page.getByLabel('ค้นหา').fill('');
  await selectByName(page, 'อำเภอ', districtName);
  await expect(page.getByRole('link', { name: new RegExp(doctorName) })).toBeVisible();

  // 2. Open the doctor's ID card: name, its recipe, the recipe's ingredient
  // (herb) name, and the uploaded ID-card photo are all shown.
  await page.getByRole('link', { name: new RegExp(doctorName) }).click();
  await expect(page).toHaveURL(new RegExp(`/doctor\\?id=${doctorId}$`));
  await expect(page.getByRole('heading', { name: doctorName })).toBeVisible();
  await expect(page.getByText(recipeName)).toBeVisible();
  await expect(page.getByText(herbName)).toBeVisible();
  await expect(page.locator('article > img').first()).toHaveAttribute('src', /^\/media\//);

  // 4. Print stylesheet hides the nav while the healer's name stays visible.
  await page.emulateMedia({ media: 'print' });
  await expect(page.locator('nav.no-print')).toBeHidden();
  await expect(page.getByRole('heading', { name: doctorName })).toBeVisible();
  await page.emulateMedia({ media: 'screen' });

  // 5. Toggling language changes a label, never the doctor's own name.
  await page.getByRole('button', { name: 'EN' }).click();
  await expect(page.getByRole('button', { name: 'Print' })).toBeVisible();
  await expect(page.getByRole('heading', { name: doctorName })).toBeVisible();

  // 3. /recipes: filter by herb finds the recipe.
  await page.goto('/recipes');
  await selectByName(page, 'Herb', herbName);
  await expect(page.getByText(recipeName)).toBeVisible();
});
