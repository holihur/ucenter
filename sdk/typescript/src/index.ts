export interface Branding {
  logoUrl: string;
  primaryColor: string;
  backgroundColor: string;
  supportEmail: string;
  loginTemplate?: string;
  loginTagline?: string;
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
  settings: Record<string, unknown>;
  deletedAt?: string | null;
  purgeAfter?: string | null;
}

export interface User {
  id: string;
  email: string;
  emailVerified: boolean;
  username: string;
  name: string;
  status: string;
  platformAdmin: boolean;
  roles?: string[];
  mfa?: { enabled: boolean };
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
}

export interface OidcClient {
  id: string;
  clientId: string;
  name: string;
  type: "confidential" | "public";
  redirectUris: string[];
  postLogoutRedirectUris: string[];
  allowedScopes: string[];
  grantTypes: string[];
  requirePkce: boolean;
  trusted: boolean;
  enabled: boolean;
  secretSet: boolean;
}

export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
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

export interface LoginResult {
  mfaRequired: boolean;
  mfaToken?: string;
  mfaMethods?: string[];
  user?: User;
  tenant?: PublicTenant;
  next?: string;
  requestId?: string;
}

export interface AuditLog {
  id: string;
  action: string;
  actorEmail: string;
  targetType: string;
  targetId: string;
  ip: string;
  createdAt: string;
}

export interface ClientOptions {
  baseURL: string;
  tenantId?: string;
  cookie?: string;
  headers?: Record<string, string>;
  fetch?: typeof fetch;
}

export class UCenterError extends Error {
  status: number;
  code: string;
  constructor(message: string, status: number, code = "error") {
    super(message);
    this.name = "UCenterError";
    this.status = status;
    this.code = code;
  }
}

export class UCenterClient {
  readonly baseURL: string;
  tenantId?: string;
  cookie?: string;
  headers: Record<string, string>;
  private fetchImpl: typeof fetch;

  constructor(options: ClientOptions) {
    this.baseURL = options.baseURL.replace(/\/$/, "");
    this.tenantId = options.tenantId;
    this.cookie = options.cookie;
    this.headers = options.headers ?? {};
    this.fetchImpl = options.fetch ?? fetch;
  }

  private async request<T>(path: string, init: (Omit<RequestInit, "body"> & { body?: unknown }) = {}): Promise<T> {
    const headers = new Headers(this.headers);
    if (this.cookie) headers.set("Cookie", this.cookie);
    if (this.tenantId) headers.set("X-Tenant-ID", this.tenantId);
    let body = init.body as BodyInit | undefined;
    if (body && typeof body !== "string" && !(body instanceof FormData)) {
      headers.set("Content-Type", "application/json");
      body = JSON.stringify(body);
    }
    const response = await this.fetchImpl(`${this.baseURL}${path.startsWith("/") ? path : `/${path}`}`, {
      ...init,
      body,
      headers,
      credentials: "include",
    } as RequestInit);
    const setCookies = (response.headers as Headers & { getSetCookie?: () => string[] }).getSetCookie?.() ?? [];
    const setCookie = setCookies[0] ?? response.headers.get("set-cookie");
    if (setCookie) this.cookie = setCookie.split(";")[0];
    const text = await response.text();
    const data = text ? safeJSON(text) : null;
    if (!response.ok) {
      const payload = data as { error?: { code?: string; message?: string } | string } | null;
      const message =
        typeof payload?.error === "string"
          ? payload.error
          : payload?.error?.message ?? `Request failed (${response.status})`;
      const code = typeof payload?.error === "object" ? payload.error?.code ?? "error" : "error";
      throw new UCenterError(message, response.status, code);
    }
    return data as T;
  }

  setTenant(tenantId: string | undefined) {
    this.tenantId = tenantId;
    return this;
  }

  login(input: { tenant: string; email: string; password: string; totp?: string; requestId?: string }) {
    return this.request<LoginResult>("/api/auth/login", { method: "POST", body: input });
  }

  me() {
    return this.request<MeResponse>("/api/auth/me");
  }

  logout() {
    return this.request<{ ok: boolean }>("/api/auth/logout", { method: "POST" });
  }

  listUsers(params: { q?: string; status?: string; page?: number; pageSize?: number } = {}) {
    const query = new URLSearchParams();
    if (params.q) query.set("q", params.q);
    if (params.status) query.set("status", params.status);
    query.set("page", String(params.page ?? 1));
    query.set("pageSize", String(params.pageSize ?? 20));
    return this.request<Page<User>>(`/api/admin/users?${query}`);
  }

  createUser(input: {
    email: string;
    name: string;
    username?: string;
    roles?: string[];
    password?: string;
  }) {
    return this.request<{ user: User; tempPassword: string; inviteSent?: boolean }>("/api/admin/users", {
      method: "POST",
      body: input,
    });
  }

  updateUser(id: string, input: Partial<Pick<User, "name" | "status">> & { roles?: string[]; username?: string }) {
    return this.request<{ user: User }>(`/api/admin/users/${id}`, { method: "PATCH", body: input });
  }

  deleteUser(id: string) {
    return this.request<{ ok: boolean }>(`/api/admin/users/${id}`, { method: "DELETE" });
  }

  listRoles() {
    return this.request<{ items: Role[] }>("/api/admin/roles");
  }

  listClients() {
    return this.request<{ items: OidcClient[] }>("/api/admin/clients");
  }

  getOverview() {
    return this.request<{ stats: Record<string, number>; recentAudit: AuditLog[]; recentUsers: User[] }>(
      "/api/admin/overview",
    );
  }

  getSettings() {
    return this.request<{ tenant: Tenant }>("/api/admin/settings");
  }

  updateSettings(input: Record<string, unknown>) {
    return this.request<{ tenant: Tenant }>("/api/admin/settings", { method: "PATCH", body: input });
  }

  listTenants() {
    return this.request<{ items: Tenant[] }>("/api/admin/tenants");
  }

  createTenant(input: { slug: string; name: string; ownerEmail?: string; ownerName?: string }) {
    return this.request<{ tenant: Tenant; tempPassword: string }>("/api/admin/tenants", {
      method: "POST",
      body: input,
    });
  }

  exportTenant() {
    return this.request<Record<string, unknown>>("/api/admin/export");
  }
}

function safeJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}
