import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { AlertCircle, ArrowRight, Loader2, LockKeyhole, Mail, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { AuthShell } from "@/components/AuthShell";
import { TenantLogo } from "@/components/TenantLogo";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { api, setAdminTenantId, type AuthRequestInfo, type PublicTenant, type User } from "@/lib/api";
import { applyBrandColor } from "@/lib/utils";
import { useAuth } from "@/lib/auth";
import { useErrorText, useI18n } from "@/lib/i18n";

interface LoginResponse {
  mfaRequired: boolean;
  mfaSetupRequired?: boolean;
  user?: User;
  tenant?: PublicTenant;
  next?: string;
  requestId?: string;
}

export function LoginPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { refresh } = useAuth();
  const { t } = useI18n();
  const errorText = useErrorText();
  const requestId = params.get("request") || "";
  const reauth = params.get("reauth") === "1";
  const tenantParam = params.get("tenant") || localStorage.getItem("uc_tenant") || "";

  const [selectedTenant, setSelectedTenant] = useState(tenantParam);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [totp, setTotp] = useState("");
  const [step, setStep] = useState<"credentials" | "mfa">("credentials");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [continuing, setContinuing] = useState(false);

  const tenantsQuery = useQuery({
    queryKey: ["public-tenants"],
    queryFn: () => api.get<{ items: PublicTenant[] }>("/public/tenants"),
  });

  const requestQuery = useQuery({
    queryKey: ["auth-request", requestId],
    queryFn: () => api.get<AuthRequestInfo>(`/auth/request/${requestId}`),
    enabled: Boolean(requestId),
    retry: false,
  });

  const tenants = tenantsQuery.data?.items ?? [];
  const request = requestQuery.data;

  const tenant = useMemo<PublicTenant | null>(() => {
    if (request) return request.tenant;
    return tenants.find((t) => t.slug === selectedTenant) ?? tenants[0] ?? null;
  }, [request, tenants, selectedTenant]);

  useEffect(() => {
    if (!requestId && !selectedTenant && tenants.length > 0) {
      setSelectedTenant(tenants[0].slug);
    }
  }, [requestId, selectedTenant, tenants]);

  useEffect(() => {
    if (!tenant) return;
    localStorage.setItem("uc_tenant", tenant.slug);
    applyBrandColor(tenant.branding.primaryColor);
    document.title = `${tenant.name} · Sign in`;
  }, [tenant?.slug, tenant?.branding?.primaryColor]);

  useEffect(() => {
    if (request?.loginHint && !email) setEmail(request.loginHint);
  }, [request?.loginHint]);

  async function finishRequest() {
    if (!requestId) {
      navigate("/admin");
      return;
    }
    setContinuing(true);
    try {
      const out = await api.post<{ next: string; redirectUrl?: string }>(
        `/auth/request/${requestId}/complete`,
        {},
      );
      if (out.redirectUrl) {
        window.location.assign(out.redirectUrl);
        return;
      }
      if (out.next === "consent") navigate(`/consent?request=${requestId}`);
    } catch (err) {
      toast.error(errorText(err));
      setContinuing(false);
    }
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!tenant) return;
    setError("");
    setSubmitting(true);
    try {
      const res = await api.post<LoginResponse>("/auth/login", {
        tenant: tenant.slug,
        email,
        password,
        totp: step === "mfa" ? totp : undefined,
        requestId: requestId || undefined,
      });
      if (res.mfaRequired) {
        setStep("mfa");
        setTotp("");
        toast.info(t("Enter the 6-digit code from your authenticator app"));
        return;
      }
      localStorage.setItem("uc_tenant", tenant.slug);
      if (res.user?.platformAdmin && res.tenant?.id) {
        setAdminTenantId(res.tenant.id);
      }
      await refresh();
      if (res.mfaSetupRequired) {
        toast.warning(t("Your organization requires two-factor authentication. Please set it up in Account → Security."));
      }
      if (requestId) {
        await finishRequest();
      } else {
        window.location.assign("/admin");
      }
    } catch (err) {
      setError(errorText(err));
    } finally {
      setSubmitting(false);
    }
  }

  const supportEmail = tenant?.branding?.supportEmail ?? "";
  const helpParts = supportEmail
    ? t("Need help? Contact {email}", { email: "__EMAIL__" }).split("__EMAIL__")
    : [];
  const expired = request && (request.status === "expired" || request.status === "completed" || request.status === "denied");
  const canContinue = Boolean(request?.status === "authenticated" && request.user && !reauth);

  return (
    <AuthShell
      tenant={tenant}
      title={canContinue ? t("Continue to your app") : t("Welcome back")}
      subtitle={
        canContinue
          ? t("You are already signed in to {tenant}.", { tenant: tenant?.name ?? "" })
          : tenant
            ? t("Sign in to {tenant} to continue.", { tenant: tenant.name })
            : t("Choose an organization to continue.")
      }
      footer={
        tenant?.allowRegistration && !canContinue ? (
          <p className="text-center text-sm text-muted-foreground">
            {t("New to {tenant}?", { tenant: tenant.name })}{" "}
            <Link
              className="font-medium text-primary hover:underline"
              to={`/register?tenant=${encodeURIComponent(tenant.slug)}${requestId ? `&request=${requestId}` : ""}`}
            >
              {t("Create an account")}
            </Link>
          </p>
        ) : null
      }
    >
      {requestQuery.isLoading && (
        <div className="flex items-center justify-center rounded-xl border bg-card p-10">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      )}

      {requestQuery.error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{t("This sign-in request is invalid or has expired. Please return to the application and try again.")}</AlertDescription>
        </Alert>
      )}

      {expired && !requestQuery.error && (
        <Alert variant="warning">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{t("This authorization request can no longer be used. Please restart the sign-in from the application.")}</AlertDescription>
        </Alert>
      )}

      {canContinue && request?.user && (
        <div className="space-y-4">
          <div className="rounded-xl border bg-card p-5 shadow-soft">
            <div className="flex items-center gap-3">
              <TenantLogo tenant={tenant} className="h-11 w-11" />
              <div className="min-w-0">
                <p className="truncate font-medium">{request.user.name || request.user.email}</p>
                <p className="truncate text-sm text-muted-foreground">{request.user.email}</p>
              </div>
            </div>
            <div className="mt-4 flex items-center gap-2 rounded-lg bg-muted/60 px-3 py-2 text-xs text-muted-foreground">
              <ShieldCheck className="h-4 w-4 text-primary" />
              <span className="truncate">
                {request.client.name} is requesting access
                {request.scopes.length > 0 ? ` to ${request.scopes.map((s) => s.key).join(", ")}` : ""}
              </span>
            </div>
          </div>
          <Button className="w-full" size="lg" onClick={finishRequest} disabled={continuing}>
            {continuing ? <Loader2 className="h-4 w-4 animate-spin" /> : <ArrowRight className="h-4 w-4" />}
            {t("Continue as {name}", { name: request.user.name?.split(" ")[0] || request.user.email })}
          </Button>
          <Button variant="ghost" className="w-full" asChild>
            <Link to={`/login?request=${requestId}&reauth=1`}>{t("Use a different account")}</Link>
          </Button>
        </div>
      )}

      {!canContinue && !expired && !requestQuery.error && (
        <form className="space-y-5" onSubmit={onSubmit}>
          {!requestId && tenants.length > 1 && (
            <div className="space-y-2">
              <Label>{t("Organization")}</Label>
              <Select
                value={tenant?.slug ?? ""}
                onValueChange={(v) => {
                  setSelectedTenant(v);
                  localStorage.setItem("uc_tenant", v);
                }}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select organization" />
                </SelectTrigger>
                <SelectContent>
                  {tenants.map((t) => (
                    <SelectItem key={t.slug} value={t.slug}>
                      {t.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          {request && (
            <div className="flex items-center gap-3 rounded-xl border bg-card p-4">
              <TenantLogo tenant={{ name: request.client.name, branding: { ...tenant!.branding, logoUrl: request.client.logoUrl } }} className="h-10 w-10" />
              <div className="min-w-0">
                <p className="text-xs text-muted-foreground">{t("Signing in to")}</p>
                <p className="truncate text-sm font-medium">{request.client.name}</p>
              </div>
            </div>
          )}

          {step === "credentials" ? (
            <>
              <div className="space-y-2">
                <Label htmlFor="email">{t("Email")}</Label>
                <div className="relative">
                  <Mail className="absolute start-3 top-2.5 h-4 w-4 text-muted-foreground" />
                  <Input
                    id="email"
                    type="email"
                    autoComplete="username"
                    placeholder="you@company.com"
                    className="ps-9"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    required
                  />
                </div>
              </div>
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label htmlFor="password">{t("Password")}</Label>
                  <Link
                    to={`/forgot-password?tenant=${encodeURIComponent(tenant?.slug ?? "")}`}
                    className="text-xs font-medium text-primary hover:underline"
                  >
                    {t("Forgot password?")}
                  </Link>
                </div>
                <div className="relative">
                  <LockKeyhole className="absolute start-3 top-2.5 h-4 w-4 text-muted-foreground" />
                  <Input
                    id="password"
                    type="password"
                    autoComplete="current-password"
                    placeholder="••••••••••"
                    className="ps-9"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                  />
                </div>
              </div>
            </>
          ) : (
            <div className="space-y-2">
              <Label htmlFor="totp">{t("Verification code")}</Label>
              <Input
                id="totp"
                inputMode="numeric"
                autoComplete="one-time-code"
                placeholder="123456 or recovery code"
                autoFocus
                value={totp}
                onChange={(e) => setTotp(e.target.value)}
                required
              />
              <p className="text-xs text-muted-foreground">
                {t("Open your authenticator app and enter the 6-digit code. You can also use a recovery code.")}
              </p>
              <div className="flex items-center justify-between">
                <button
                  type="button"
                  className="text-xs font-medium text-primary hover:underline"
                  onClick={() => {
                    setStep("credentials");
                    setTotp("");
                  }}
                >
                  {t("Back to password")}
                </button>
                <Link
                  to={`/mfa-recovery?tenant=${encodeURIComponent(tenant?.slug ?? "")}`}
                  className="text-xs font-medium text-primary hover:underline"
                >
                  {t("Lost your authenticator?")}
                </Link>
              </div>
            </div>
          )}

          {error && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <Button type="submit" className="w-full" size="lg" disabled={submitting || !tenant}>
            {submitting && <Loader2 className="h-4 w-4 animate-spin" />}
            {step === "mfa" ? t("Verify and sign in") : t("Sign in")}
          </Button>

          {tenant && !tenant.allowPasswordLogin && (
            <p className="text-center text-xs text-muted-foreground">
              {t("Password sign-in is disabled for this organization. Contact your administrator.")}
            </p>
          )}
        </form>
      )}

      {tenant?.branding?.supportEmail && (
        <p className="mt-6 text-center text-xs text-muted-foreground">
          {helpParts[0]}
          <a className="text-primary hover:underline" href={`mailto:${supportEmail}`}>
            {supportEmail}
          </a>
          {helpParts[1]}
        </p>
      )}
    </AuthShell>
  );
}
