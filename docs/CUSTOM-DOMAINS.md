# Custom domains and white-label branding

Each tenant can own multiple hostnames. UCenter resolves the tenant from the request
hostname (or the `?domain=` preview parameter), then applies that tenant's branding and
login template.

## Configure a domain

1. Add the hostname under **Admin → Settings → Domains** (one per line). Names are
   normalized to lowercase and must be valid DNS names.
2. Point DNS at your reverse proxy:

   ```dns
   login.acme.example.  CNAME  sso.example.com.
   ```

3. Configure TLS at the proxy and forward the original host:

   ```nginx
   server {
     listen 443 ssl http2;
     server_name login.acme.example;
     ssl_certificate     /etc/letsencrypt/live/login.acme.example/fullchain.pem;
     ssl_certificate_key /etc/letsencrypt/live/login.acme.example/privkey.pem;

     location / {
       proxy_pass http://ucenter:8080;
       proxy_set_header Host $host;
       proxy_set_header X-Forwarded-Proto $scheme;
       proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
     }
   }
   ```

   Caddy equivalent:

   ```caddyfile
   login.acme.example {
     reverse_proxy ucenter:8080
   }
   ```

   Set `TRUSTED_PROXIES` so client IPs in audit logs and rate limiting are correct.

## Resolution and branding

- `GET /api/public/tenant-by-host?domain=login.acme.example` returns the public tenant.
- The login page calls this endpoint for the current hostname when no explicit
  `?tenant=` is present; `?domain=` is useful for local preview/testing.
- Branding fields (`brandName`, `hidePlatformBranding`, logo, colors, template, tagline)
  are applied before the form renders.
- A domain can belong to only one tenant. Conflicts return `400 domain_in_use` from
  `PATCH /api/admin/settings` and `PATCH /api/admin/tenants/{id}`.
- Disabling/deleting a tenant stops domain resolution; restoring it resumes.

## White-label

Enable **Hide platform branding** and set **Brand name** to replace the UCenter mark with
the tenant logo/name on sign-in, registration and consent pages. OIDC discovery and token
endpoints remain standards-compliant and unchanged.

## OIDC issuer note

The issuer is the deployed `BASE_URL` (for example `https://sso.example.com`), not each
custom hostname. Register the issuer with applications once; custom domains are for
human-facing sign-in. If every tenant needs a distinct issuer, deploy a per-domain reverse
proxy that rewrites to the tenant path and treats that domain as `BASE_URL`.

## Verification

- `scripts/tenant-smoke.sh` covers hostname resolution, duplicate-domain rejection,
  tenant disable/restore and purge.
- Playwright covers `?domain=` tenant selection and white-label branding.
