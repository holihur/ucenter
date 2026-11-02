export class ApiError extends Error {
  status: number;
  code: string;

  constructor(message: string, status: number, code = "error") {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

const TENANT_KEY = "uc_admin_tenant";

export function getAdminTenantId() {
  return localStorage.getItem(TENANT_KEY) || "";
}

export function setAdminTenantId(id: string | null) {
  if (id) localStorage.setItem(TENANT_KEY, id);
  else localStorage.removeItem(TENANT_KEY);
}

type Options = Omit<RequestInit, "body"> & { body?: unknown };

async function request<T>(path: string, options: Options = {}): Promise<T> {
  const headers = new Headers(options.headers);
  const tenantId = getAdminTenantId();
  if (tenantId) headers.set("X-Tenant-ID", tenantId);
  let body = options.body as BodyInit | undefined;
  if (body !== undefined && !(body instanceof FormData) && typeof body !== "string") {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(body);
  }
  const res = await fetch(path.startsWith("/api") ? path : `/api${path}`, {
    ...options,
    body,
    headers,
    credentials: "include",
  });
  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = text;
    }
  }
  if (!res.ok) {
    const payload = data as
      | { error?: { message?: string; code?: string } | string; error_description?: string }
      | null;
    let message = `Request failed (${res.status})`;
    let code = "error";
    if (payload && typeof payload === "object") {
      if (typeof payload.error === "string") {
        message = payload.error_description || payload.error;
        code = payload.error;
      } else if (payload.error) {
        message = payload.error.message || message;
        code = payload.error.code || code;
      }
    }
    throw new ApiError(message, res.status, code);
  }
  return data as T;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: "POST", body }),
  patch: <T>(path: string, body?: unknown) => request<T>(path, { method: "PATCH", body }),
  del: <T>(path: string) => request<T>(path, { method: "DELETE" }),
};

export interface Branding {
  logoUrl: string;
  primaryColor: string;
  backgroundColor: string;
  supportEmail: string;
}

export interface TenantSettings {
  allowRegistration: boolean;
  requireMfa: boolean;
  passwordMinLength: number;
  sessionTtlHours: number;
  allowedEmailDomains: string[] | null;
  allowPasswordLogin: boolean;
}

export interface PublicTenant {
  id?: string;
  slug: string;
  name: string;
  status: string;
  branding: Branding;
  allowRegistration: boolean;
  allowPasswordLogin: boolean;
  requireMfa: boolean;
}

export interface Tenant extends PublicTenant {
  id: string;
  domains: string[] | null;
  settings: TenantSettings;
  createdAt: string;
  updatedAt: string;
}

export interface Membership {
  tenantId: string;
  roles: string[];
  joinedAt: string;
}

export interface User {
  id: string;
  email: string;
  emailVerified: boolean;
  username: string;
  name: string;
  picture: string;
  status: string;
  platformAdmin: boolean;
  memberships: Membership[];
  mfa: { enabled: boolean; enabledAt?: string | null };
  lastLoginAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface MeResponse {
  user: User;
  tenant: PublicTenant;
  roles: string[];
  permissions: string[];
  memberships: { tenant: PublicTenant; roles: string[] }[];
  isPlatformAdmin: boolean;
  mfaSetupRequired: boolean;
}

export interface AdminUser {
  id: string;
  email: string;
  emailVerified: boolean;
  username: string;
  name: string;
  picture: string;
  status: string;
  platformAdmin: boolean;
  mfaEnabled: boolean;
  roles: string[];
  lastLoginAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Role {
  id: string;
  key: string;
  name: string;
  description: string;
  permissions: string[];
  system: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Permission {
  key: string;
  label: string;
  description: string;
  group: string;
}

export interface OidcClient {
  id: string;
  clientId: string;
  name: string;
  description: string;
  type: "confidential" | "public";
  redirectUris: string[] | null;
  postLogoutRedirectUris: string[] | null;
  allowedScopes: string[] | null;
  grantTypes: string[] | null;
  requirePkce: boolean;
  trusted: boolean;
  accessTokenTtl: number;
  idTokenTtl: number;
  refreshTokenTtl: number;
  enabled: boolean;
  logoUrl: string;
  secretSet: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface AuditLog {
  id: string;
  tenantId: string;
  actorId?: string;
  actorEmail: string;
  action: string;
  targetType: string;
  targetId: string;
  ip: string;
  userAgent: string;
  metadata?: Record<string, unknown>;
  createdAt: string;
}

export interface SessionItem {
  id: string;
  userId: string;
  email: string;
  name: string;
  ip: string;
  userAgent: string;
  mfa: boolean;
  createdAt: string;
  expiresAt: string;
  authTime: string;
  current?: boolean;
}

export interface AuthRequestInfo {
  id: string;
  status: "pending" | "authenticated" | "approved" | "denied" | "completed" | "expired";
  prompt: string;
  loginHint: string;
  expiresAt: string;
  tenant: PublicTenant;
  client: { clientId: string; name: string; logoUrl: string; trusted: boolean };
  scopes: { key: string; description: string }[];
  user?: User;
}

export interface Paginated<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}
