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

test("tenants can use different built-in auth templates", async ({ page }) => {
  await page.goto("/login?tenant=acme");
  await expect(page.locator('[data-template="aurora"]')).toBeVisible();
  await page.goto("/login?tenant=globex");
  await expect(page.locator('[data-template="fintech"]')).toBeVisible();
});

test("admin can browse the login template gallery", async ({ page }) => {
  await login(page);
  await page.goto("/admin/settings");
  await page.getByRole("tab", { name: /Branding/i }).click();
  await expect(page.getByText("Login page template")).toBeVisible();
  await expect(page.getByText("Fintech", { exact: true })).toBeVisible();
  await expect(page.getByText("Government", { exact: true })).toBeVisible();
});

test("mock upstream provider provisions and links a user", async ({ page, browser }) => {
  await login(page);
  const origin = new URL(page.url()).origin;
  const configure = await page.request.put("/api/admin/identity-providers/mock", {
    data: {
      name: "Mock Provider",
      enabled: true,
      config: { subject: "mock-e2e", email: "mock-idp@acme.test", name: "Mock IdP User" },
      autoProvision: true,
      defaultRoles: ["member"],
      trustEmail: true,
    },
  });
  expect(configure.ok()).toBeTruthy();

  const publicList = await page.request.get("/api/public/tenants/acme/identity-providers");
  expect((await publicList.json()).items.some((item: { type: string }) => item.type === "mock")).toBeTruthy();

  const mockContext = await browser.newContext({ baseURL: origin });
  const mockPage = await mockContext.newPage();
  try {
    await mockPage.goto("/t/acme/idp/mock/start?return_to=/account");
    await mockPage.waitForURL(/\/account/);
    await expect(mockPage.locator("#account-name")).toHaveValue("Mock IdP User");
    await expect(mockPage.locator("body")).toContainText("Mock Provider");
  } finally {
    await mockContext.close();
  }

  const users = await page.request.get("/api/admin/users?q=mock-idp@acme.test");
  const user = (await users.json()).items[0];
  if (user) await page.request.delete(`/api/admin/users/${user.id}`);
  await page.request.delete("/api/admin/identity-providers/mock");
});

test("SAML mock IdP signs in and provisions a user", async ({ page, browser }) => {
  await login(page);
  const origin = new URL(page.url()).origin;
  const idpMetadata = await (await page.request.get("http://127.0.0.1:9091/metadata")).text();
  const configure = await page.request.put("/api/admin/identity-providers/saml", {
    data: {
      name: "Mock SAML IdP",
      enabled: true,
      config: { idpMetadataXML: idpMetadata, emailAttribute: "email", nameAttribute: "name" },
      autoProvision: true,
      defaultRoles: ["member"],
      trustEmail: false,
    },
  });
  expect(configure.ok()).toBeTruthy();

  const spMetadata = await (await page.request.get("/t/acme/saml/metadata")).text();
  const register = await page.request.post("http://127.0.0.1:9091/services/acme", {
    headers: { "Content-Type": "application/xml" },
    data: spMetadata,
  });
  expect(register.ok()).toBeTruthy();

  const samlContext = await browser.newContext({ baseURL: origin });
  const samlPage = await samlContext.newPage();
  try {
    await samlPage.goto("/t/acme/saml/login?return_to=/account");
    await samlPage.locator('input[name="user"]').fill("samluser");
    await samlPage.locator('input[name="password"]').fill("Password123!");
    await samlPage.locator('input[type="submit"]').click();
    await samlPage.waitForURL(/\/account/, { timeout: 30_000 });
    await expect(samlPage.locator("body")).toContainText("saml@acme.test");
  } finally {
    await samlContext.close();
  }

  const users = await page.request.get("/api/admin/users?q=saml@acme.test");
  const user = (await users.json()).items[0];
  if (user) await page.request.delete(`/api/admin/users/${user.id}`);
  await page.request.delete("/api/admin/identity-providers/saml");
});

test("hand-drawn auth template renders and can be restored", async ({ page }) => {
  await login(page);
  const settingsResponse = await page.request.get("/api/admin/settings");
  const settings = await settingsResponse.json();
  const original = settings.tenant.branding;
  const handDrawn = { ...original, loginTemplate: "handdrawn" };
  try {
    const patch = await page.request.patch("/api/admin/settings", { data: { branding: handDrawn } });
    expect(patch.ok()).toBeTruthy();
    await page.goto("/login?tenant=acme");
    await expect(page.locator('[data-template="handdrawn"]')).toBeVisible();
    await expect(page.locator(".auth-hand-card")).toBeVisible();
    await expect(page.locator(".font-hand").first()).toBeVisible();
  } finally {
    await page.request.patch("/api/admin/settings", { data: { branding: original } });
  }
});

test("custom domain resolves the tenant and applies white-label branding", async ({ page, browser }) => {
  await login(page);
  const settings = await (await page.request.get("/api/admin/settings")).json();
  const original = settings.tenant.branding;
  const originalDomains = settings.tenant.domains ?? [];
  const domain = `login-${Date.now()}.acme.test`;
  const branded = { ...original, brandName: "Acme Identity Cloud", hidePlatformBranding: true };
  try {
    const patch = await page.request.patch("/api/admin/settings", { data: { domains: [...originalDomains, domain], branding: branded } });
    expect(patch.ok()).toBeTruthy();
    const ctx = await browser.newContext();
    const domainPage = await ctx.newPage();
    await domainPage.goto(`/login?domain=${domain}`);
    await expect(domainPage.locator("[data-template]")).toBeVisible();
    await expect(domainPage.getByText("Acme Identity Cloud").first()).toBeVisible();
    await expect(domainPage.getByText("Acme Corporation").first()).toBeVisible();
    await ctx.close();
  } finally {
    await page.request.patch("/api/admin/settings", { data: { domains: originalDomains, branding: original } });
  }
});

test("passkey enrollment, password + passkey login, and removal", async ({ page, context }) => {
  await login(page);
  const client = await context.newCDPSession(page);
  await client.send("WebAuthn.enable");
  const { authenticatorId } = await client.send("WebAuthn.addVirtualAuthenticator", {
    options: { protocol: "ctap2", transport: "internal", hasResidentKey: true, hasUserVerification: true, isUserVerified: true },
  });

  await page.goto("/account");
  await page.getByRole("button", { name: "Add passkey" }).click();
  await expect(page.getByRole("button", { name: "Remove" }).first()).toBeVisible({ timeout: 30_000 });

  await page.request.post("/api/auth/logout");
  await page.goto("/login");
  await page.fill("#email", "owner@acme.test");
  await page.fill("#password", "Password123!");
  await page.locator('button[type="submit"]').click();
  await expect(page.getByRole("button", { name: "Use a passkey" })).toBeVisible({ timeout: 30_000 });
  await page.getByRole("button", { name: "Use a passkey" }).click();
  await page.waitForURL(/\/admin$/, { timeout: 60_000 });

  await page.goto("/account");
  await page.getByRole("button", { name: "Remove" }).first().click();
  await expect(page.getByText("No passkeys registered yet.")).toBeVisible({ timeout: 30_000 });
  await client.send("WebAuthn.removeVirtualAuthenticator", { authenticatorId });
});

test("Arabic switches the shell to RTL", async ({ page }) => {
  await page.goto("/login");
  await page.getByTestId("language-menu").click();
  await page.getByText("العربية", { exact: true }).first().click();
  await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
  await expect(page.locator("html")).toHaveAttribute("lang", "ar-SA");
  await expect(page.locator("h2")).toContainText("مرحبًا بعودتك");
});

test("admin can open billing, plans and usage", async ({ page }) => {
  await login(page);
  await page.goto("/admin/billing");
  await expect(page.locator("h1").first()).toContainText("Billing & usage");
  await expect(page.getByText("Current plan").first()).toBeVisible();
  await expect(page.getByText("Free", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("API requests this month")).toBeVisible();
  await expect(page.getByText("Pro", { exact: true }).first()).toBeVisible();
});

test("admin can open MCP access and mail outbox", async ({ page }) => {
  await login(page);
  await page.goto("/admin/mcp");
  await expect(page.locator("h1").first()).toContainText("MCP access");
  await expect(page.locator("text=/\\/mcp$/").first()).toBeVisible();

  await page.goto("/admin/mail");
  await expect(page.locator("h1").first()).toContainText("Mail outbox");
});
