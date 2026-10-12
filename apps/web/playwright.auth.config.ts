import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "./tests/auth-e2e",
  outputDir: "../../.data/auth-e2e",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: !!process.env.CI,
  reporter: [["list"]],
  timeout: 60_000,
  expect: { timeout: 10_000 },
  use: { baseURL: "http://127.0.0.1:3100", trace: "off", screenshot: "only-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: "npm run start",
    url: "http://127.0.0.1:3100",
    env: { PORT: "3100", HOSTNAME: "127.0.0.1", NEXT_TELEMETRY_DISABLED: "1" },
    reuseExistingServer: false,
    timeout: 60_000,
  },
});
