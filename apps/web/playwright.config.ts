import { defineConfig, devices } from "@playwright/test";

const databaseUrl =
  process.env.E2E_DATABASE_URL ??
  "postgres://wiki@127.0.0.1:55432/wiki?sslmode=disable";
const testActorId = "00000000-0000-7000-8000-000000000201";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  timeout: 90_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: "http://127.0.0.1:3000",
    extraHTTPHeaders: { "X-Actor-ID": testActorId },
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        channel: process.env.PLAYWRIGHT_CHANNEL || undefined,
      },
    },
  ],
  webServer: [
    {
      command: "go run ./cmd/api",
      cwd: "../../backend",
      url: "http://127.0.0.1:8080/healthz",
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        DATABASE_URL: databaseUrl,
        REDIS_URL: "redis://127.0.0.1:6379/0",
        S3_ENDPOINT: "http://127.0.0.1:9000",
        S3_BUCKET: "wiki-e2e",
        S3_ACCESS_KEY: "e2e-placeholder",
        S3_SECRET_KEY: "e2e-placeholder",
        AUTH_DEV_HEADER_ENABLED: "true",
        PORT: "8080",
      },
    },
    {
      command: "npm run dev -- --hostname 127.0.0.1 --port 3000",
      url: "http://127.0.0.1:3000",
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        API_BASE_URL: "http://127.0.0.1:8080",
      },
    },
  ],
});
