# Next.js integration

Use the tenant discovery document with Auth.js (NextAuth) or any OIDC library:

```ts
import NextAuth from "next-auth";
import Keycloak from "next-auth/providers/keycloak";

export const { handlers, auth } = NextAuth({
  providers: [
    Keycloak({
      issuer: "http://localhost:8080/t/acme",
      clientId: "app_demo_acme",
      clientSecret: process.env.UCENTER_CLIENT_SECRET!,
sense: undefined,
    }),
  ],
});
```

Generic `next-auth` OIDC provider:

```ts
{
  id: "ucenter",
  name: "UCenter",
  type: "oidc",
  issuer: "http://localhost:8080/t/acme",
  clientId: "app_demo_acme",
  clientSecret: process.env.UCENTER_CLIENT_SECRET!,
  authorization: { params: { scope: "openid profile email roles" } },
}
```

Copy `.env.local`:

```bash
UCENTER_CLIENT_SECRET=demo-secret-acme
AUTH_SECRET=replace-with-a-long-random-value
NEXTAUTH_URL=http://localhost:3000
```

Register `http://localhost:3000/api/auth/callback/ucenter` as a redirect URI in UCenter.
The demo client already allows `http://localhost:3000/callback`; add the Auth.js callback
URI before running. This directory is documentation-only; use your app's Next.js setup.
