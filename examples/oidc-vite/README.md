# Vite OIDC example

A dependency-light Vite + TypeScript demo using the authorization code flow with PKCE.

```bash
cd examples/oidc-vite
pnpm install
pnpm dev
```

Open `http://localhost:5173`, click **Sign in with UCenter**, and sign in with
`owner@acme.test / Password123!`. The demo uses the seeded public client
`app_demo_cli` and redirect `http://localhost:5173/callback`.

Environment overrides:

```bash
VITE_ISSUER=http://localhost:8080/t/acme \
VITE_CLIENT_ID=app_demo_cli \
VITE_REDIRECT_URI=http://localhost:5173/callback \
pnpm dev
```

Register a different public client in **Admin → Applications** if you change the redirect
URI. Never put a confidential client secret in browser code.
