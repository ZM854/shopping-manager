import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 180_000,
  expect: { timeout: 15_000 },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: 'http://127.0.0.1:55173',
    browserName: 'chromium',
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
    channel:
      process.env.E2E_BROWSER_CHANNEL ??
      (process.platform === 'win32' ? 'msedge' : undefined),
    screenshot: 'only-on-failure',
    trace: 'off',
    video: 'off',
  },
});
