import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: process.env.CI ? 2 : 2,
  reporter: [["list"]],
  timeout: 30_000,
  expect: { timeout: 8000 },
  use: {
    baseURL: "http://127.0.0.1:3100",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: "npm run start",
    url: "http://127.0.0.1:3100",
    env: { PORT: "3100", HOSTNAME: "127.0.0.1", NEXT_TELEMETRY_DISABLED: "1" },
    reuseExistingServer: false,
    timeout: 60_000,
  },
});
