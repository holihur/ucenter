# SAML 2.0 Service Provider

UCenter can act as a tenant-level SAML 2.0 Service Provider. SAML assertions are
validated with `crewjam/saml` (signature, issuer/audience, conditions/time and request ID),
then mapped to a UCenter user and session. Users can be provisioned JIT or linked to an
existing account.

## Endpoints

```text
GET  /t/{tenant}/saml/metadata   SP metadata XML
GET  /t/{tenant}/saml/login      start SP-initiated login
POST /t/{tenant}/saml/acs        assertion consumer service
```

The admin console **Identity providers → SAML 2.0** configures the tenant.

## Admin configuration

1. Generate or paste the IdP metadata XML from the customer IdP.
2. Save once; UCenter generates a stable SP RSA key pair and self-signed certificate
   (encrypted at rest with `ENCRYPTION_KEY`).
3. Give the IdP these values:
   - **SP Entity ID / Audience**: `https://id.example.com/t/acme/saml/metadata`
   - **ACS URL**: `https://id.example.com/t/acme/saml/acs`
   - **SP metadata**: `https://id.example.com/t/acme/saml/metadata`
4. Optionally map attribute names:
   - `emailAttribute` (default `email`)
   - `nameAttribute` (default `displayName`)
   - `groupsAttribute` (reserved for role mapping)
5. Enable the provider. The login page shows the SAML button; users can also link it from
   **Account → Linked sign-in methods**.

## Validation

The SP calls `ParseResponse` with the stored AuthnRequest ID, which validates:

- XML signature using the IdP metadata certificate
- `AudienceRestriction` against the SP Entity ID
- `NotBefore` / `NotOnOrAfter` with clock skew
- response `Destination` and request correlation
- Replay protection via the one-time `RelayState` state record (10 minutes)

## User mapping

- `NameID` is the provider subject. If it looks like an email and no email attribute is
  present, it is used as the email.
- If the identity is already linked, login is immediate.
- If the email matches an existing member, linking requires **Trust provider email**;
  otherwise the user must sign in first and link from Account.
- With **Auto create users**, a member is provisioned with the configured default roles.
- TOTP MFA users cannot bypass MFA via SAML.

## Mock / local testing

Use the `mock` provider for local JIT/link E2E tests. For SAML protocol tests, use a local
SAML IdP such as `samlidp` or a test IdP container and point `idpMetadataXML` at its
metadata. No real corporate IdP credentials are required for development.

## Limitations

- SP-initiated HTTP-Redirect and IdP-initiated POST responses are supported; SAML logout
  (SLO) is not yet wired to UCenter sessions.
- Group-to-role mapping is reserved but not applied; default roles are used.
- Encrypted assertions are not explicitly configured; signed assertions are required.
- The SP certificate is self-signed and rotated only by re-saving the provider config.
