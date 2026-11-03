const ISSUER = import.meta.env.VITE_ISSUER || "http://localhost:8080/t/acme";
const CLIENT_ID = import.meta.env.VITE_CLIENT_ID || "app_demo_cli";
const REDIRECT_URI = import.meta.env.VITE_REDIRECT_URI || "http://localhost:5173/callback";
const SCOPE = "openid profile email roles";

const output = document.querySelector<HTMLPreElement>("#output")!;
const loginButton = document.querySelector<HTMLButtonElement>("#login")!;

function base64url(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

async function sha256(value: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value));
  return base64url(new Uint8Array(digest));
}

async function startLogin() {
  const verifier = base64url(crypto.getRandomValues(new Uint8Array(48)));
  const state = base64url(crypto.getRandomValues(new Uint8Array(16)));
  sessionStorage.setItem("pkce_verifier", verifier);
  sessionStorage.setItem("oidc_state", state);
  const params = new URLSearchParams({
    client_id: CLIENT_ID,
    redirect_uri: REDIRECT_URI,
    response_type: "code",
    scope: SCOPE,
    state,
    nonce: base64url(crypto.getRandomValues(new Uint8Array(16))),
    code_challenge: await sha256(verifier),
    code_challenge_method: "S256",
  });
  window.location.assign(`${ISSUER}/oidc/authorize?${params}`);
}

async function handleCallback() {
  const params = new URLSearchParams(window.location.search);
  const code = params.get("code");
  const state = params.get("state");
  const error = params.get("error");
  if (error) throw new Error(`${error}: ${params.get("error_description") || ""}`);
  if (!code) return false;
  if (state !== sessionStorage.getItem("oidc_state")) throw new Error("state mismatch");
  const verifier = sessionStorage.getItem("pkce_verifier") || "";
  const tokenResponse = await fetch(`${ISSUER}/oidc/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "authorization_code",
      client_id: CLIENT_ID,
      code,
      redirect_uri: REDIRECT_URI,
      code_verifier: verifier,
    }),
  });
  const tokens = await tokenResponse.json();
  if (!tokenResponse.ok) throw new Error(JSON.stringify(tokens));
  const userinfo = await fetch(`${ISSUER}/oidc/userinfo`, {
    headers: { Authorization: `Bearer ${tokens.access_token}` },
  }).then((r) => r.json());
  output.textContent = JSON.stringify({ tokens: { ...tokens, access_token: "…", id_token: "…" }, userinfo }, null, 2);
  history.replaceState({}, "", "/");
  return true;
}

loginButton.addEventListener("click", () => void startLogin());
handleCallback().catch((err) => {
  output.textContent = String(err);
});
