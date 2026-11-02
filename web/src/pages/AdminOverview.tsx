import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  AppWindow,
  ArrowUpRight,
  KeyRound,
  MonitorSmartphone,
  ScrollText,
  ShieldCheck,
  UserPlus,
  Users,
} from "lucide-react";
import { PageHeader } from "@/components/PageHeader";
import { StatusBadge } from "@/components/StatusBadge";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useAuth } from "@/lib/auth";
import { useAdminTenant } from "@/lib/useAdminTenant";
import { useI18n } from "@/lib/i18n";
import { api, type AdminUser, type AuditLog } from "@/lib/api";
import { initials, shortId, timeAgo } from "@/lib/utils";

interface OverviewData {
  stats: {
    users: number;
    activeUsers: number;
    clients: number;
    roles: number;
    sessions: number;
    events24h: number;
  };
  recentAudit: AuditLog[];
  recentUsers: AdminUser[];
}

const actionLabels: Record<string, string> = {
  "auth.login.success": "Signed in",
  "auth.login.failed": "Failed sign-in",
  "auth.logout": "Signed out",
  "auth.mfa.failed": "Failed MFA challenge",
  "oidc.authorize.approved": "Consent granted",
  "oidc.authorize.denied": "Consent denied",
  "oidc.token.issued": "Token issued",
  "user.created": "User created",
  "user.registered": "User registered",
  "user.updated": "User updated",
  "user.deleted": "User removed",
  "user.password.changed": "Password changed",
  "user.password.reset": "Password reset",
  "user.mfa.enabled": "MFA enabled",
  "user.mfa.disabled": "MFA disabled",
  "session.revoked": "Session revoked",
  "sessions.revoked": "Sessions revoked",
  "role.created": "Role created",
  "role.updated": "Role updated",
  "role.deleted": "Role deleted",
  "client.created": "Application created",
  "client.updated": "Application updated",
  "client.deleted": "Application deleted",
  "client.secret.rotated": "Client secret rotated",
  "tenant.created": "Tenant created",
  "tenant.updated": "Tenant updated",
  "tenant.settings.updated": "Settings updated",
};

function actionLabel(action: string) {
  return actionLabels[action] ?? action.replaceAll(".", " ");
}

function actionTone(action: string) {
  if (action.includes("failed") || action.includes("denied") || action.includes("deleted")) return "destructive" as const;
  if (action.includes("created") || action.includes("registered")) return "success" as const;
  if (action.includes("updated") || action.includes("rotated")) return "warning" as const;
  return "default" as const;
}

export function AdminOverview() {
  const { can, me } = useAuth();
  const tenant = useAdminTenant();
  const { t } = useI18n();
  const query = useQuery({
    queryKey: ["admin-overview"],
    queryFn: () => api.get<OverviewData>("/admin/overview"),
  });
  const data = query.data;

  const cards = [
    { label: "Total users", value: data?.stats.users, icon: Users, hint: `${data?.stats.activeUsers ?? 0} active`, to: "/admin/users" },
    { label: "Applications", value: data?.stats.clients, icon: AppWindow, hint: "OIDC clients", to: "/admin/clients" },
    { label: "Active sessions", value: data?.stats.sessions, icon: MonitorSmartphone, hint: "Right now", to: "/admin/sessions" },
    { label: "Roles", value: data?.stats.roles, icon: ShieldCheck, hint: "Access policies", to: "/admin/roles" },
    { label: "Events · 24h", value: data?.stats.events24h, icon: Activity, hint: "Audit activity", to: "/admin/audit" },
  ];

  return (
    <div className="space-y-6">
      <PageHeader
        title={`${t("Welcome back")}, ${me?.user.name?.split(" ")[0] ?? "there"}`}
        description={t("Here is what is happening across {tenant}.", { tenant: tenant?.name ?? "" })}
        actions={
          <>
            {can("users:write") && (
              <Button asChild>
                <Link to="/admin/users?new=1">
                  <UserPlus className="h-4 w-4" />
                  {t("Invite user")}
                </Link>
              </Button>
            )}
            {can("clients:write") && (
              <Button variant="outline" asChild>
                <Link to="/admin/clients?new=1">
                  <AppWindow className="h-4 w-4" />
                  {t("New application")}
                </Link>
              </Button>
            )}
          </>
        }
      />

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
        {cards.map((card) => (
          <Link key={card.label} to={card.to} className="group">
            <Card className="h-full transition-all group-hover:-translate-y-0.5 group-hover:shadow-lg">
              <CardContent className="p-5">
                <div className="flex items-start justify-between">
                  <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <card.icon className="h-4 w-4" />
                  </div>
                  <ArrowUpRight className="h-4 w-4 text-muted-foreground/50 transition-colors group-hover:text-primary" />
                </div>
                <div className="mt-4">
                  {query.isLoading ? (
                    <Skeleton className="h-8 w-16" />
                  ) : (
                    <p className="text-2xl font-semibold tracking-tight">{card.value ?? 0}</p>
                  )}
                  <p className="text-sm font-medium">{t(card.label)}</p>
                  <p className="text-xs text-muted-foreground">{t(card.hint)}</p>
                </div>
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>

      <div className="grid gap-6 xl:grid-cols-[1.15fr_1fr]">
        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <div>
              <CardTitle>{t("Recent activity")}</CardTitle>
              <CardDescription>{t("Latest security and administration events")}</CardDescription>
            </div>
            {can("audit:read") && (
              <Button variant="ghost" size="sm" asChild>
                <Link to="/admin/audit">{t("View all")}</Link>
              </Button>
            )}
          </CardHeader>
          <CardContent>
            {query.isLoading ? (
              <div className="space-y-3">
                {Array.from({ length: 5 }).map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : data?.recentAudit?.length ? (
              <div className="space-y-1">
                {data.recentAudit.map((event) => (
                  <div key={event.id} className="flex items-start gap-3 rounded-lg px-2 py-2.5 hover:bg-muted/50">
                    <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted">
                      <ScrollText className="h-3.5 w-3.5 text-muted-foreground" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant={actionTone(event.action)}>{t(actionLabel(event.action))}</Badge>
                        <span className="truncate text-xs text-muted-foreground">{event.actorEmail || "system"}</span>
                      </div>
                      <p className="mt-1 truncate text-xs text-muted-foreground">
                        {event.targetType}
                        {event.targetId ? ` · ${shortId(event.targetId)}` : ""} · {event.ip || "unknown IP"}
                      </p>
                    </div>
                    <span className="shrink-0 text-[11px] text-muted-foreground">{timeAgo(event.createdAt)}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="py-10 text-center text-sm text-muted-foreground">{t("No activity recorded yet.")}</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <div>
              <CardTitle>{t("Newest users")}</CardTitle>
              <CardDescription>{t("Recently added members of this tenant")}</CardDescription>
            </div>
            {can("users:read") && (
              <Button variant="ghost" size="sm" asChild>
                <Link to="/admin/users">{t("View all")}</Link>
              </Button>
            )}
          </CardHeader>
          <CardContent>
            {query.isLoading ? (
              <div className="space-y-3">
                {Array.from({ length: 4 }).map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : data?.recentUsers?.length ? (
              <div className="space-y-1">
                {data.recentUsers.map((user) => (
                  <div key={user.id} className="flex items-center gap-3 rounded-lg px-2 py-2.5 hover:bg-muted/50">
                    <Avatar className="h-9 w-9">
                      {user.picture && <AvatarImage src={user.picture} alt={user.name} />}
                      <AvatarFallback>{initials(user.name, user.email)}</AvatarFallback>
                    </Avatar>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{user.name || user.email}</p>
                      <p className="truncate text-xs text-muted-foreground">{user.email}</p>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <StatusBadge status={user.status} />
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className="py-10 text-center text-sm text-muted-foreground">{t("No users yet.")}</p>
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("Quick start")}</CardTitle>
          <CardDescription>{t("Connect an application using standard OpenID Connect discovery")}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-3">
          {[
            {
              icon: AppWindow,
              title: "1. Register an application",
              text: "Create an OIDC client with redirect URIs and scopes.",
              to: "/admin/clients",
              permission: "clients:read",
            },
            {
              icon: KeyRound,
              title: "2. Configure the SDK",
              text: "Point your OIDC library at the tenant discovery document.",
              to: `/t/${tenant?.slug ?? ""}/.well-known/openid-configuration`,
              external: true,
            },
            {
              icon: ShieldCheck,
              title: "3. Assign roles",
              text: "Grant tenant roles to control access in the console.",
              to: "/admin/roles",
              permission: "roles:read",
            },
          ].map((step) => {
            if (step.permission && !can(step.permission)) return null;
            const content = (
              <div className="h-full rounded-lg border bg-background p-4 transition-colors hover:border-primary/40">
                <step.icon className="h-5 w-5 text-primary" />
                <p className="mt-3 text-sm font-medium">{t(step.title)}</p>
                <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{t(step.text)}</p>
              </div>
            );
            return step.external ? (
              <a key={step.title} href={step.to} target="_blank" rel="noreferrer">
                {content}
              </a>
            ) : (
              <Link key={step.title} to={step.to}>
                {content}
              </Link>
            );
          })}
        </CardContent>
      </Card>
    </div>
  );
}
