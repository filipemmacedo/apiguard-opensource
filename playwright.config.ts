import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  globalSetup: "./e2e/setup.ts",
  timeout: 30_000,
  retries: 1,
  workers: 1,
  use: {
    headless: true,
    screenshot: "only-on-failure",
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "admin",
      testMatch: /admin\..+\.ts$/,
      use: { baseURL: "http://localhost:3002" },
    },
    {
      name: "dashboard",
      testMatch: /dashboard\..+\.ts$/,
      use: { baseURL: "http://localhost:3000" },
    },
  ],
});
