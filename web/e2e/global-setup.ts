import { request, type FullConfig } from '@playwright/test';

// The import-template fixture hard-codes district_id=1. A fresh e2e DB has no
// districts, so seed exactly one before the parallel workers run — the first
// insert on a fresh DB becomes id 1. Idempotent: does nothing if a district
// already exists (a locally reused server).
export default async function globalSetup(config: FullConfig) {
  const baseURL = config.projects[0]?.use.baseURL ?? 'http://localhost:8080';
  const context = await request.newContext({ baseURL });
  try {
    const login = await context.post('/api/login', {
      data: { email: 'admin@test', password: 'pw123456' },
    });
    if (!login.ok()) {
      return;
    }
    const districts = await (await context.get('/api/districts')).json();
    if (Array.isArray(districts) && districts.length === 0) {
      await context.post('/api/districts', {
        data: { name: 'นำเข้าอำเภอ', province: 'E2E' },
      });
    }
  } finally {
    await context.dispose();
  }
}
