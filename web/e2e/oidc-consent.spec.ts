import { createHash } from "node:crypto";

import { expect, test } from "@playwright/test";

test("OIDC login reaches the consent screen instead of bouncing back", async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);
  const login = await request.post("/api/auth/login", {
    data: { tenant: "acme", email: "owner@acme.test", password: "Password123!" },
  });
  expect(login.ok()).toBeTruthy();

  const redirectUri = `http://localhost:3000/consent-test-${Date.now()}`;
  const created = await request.post("/api/admin/clients", {
    data: {
      name: `Consent regression ${Date.now()}`,
      description: "Temporary client for the consent navigation regression",
      type: "public",
      redirectUris: [redirectUri],
      allowedScopes: ["openid", "profile", "email", "roles"],
      grantTypes: ["authorization_code"],
      requirePkce: true,
      enabled: true,
    },
  });
  expect(created.ok()).toBeTruthy();
  const createdBody = (await created.json()) as {
    client: { id: string; clientId: string };
  };
  const client = createdBody.client;
  // The request fixture has its own admin session; start the browser logged out
  // so authorize goes through the login page and then the consent screen.
  await page.context().clearCookies();

  try {
    const verifier = "consent-regression-verifier-abcdefghijklmnopqrstuvwxyz0123456789";
    const challenge = createHash("sha256").update(verifier).digest("base64url");
    const params = new URLSearchParams({
      client_id: client.clientId,
      redirect_uri: redirectUri,
      response_type: "code",
      scope: "openid profile email roles",
      state: "consent-regression-state",
      nonce: "consent-regression-nonce",
      code_challenge: challenge,
      code_challenge_method: "S256",
    });

    await page.goto(`/t/acme/oidc/authorize?${params.toString()}`);
    await page.locator("#email").fill("owner@acme.test");
    await page.locator("#password").fill("Password123!");
    await page.locator('button[type="submit"]').click();

    // Regression: a cached "pending" auth request used to send the user back
    // to /login as soon as ConsentPage mounted.
    await expect(page).toHaveURL(/\/consent\?request=/, { timeout: 60_000 });
    await expect(page.getByRole("button", { name: "Allow access" })).toBeVisible();
  } finally {
    await request.delete(`/api/admin/clients/${client.id}`);
  }
});
