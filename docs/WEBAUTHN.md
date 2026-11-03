# WebAuthn / Passkeys

UCenter supports WebAuthn passkeys for both MFA and step-up verification.

## Enrolment

Signed-in users open **Account → Passkeys → Add passkey**. The browser performs
`navigator.credentials.create()` through `@simplewebauthn/browser`; the server verifies
the attestation and stores:

- credential ID and public key
- AAGUID, signature counter, transports
- backup eligibility/state
- display name, created/last-used timestamps

Credentials are scoped to `(tenant, user)` and are never returned as private material.

## Login

When a user has one or more passkeys, password login returns:

```json
{ "mfaRequired": true, "mfaToken": "mfa_...", "mfaMethods": ["webauthn"] }
```

The client calls:

1. `POST /api/auth/webauthn/begin` with `mfaToken`
2. browser `navigator.credentials.get()` using the returned `options.publicKey`
3. `POST /api/auth/webauthn/finish` with `sessionId` and the assertion

A successful assertion creates an MFA-marked UCenter session. TOTP and passkeys can
coexist; the login response lists both methods. A user with TOTP can still submit the
6-digit code, while passkey users don't need a password reset or authenticator app.

## Step-up

Authenticated users can request a fresh passkey assertion:

```text
POST /api/me/step-up/webauthn/begin
POST /api/me/step-up/webauthn/finish
```

The finish endpoint records `step_up_at` on the session and writes an audit event. Product
surfaces that need stronger assurance can require a recent step-up timestamp.

## API

```text
POST   /api/auth/webauthn/begin
POST   /api/auth/webauthn/finish
POST   /api/me/webauthn/register/begin
POST   /api/me/webauthn/register/finish
GET    /api/me/webauthn/credentials
DELETE /api/me/webauthn/credentials/{id}
POST   /api/me/step-up/webauthn/begin
POST   /api/me/step-up/webauthn/finish
```

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `WEBAUTHN_RP_ID` | hostname of `BASE_URL` | Relying Party ID; must be the registrable domain (or `localhost`) |
| `WEBAUTHN_ORIGINS` | `BASE_URL` | Comma-separated allowed origins |

In development, the Vite origins `http://localhost:5173` and `http://127.0.0.1:5173` are
added automatically. Use `localhost` in `BASE_URL` for local passkey testing; WebAuthn
does not accept arbitrary IP addresses as RP IDs.

## Security

- Challenges and session data expire after five minutes and are single-use.
- Signature counters are updated after every assertion; clone warnings are surfaced by the
  library.
- Credential deletion is audited and requires a valid session.
- TOTP MFA and passkeys both count as multi-factor; users without either still follow the
  tenant's `Require MFA` policy.
- The CI/mock flow uses Chromium's virtual authenticator and never requires a hardware
  security key.
