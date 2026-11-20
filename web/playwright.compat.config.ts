import { defineConfig, devices } from "@playwright/test";

const projects = [
  { name: "chromium-ipv6", use: { ...devices["Desktop Chrome"] } },
  { name: "firefox-ipv6", use: { ...devices["Desktop Firefox"] } },
];

if (process.env.PLAYWRIGHT_WEBKIT === "1") {
  projects.push({ name: "webkit-ipv6", use: { ...devices["Desktop Safari"] } });
}

const webServer =
  process.env.PLAYWRIGHT_SKIP_WEBSERVER === "1"
    ? {}
    : {
        webServer: {
          command:
            "VIRTMGR_E2E_PORT=18082 VIRTMGR_E2E_LISTEN='[::1]:18082' VIRTMGR_E2E_DATABASE_PATH=/tmp/virtmgr-e2e-compat.db bash ../scripts/run-e2e-server.sh",
          url: "http://[::1]:18082/healthz",
          reuseExistingServer: false,
          timeout: 120_000,
        },
      };

export default defineConfig({
  testDir: "./e2e-compat",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  maxFailures: 1,
  timeout: 30_000,
  expect: { timeout: 5_000 },
  reporter: [["list"]],
  use: {
    baseURL: "http://[::1]:18082",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "off",
  },
  projects,
  ...webServer,
});
