import { test, expect } from '@playwright/test';

// #28: a recipe can hold MANY photos. The public healer page must render every
// photo the backend returns in the recipe `photos` array (it previously read a
// single `photo` field, which the projection no longer sends). Seed a recipe,
// append two photos, and assert both render on the public doctor page.
test('public healer page renders all recipe photos', async ({ page, request }) => {
  test.setTimeout(60_000);

  const stamp = Date.now();
  const districtName = `PhotoDistrict${stamp}`;
  const doctorName = `PhotoDoctor${stamp}`;
  const herbName = `PhotoHerb${stamp}`;
  const recipeName = `PhotoRecipe${stamp}`;

  const login = await request.post('/api/login', {
    data: { email: 'admin@test', password: 'pw123456' },
  });
  expect(login.ok()).toBeTruthy();

  const districtRes = await request.post('/api/districts', {
    data: { name: districtName, province: 'Test' },
  });
  const { ID: districtId } = await districtRes.json();

  const herbRes = await request.post('/api/herbs', { data: { thai_name: herbName } });
  const { ID: herbId } = await herbRes.json();

  const doctorRes = await request.post('/api/doctors', {
    data: {
      code: `PHD${stamp}`,
      full_name: doctorName,
      district_id: districtId,
      status: 'active',
      consent_obtained: true,
    },
  });
  const { ID: doctorId } = await doctorRes.json();

  const recipeRes = await request.post('/api/recipes', {
    data: {
      code: `PHR${stamp}`,
      name: recipeName,
      doctor_id: doctorId,
      data_year: 2024,
      ingredients: [{ herb_id: herbId, amount: '1', unit: 'g' }],
    },
  });
  const { ID: recipeId } = await recipeRes.json();

  // A 1x1 PNG, small enough to inline, so /media/<path> serves a real file.
  const onePixelPng = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
    'base64',
  );
  for (const name of ['first.png', 'second.png']) {
    const res = await request.post(`/api/recipes/${recipeId}/photo`, {
      multipart: { photo: { name, mimeType: 'image/png', buffer: onePixelPng } },
    });
    expect(res.ok()).toBeTruthy();
  }

  await request.post('/api/logout');

  await page.goto(`/doctor?id=${doctorId}`);
  await expect(page.getByRole('heading', { name: doctorName })).toBeVisible();

  const recipeImages = page.getByRole('img', { name: recipeName });
  await expect(recipeImages).toHaveCount(2);
  await expect(recipeImages.nth(0)).toHaveAttribute('src', /^\/media\//);
  await expect(recipeImages.nth(1)).toHaveAttribute('src', /^\/media\//);
});
