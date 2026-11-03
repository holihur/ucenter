import { useEffect, useMemo, useState } from "react";
import { Link, NavLink, Navigate, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AppWindow,
  Bot,
  Building2,
  Check,
  ChevronDown,
  LayoutDashboard,
  LogIn,
  LogOut,
  Mail,
  Menu,
  MonitorSmartphone,
  ScrollText,
  Settings,
  ShieldCheck,
  UserRound,
  Users,
  X,
} from "lucide-react";
import { BrandMark, TenantLogo } from "@/components/TenantLogo";
import { PreferencesControls } from "@/components/Preferences";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Skeleton } from "@/components/ui/skeleton";
import { useAuth } from "@/lib/auth";
import {
  api,
  getAdminTenantId,
  setAdminTenantId,
  type MeResponse,
  type Tenant,
} from "@/lib/api";
import { applyBrandColor, cn, initials } from "@/lib/utils";

interface NavItem {
  to: string;
  label: string;
  icon: typeof Users;
  permission?: string;
  end?: boolean;
}

const navItems: NavItem[] = [
  { to: "/admin", label: "Overview", icon: LayoutDashboard, end: true },
  { to: "/admin/users", label: "Users", icon: Users, permission: "users:read" },
  { to: "/admin/roles", label: "Roles", icon: ShieldCheck, permission: "roles:read" },
  { to: "/admin/clients", label: "Applications", icon: AppWindow, permission: "clients:read" },
  { to: "/admin/identity-providers", label: "Identity providers", icon: LogIn, permission: "tenant:read" },
  { to: "/admin/mcp", label: "MCP access", icon: Bot },
  { to: "/admin/mail", label: "Mail outbox", icon: Mail, permission: "tenant:write" },
  { to: "/admin/sessions", label: "Sessions", icon: MonitorSmartphone, permission: "sessions:read" },
  { to: "/admin/audit", label: "Audit log", icon: ScrollText, permission: "audit:read" },
  { to: "/admin/settings", label: "Settings", icon: Settings, permission: "tenant:read" },
];

export function AdminLayout() {
  const { me, loading, can, logout } = useAuth();
  const { t } = useI18n();
  const navigate = useNavigate();
  const location = useLocation();
  const queryClient = useQueryClient();
  const isPlatform = Boolean(me?.isPlatformAdmin);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [tenantId, setTenantIdState] = useState(() => getAdminTenantId());
  const [ready, setReady] = useState(!isPlatform);

  const tenantsQuery = useQuery({
    queryKey: ["admin-tenants"],
    queryFn: () => api.get<{ items: Tenant[] }>("/admin/tenants"),
    enabled: isPlatform,
    retry: false,
  });
  const tenants = tenantsQuery.data?.items ?? [];

  useEffect(() => {
    if (!isPlatform) {
      setReady(true);
      return;
    }
    if (tenants.length === 0) {
      if (tenantsQuery.isFetched) setReady(true);
      return;
    }
    const valid = tenants.find((t) => t.id === tenantId);
    const chosen = valid ?? tenants[0];
    if (chosen.id !== tenantId) {
      setAdminTenantId(chosen.id);
      setTenantIdState(chosen.id);
    }
    setReady(true);
  }, [isPlatform, tenants, tenantId]);

  const contextQuery = useQuery<MeResponse>({
    queryKey: ["admin-context", tenantId],
    queryFn: () => api.get<MeResponse>("/admin/context"),
    enabled: Boolean(me) && ready,
    retry: false,
  });

  const context = contextQuery.data;
  const activeTenant = context?.tenant;

  useEffect(() => {
    if (activeTenant) applyBrandColor(activeTenant.branding.primaryColor);
  }, [activeTenant?.branding?.primaryColor]);

  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname]);

  const hasAdminAccess = useMemo(
    () => Boolean(me?.permissions?.some((p) => p !== "platform:manage")),
    [me?.permissions],
  );

  function switchTenant(id: string) {
    setAdminTenantId(id);
    setTenantIdState(id);
    queryClient.invalidateQueries();
    navigate("/admin");
  }

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <BrandMark className="h-9 w-9 animate-pulse" />
          {t("Loading workspace…")}
        </div>
      </div>
    );
  }

  if (!me) return <Navigate to="/login" replace />;

  if (!hasAdminAccess) return <Navigate to="/account" replace />;

  const visibleNav = navItems.filter((item) => !item.permission || can(item.permission));

  return (
    <div className="min-h-screen bg-background">
      {mobileOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm lg:hidden"
          onClick={() => setMobileOpen(false)}
        />
      )}

      <aside
        className={cn(
          "fixed inset-y-0 start-0 z-50 flex w-[270px] flex-col border-e bg-card transition-transform duration-200 lg:translate-x-0",
          mobileOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <div className="flex h-16 items-center justify-between border-b px-5">
          <Link to="/admin" className="flex items-center gap-2.5">
            <BrandMark className="h-8 w-8" />
            <div>
              <p className="text-sm font-semibold leading-tight">UCenter</p>
              <p className="text-[11px] text-muted-foreground">{t("Admin console")}</p>
            </div>
          </Link>
          <Button variant="ghost" size="icon" className="lg:hidden" onClick={() => setMobileOpen(false)}>
            <X className="h-4 w-4" />
          </Button>
        </div>

        <div className="border-b p-3">
          {isPlatform ? (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <button className="flex w-full items-center gap-3 rounded-lg border bg-background p-2.5 text-start transition-colors hover:bg-accent">
                  <TenantLogo tenant={activeTenant} className="h-9 w-9" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{activeTenant?.name ?? t("Tenants")}</p>
                    <p className="truncate text-[11px] text-muted-foreground">
                      {activeTenant?.slug ?? t("Platform")}
                    </p>
                  </div>
                  <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent className="w-[246px]" align="start">
                <DropdownMenuLabel>{t("Switch organization")}</DropdownMenuLabel>
                {tenants.map((t) => (
                  <DropdownMenuItem key={t.id} onSelect={() => switchTenant(t.id)} className="gap-2">
                    <TenantLogo tenant={t} className="h-6 w-6" rounded="rounded-md" />
                    <span className="flex-1 truncate">{t.name}</span>
                    {t.id === tenantId && <Check className="h-3.5 w-3.5 text-primary" />}
                  </DropdownMenuItem>
                ))}
                <DropdownMenuSeparator />
                <DropdownMenuItem onSelect={() => navigate("/admin/tenants")} className="gap-2">
                  <Building2 className="h-4 w-4" />
                  {t("Manage tenants")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          ) : (
            <div className="flex items-center gap-3 rounded-lg border bg-background p-2.5">
              <TenantLogo tenant={activeTenant} className="h-9 w-9" />
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{activeTenant?.name}</p>
                <p className="truncate text-[11px] text-muted-foreground">{activeTenant?.slug}</p>
              </div>
            </div>
          )}
        </div>

        <nav className="flex-1 space-y-1 overflow-y-auto p-3">
          {visibleNav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                  isActive
                    ? "bg-primary/10 text-primary"
                    : "text-muted-foreground hover:bg-accent hover:text-foreground",
                )
              }
            >
              <item.icon className="h-4 w-4" />
              {t(item.label)}
            </NavLink>
          ))}

          {can("platform:manage") && (
            <>
              <p className="px-3 pb-1 pt-4 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                {t("Platform")}
              </p>
              <NavLink
                to="/admin/tenants"
                className={({ isActive }) =>
                  cn(
                    "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                    isActive
                      ? "bg-primary/10 text-primary"
                      : "text-muted-foreground hover:bg-accent hover:text-foreground",
                  )
                }
              >
                <Building2 className="h-4 w-4" />
                {t("Tenants")}
              </NavLink>
            </>
          )}
        </nav>

        <div className="border-t p-3">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button className="flex w-full items-center gap-3 rounded-lg p-2 text-start transition-colors hover:bg-accent">
                <Avatar className="h-8 w-8">
                  {me.user.picture && <AvatarImage src={me.user.picture} alt={me.user.name} />}
                  <AvatarFallback>{initials(me.user.name, me.user.email)}</AvatarFallback>
                </Avatar>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{me.user.name}</p>
                  <p className="truncate text-[11px] text-muted-foreground">{me.user.email}</p>
                </div>
                <ChevronDown className="h-4 w-4 text-muted-foreground" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuLabel>
                <p className="text-sm font-medium text-foreground">{me.user.name}</p>
                <p className="truncate text-xs font-normal text-muted-foreground">{me.user.email}</p>
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => navigate("/account")}>
                <UserRound className="h-4 w-4" />
                {t("My account")}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => void logout()} className="text-destructive focus:text-destructive">
                <LogOut className="h-4 w-4" />
                {t("Sign out")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </aside>

      <div className="lg:ps-[270px]">
        <header className="sticky top-0 z-30 flex h-16 items-center justify-between gap-3 border-b bg-background/80 px-4 backdrop-blur sm:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <Button variant="ghost" size="icon" className="lg:hidden" onClick={() => setMobileOpen(true)}>
              <Menu className="h-5 w-5" />
            </Button>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{activeTenant?.name ?? "Platform"}</p>
              <p className="truncate text-xs text-muted-foreground">
                {me.isPlatformAdmin ? t("Platform administrator") : context?.roles?.join(", ") || t("Administrator")}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-1 sm:gap-2">
            <Badge variant="outline" className="hidden md:inline-flex">
              OIDC · {activeTenant?.slug ?? "platform"}
            </Badge>
            <PreferencesControls compact />
            <Button variant="outline" size="sm" asChild className="hidden sm:inline-flex">
              <a
                href={activeTenant ? `/t/${activeTenant.slug}/.well-known/openid-configuration` : "#"}
                target="_blank"
                rel="noreferrer"
              >
                {t("Discovery")}
              </a>
            </Button>
          </div>
        </header>

        <main className="mx-auto w-full max-w-7xl p-4 sm:p-6 lg:p-8">
          {contextQuery.isLoading && (
            <div className="space-y-4">
              <Skeleton className="h-9 w-64" />
              <Skeleton className="h-40 w-full" />
            </div>
          )}
          {contextQuery.error && (
            <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-6 text-sm text-destructive">
              {t("You do not have access to this organization. Switch tenants or contact a platform administrator.")}
            </div>
          )}
          {!contextQuery.isLoading && !contextQuery.error && <Outlet context={context} />}
        </main>
      </div>
    </div>
  );
}
