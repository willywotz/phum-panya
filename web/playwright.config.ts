import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/global-setup',
  timeout: 30_000,
  fullyParallel: true,
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
