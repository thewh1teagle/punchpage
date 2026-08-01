import {defineConfig} from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 180000,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: [['list']],
  use: {
    browserName: 'chromium'
  }
});
