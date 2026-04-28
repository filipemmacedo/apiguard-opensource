import { test, expect } from "@playwright/test";

const ADMIN_EMAIL = "admin-e2e@test.com";
const ADMIN_PASSWORD = "AdminPass123!";

test.describe("Admin App", () => {
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
    await expect(page.getByPlaceholder("you@company.com")).toBeVisible();
  });

  test("login with valid credentials navigates to overview", async ({ page }) => {
    await page.goto("/login");

    await page.getByPlaceholder("cto@company.com").fill(ADMIN_EMAIL);
    await page.getByPlaceholder("••••••••").fill(ADMIN_PASSWORD);
    await page.getByRole("button", { name: /sign in/i }).click();

    // Should navigate to /overview
    await expect(page).toHaveURL(/\/overview/, { timeout: 10_000 });

    // Overview heading should show
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
    await expect(page.getByText("Platform administration")).toBeVisible();
  });

  test("login with wrong password shows error", async ({ page }) => {
    await page.goto("/login");

    await page.getByPlaceholder("cto@company.com").fill(ADMIN_EMAIL);
    await page.getByPlaceholder("••••••••").fill("WrongPassword123!");
    await page.getByRole("button", { name: /sign in/i }).click();

    // Should show an error
    await expect(page.getByText(/invalid|error|credentials/i)).toBeVisible({ timeout: 5_000 });
  });

  test("sidebar navigation works after login", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("cto@company.com").fill(ADMIN_EMAIL);
    await page.getByPlaceholder("••••••••").fill(ADMIN_PASSWORD);
    await page.getByRole("button", { name: /sign in/i }).click();
    await expect(page).toHaveURL(/\/overview/, { timeout: 10_000 });

    // Sidebar should be visible with nav items
    await expect(page.getByRole("navigation", { name: "Main navigation" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Overview" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Tenants" })).toBeVisible();

    // Navigate to Tenants
    await page.getByRole("link", { name: "Tenants" }).click();
    await expect(page).toHaveURL(/\/tenants/);
  });

  test("overview page shows stats cards", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("cto@company.com").fill(ADMIN_EMAIL);
    await page.getByPlaceholder("••••••••").fill(ADMIN_PASSWORD);
    await page.getByRole("button", { name: /sign in/i }).click();
    await expect(page).toHaveURL(/\/overview/, { timeout: 10_000 });

    // Stats cards should be visible (even if values are 0)
    await expect(page.getByText("Total Tenants")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("Active Tenants")).toBeVisible();
    await expect(page.getByText("API Calls This Month")).toBeVisible();
    await expect(page.getByText("Monthly Revenue")).toBeVisible();
  });

  test("tenants page renders with table", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("cto@company.com").fill(ADMIN_EMAIL);
    await page.getByPlaceholder("••••••••").fill(ADMIN_PASSWORD);
    await page.getByRole("button", { name: /sign in/i }).click();
    await expect(page).toHaveURL(/\/overview/, { timeout: 10_000 });

    // Navigate to tenants via sidebar link
    await page.getByRole("link", { name: "Tenants" }).click();
    await expect(page).toHaveURL(/\/tenants/);

    // Should have Add Tenant button
    await expect(page.getByRole("button", { name: /add tenant/i })).toBeVisible({ timeout: 10_000 });
  });

  test("add tenant modal opens", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("cto@company.com").fill(ADMIN_EMAIL);
    await page.getByPlaceholder("••••••••").fill(ADMIN_PASSWORD);
    await page.getByRole("button", { name: /sign in/i }).click();
    await expect(page).toHaveURL(/\/overview/, { timeout: 10_000 });

    // Go to tenants page via sidebar
    await page.getByRole("link", { name: "Tenants" }).click();
    await expect(page).toHaveURL(/\/tenants/);

    // Click Add Tenant
    await page.getByRole("button", { name: /add tenant/i }).first().click();

    // Modal should appear
    await expect(page.getByText("Add New Tenant")).toBeVisible({ timeout: 5_000 });
  });

  test("unauthenticated access redirects to login", async ({ page }) => {
    await page.goto("/overview");
    // Should redirect to login
    await expect(page).toHaveURL(/\/login/, { timeout: 5_000 });
  });

  test("forgot password page renders", async ({ page }) => {
    await page.goto("/forgot-password");
    await expect(page.getByText(/forgot|reset/i).first()).toBeVisible();
  });

  test("user profile shows in sidebar after login", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("cto@company.com").fill(ADMIN_EMAIL);
    await page.getByPlaceholder("••••••••").fill(ADMIN_PASSWORD);
    await page.getByRole("button", { name: /sign in/i }).click();
    await expect(page).toHaveURL(/\/overview/, { timeout: 10_000 });

    // User profile area should show "Admin" (from JWT name or fallback) and "Super Admin" role
    await expect(page.getByText("Super Admin")).toBeVisible();
  });

  test("logout works", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("cto@company.com").fill(ADMIN_EMAIL);
    await page.getByPlaceholder("••••••••").fill(ADMIN_PASSWORD);
    await page.getByRole("button", { name: /sign in/i }).click();
    await expect(page).toHaveURL(/\/overview/, { timeout: 10_000 });

    // Click logout button
    await page.getByTitle("Sign out").click();

    // Should redirect to login
    await expect(page).toHaveURL(/\/login/, { timeout: 5_000 });
  });
});
