import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e-writable",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  maxFailures: 1,
  timeout: 30_000,
  expect: { timeout: 5_000 },
  reporter: [["list"]],
  use: {
    baseURL: "http://127.0.0.1:18081",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "off",
  },
  projects: [
    {
      name: "chromium-writable",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: {
    command: "VIRTMGR_E2E_PORT=18081 VIRTMGR_E2E_READ_ONLY=false VIRTMGR_E2E_DATABASE_PATH=/tmp/virtmgr-e2e-writable.db bash ../scripts/run-e2e-server.sh",
    url: "http://127.0.0.1:18081/healthz",
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
