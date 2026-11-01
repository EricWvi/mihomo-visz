import { defineConfig, devices } from '@playwright/test'
export default defineConfig({
  testDir: './e2e', outputDir: '../.tmp/playwright-results',
  use: { baseURL: 'http://127.0.0.1:4175', ...devices['Desktop Chrome'], launchOptions: { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || undefined } },
  webServer: { command: 'npm run dev -- --port 4175', url: 'http://127.0.0.1:4175', reuseExistingServer: false },
  reporter: 'list',
})
