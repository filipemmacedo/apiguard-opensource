import { test, expect } from "@playwright/test";

const DASH_EMAIL = "dash-e2e@test.com";
const DASH_PASSWORD = "DashPass123!";

/** Helper: login and wait for dashboard to load */
async function loginAndWait(page: import("@playwright/test").Page) {
  await page.goto("/login");
  // Wait for the form to be interactive (SSR hydration)
  await page.getByPlaceholder("cto@company.com").waitFor({ state: "visible", timeout: 10_000 });
  await page.getByPlaceholder("cto@company.com").fill(DASH_EMAIL);
  await page.getByPlaceholder("••••••••").fill(DASH_PASSWORD);
  await page.getByRole("button", { name: /sign in/i }).click();
  // Wait for sidebar to appear (indicates successful auth + layout loaded)
  await expect(page.getByRole("link", { name: "Logs" })).toBeVisible({ timeout: 15_000 });
}

test.describe("Dashboard App", () => {
  test("login page renders correctly", async ({ page }) => {
    await page.goto("/login");
    await expect(page.getByText("Welcome back")).toBeVisible();
    await expect(page.getByText("Sign in to your account")).toBeVisible();
    await expect(page.getByPlaceholder("cto@company.com")).toBeVisible();
    await expect(page.getByRole("button", { name: /sign in/i })).toBeVisible();
  });

  test("register page renders correctly", async ({ page }) => {
    await page.goto("/register");
    await expect(page.getByText("Create your account")).toBeVisible();
    await expect(page.getByPlaceholder("Jane Smith")).toBeVisible();
  });

  test("login with valid credentials navigates to home", async ({ page }) => {
    await loginAndWait(page);
    // Sidebar should be visible
    await expect(page.getByRole("link", { name: "Logs" })).toBeVisible();
  });

  test("login with wrong password shows error", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("cto@company.com").waitFor({ state: "visible", timeout: 10_000 });
    await page.getByPlaceholder("cto@company.com").fill(DASH_EMAIL);
    await page.getByPlaceholder("••••••••").fill("WrongPassword!");
    await page.getByRole("button", { name: /sign in/i }).click();

    await expect(page.getByText("Invalid credentials")).toBeVisible({ timeout: 5_000 });
  });

  test("sidebar navigation visible after login", async ({ page }) => {
    await loginAndWait(page);

    // All nav items should be visible (use role to avoid strict mode)
    await expect(page.getByRole("link", { name: "Logs" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Usage" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Playground" })).toBeVisible();
    await expect(page.getByRole("link", { name: "API Management" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Guardrails" })).toBeVisible();
  });

  test("navigate to usage page", async ({ page }) => {
    await loginAndWait(page);
    await page.getByRole("link", { name: "Usage" }).click();
    await expect(page).toHaveURL(/\/usage/);
  });

  test("navigate to playground page", async ({ page }) => {
    await loginAndWait(page);
    await page.getByRole("link", { name: "Playground" }).click();
    await expect(page).toHaveURL(/\/playground/);
  });

  test("navigate to API Management page", async ({ page }) => {
    await loginAndWait(page);
    await page.getByRole("link", { name: "API Management" }).click();
    await expect(page).toHaveURL(/\/api-management/);
  });

  test("navigate to Guardrails page", async ({ page }) => {
    await loginAndWait(page);
    await page.getByRole("link", { name: "Guardrails" }).click();
    await expect(page).toHaveURL(/\/guardrails/);
  });

  test("unauthenticated access redirects to login", async ({ page }) => {
    await page.goto("/usage");
    await expect(page).toHaveURL(/\/login/, { timeout: 5_000 });
  });

  test("forgot password page renders", async ({ page }) => {
    await page.goto("/forgot-password");
    await expect(page.getByText(/forgot|reset/i).first()).toBeVisible();
  });

  test("user profile shows in sidebar", async ({ page }) => {
    await loginAndWait(page);
    // Sidebar shows user info with role (owner for org creators)
    await expect(page.getByText("owner")).toBeVisible();
  });

  test("logout works", async ({ page }) => {
    await loginAndWait(page);
    await page.getByLabel("Sign out").click();
    await expect(page).toHaveURL(/\/login/, { timeout: 5_000 });
  });
});
