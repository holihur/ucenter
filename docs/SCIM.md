# SCIM 2.0 provisioning

UCenter exposes a tenant-scoped SCIM 2.0 endpoint for automated user and group
provisioning. SCIM Groups map directly to tenant RBAC roles, so group membership controls
the roles a provisioned user receives.

## Endpoint

```text
https://id.example.com/t/{tenant}/scim/v2
```

Routes:

```text
GET  /ServiceProviderConfig
GET  /Schemas
GET  /Users            POST /Users
GET  /Users/{id}       PUT/PATCH/DELETE /Users/{id}
GET  /Groups           POST /Groups
GET  /Groups/{id}      PUT/PATCH/DELETE /Groups/{id}
```

Authentication is a tenant-scoped bearer token (`ucs_...`) generated in
**Admin → SCIM provisioning**. Only a SHA-256 hash is stored. Disabling SCIM or rotating
the token invalidates previous tokens immediately.

Headers:

```text
Authorization: Bearer ucs_...
Content-Type: application/scim+json
```

## Users

Supported attributes:

- `userName` (must be a valid email; used as the UCenter identity)
- `externalId`
- `name.formatted`, `name.givenName`, `name.familyName`
- `displayName`
- `emails` (primary)
- `active`
- `roles` (`value` = role key) and `groups` (`value` = role id)
- `meta`

Filtering supports `eq` for `userName`, `externalId` and `active`. Pagination uses
`startIndex` (1-based) and `count` (max 200).

Creating with an existing `userName` or `externalId` returns `409`. PUT is idempotent.
PATCH supports simple `replace`/`add`/`remove` operations for `active`, `userName`,
`externalId`, `displayName`, `name.*`, `roles` and `groups`. Deactivating a user revokes
sessions, refresh tokens and bumps the access-token version.

## Groups

`displayName` is the role display name. `externalId` becomes the role key when supplied;
otherwise it is derived from `displayName`. Members are UCenter user IDs.

- Creating a group creates a custom tenant role with no administrative permissions.
- Adding/removing members adds/removes that role on the user.
- System groups (`owner`, `admin`, `member`) are visible but cannot be deleted.
- The last owner cannot be removed via group membership changes.

Roles can be granted admin permissions afterwards from **Admin → Roles**.

## Error format

```json
{
  "schemas": ["urn:ietf:params:scim:api:messages:2.0:Error"],
  "detail": "userName already exists",
  "status": "409"
}
```

## IdP configuration

Use the Users/Groups endpoints shown in the SCIM page. For Okta, Entra ID, OneLogin or
JumpCloud, paste the tenant SCIM base URL and bearer token into the provisioning connector.
Test the connection with `GET /ServiceProviderConfig`.

## Local verification

```bash
BASE_URL=http://localhost:8080 ./scripts/scim-smoke.sh
```

The script covers token auth, create/filter/PUT idempotency, deactivate, group membership
to role mapping, deletion, duplicate/error paths and cross-tenant token rejection.
