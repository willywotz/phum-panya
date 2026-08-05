import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/global-setup',
  timeout: 30_000,
  // The app is one SQLite binary with a single shared dev DB. Parallel browser
  // sessions contend on writes (SQLITE_BUSY), which makes data-heavy specs
  // flaky. Run serially so the suite is deterministic.
  fullyParallel: false,
  workers: 1,
  // On a loaded CI runner a create -> list-refetch round-trip can occasionally
  // exceed the default assertion timeout. Retry on CI only; a real regression
  // still fails every attempt. Local runs keep 0 retries (fail fast).
  retries: process.env.CI ? 2 : 0,
  use: {
    baseURL: 'http://localhost:8080',
  },
  webServer: {
    command:
      "bash -c 'cd .. && make build && APP_DEV=1 APP_ADMIN_EMAIL=admin@test APP_ADMIN_PASSWORD=pw123456 APP_DB_PATH=$(mktemp -u) APP_MEDIA_DIR=$(mktemp -d) ./server'",
    url: 'http://localhost:8080',
    reuseExistingServer: !process.env.CI,
    timeout: 180_000,
  },
});
