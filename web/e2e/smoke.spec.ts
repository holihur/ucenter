import { test, expect } from "@playwright/test";

async function login(page: import("@playwright/test").Page, email = "owner@acme.test", password = "Password123!") {
  await page.goto("/login");
  await page.fill("#email", email);
  await page.fill("#password", password);
  await page.locator('button[type="submit"]').click();
  await page.waitForURL(/\/admin$/, { timeout: 60_000 });
  await page.waitForLoadState("networkidle");
}

test("login, dark theme and Chinese UI persist", async ({ page }) => {
  await page.goto("/login");
  await expect(page.locator("#email")).toBeVisible();

  await page.getByTestId("language-menu").click();
  await page.getByText("简体中文", { exact: true }).first().click();
  await expect(page.locator("html")).toHaveAttribute("lang", "zh-CN");
  await expect(page.locator("h2")).toContainText("欢迎回来");

  await page.getByTestId("theme-menu").click();
  await page.getByText("深色", { exact: true }).click();
  await expect(page.locator("html")).toHaveClass(/dark/);

  await page.reload();
  await expect(page.locator("html")).toHaveClass(/dark/);
  await expect(page.locator("html")).toHaveAttribute("lang", "zh-CN");

  await page.fill("#email", "owner@acme.test");
  await page.fill("#password", "Password123!");
  await page.locator('button[type="submit"]').click();
  await page.waitForURL(/\/admin$/, { timeout: 60_000 });
  await expect(page.getByRole("link", { name: "概览" })).toBeVisible();
  await expect(page.locator("h1").first()).toContainText("欢迎回来");
});

test("Arabic switches the shell to RTL", async ({ page }) => {
  await page.goto("/login");
  await page.getByTestId("language-menu").click();
  await page.getByText("العربية", { exact: true }).first().click();
  await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
  await expect(page.locator("html")).toHaveAttribute("lang", "ar-SA");
  await expect(page.locator("h2")).toContainText("مرحبًا بعودتك");
});

test("admin can open MCP access and mail outbox", async ({ page }) => {
  await login(page);
  await page.goto("/admin/mcp");
  await expect(page.locator("h1").first()).toContainText("MCP access");
  await expect(page.locator("text=/\\/mcp$/").first()).toBeVisible();

  await page.goto("/admin/mail");
  await expect(page.locator("h1").first()).toContainText("Mail outbox");
});
