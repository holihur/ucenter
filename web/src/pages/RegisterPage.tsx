import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { AlertCircle, ArrowRight, Loader2, LockKeyhole, Mail, UserRound } from "lucide-react";
import { toast } from "sonner";
import { AuthShell } from "@/components/AuthShell";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api, type AuthRequestInfo, type PublicTenant, type User } from "@/lib/api";
import { applyBrandColor } from "@/lib/utils";
import { useAuth } from "@/lib/auth";
import { useErrorText, useI18n } from "@/lib/i18n";

interface RegisterResponse {
  user: User;
  tenant: PublicTenant;
  next?: string;
  requestId?: string;
}

export function RegisterPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { refresh } = useAuth();
  const { t } = useI18n();
  const errorText = useErrorText();
  const requestId = params.get("request") || "";
  const tenantSlug = params.get("tenant") || localStorage.getItem("uc_tenant") || "";

  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

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

  const tenant = useMemo<PublicTenant | null>(() => {
    if (requestQuery.data) return requestQuery.data.tenant;
    return tenantsQuery.data?.items.find((t) => t.slug === tenantSlug) ?? tenantsQuery.data?.items[0] ?? null;
  }, [requestQuery.data, tenantsQuery.data, tenantSlug]);

  useEffect(() => {
    if (!tenant) return;
    applyBrandColor(tenant.branding.primaryColor);
    document.title = `${tenant.name} · Create account`;
  }, [tenant?.slug, tenant?.branding?.primaryColor]);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!tenant) return;
    setError("");
    if (password !== confirm) {
      setError(t("Passwords do not match"));
      return;
    }
    setSubmitting(true);
    try {
      const res = await api.post<RegisterResponse>("/auth/register", {
        tenant: tenant.slug,
        email,
        password,
        name,
        requestId: requestId || undefined,
      });
      localStorage.setItem("uc_tenant", tenant.slug);
      await refresh();
      if (requestId) {
        const out = await api.post<{ next: string; redirectUrl?: string }>(
          `/auth/request/${requestId}/complete`,
          {},
        );
        if (out.redirectUrl) {
          window.location.assign(out.redirectUrl);
          return;
        }
        if (out.next === "consent") {
          navigate(`/consent?request=${requestId}`);
          return;
        }
      }
      toast.success(t("Welcome to {tenant}", { tenant: res.tenant.name }));
      window.location.assign("/admin");
    } catch (err) {
      setError(errorText(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthShell
      tenant={tenant}
      title={t("Create your account")}
      subtitle={tenant ? `Join ${tenant.name} with a new identity.` : "Choose an organization to join."}
      footer={
        <p className="text-center text-sm text-muted-foreground">
          {t("Already have an account?")}{" "}
          <Link
            className="font-medium text-primary hover:underline"
            to={`/login?tenant=${encodeURIComponent(tenant?.slug ?? "")}${requestId ? `&request=${requestId}` : ""}`}
          >
            {t("Sign in")}
          </Link>
        </p>
      }
    >
      {tenant && !tenant.allowRegistration ? (
        <Alert variant="warning">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>
            {t("Self-registration is disabled for {tenant}. Ask an administrator to invite you.", { tenant: tenant.name })}
          </AlertDescription>
        </Alert>
      ) : (
        <form className="space-y-4" onSubmit={onSubmit}>
          <div className="space-y-2">
            <Label htmlFor="name">{t("Full name")}</Label>
            <div className="relative">
              <UserRound className="absolute start-3 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                id="name"
                className="ps-9"
                placeholder="Ada Lovelace"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="email">{t("Work email")}</Label>
            <div className="relative">
              <Mail className="absolute start-3 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                id="email"
                type="email"
                autoComplete="username"
                className="ps-9"
                placeholder="you@company.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="password">{t("Password")}</Label>
              <div className="relative">
                <LockKeyhole className="absolute start-3 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  id="password"
                  type="password"
                  autoComplete="new-password"
                  className="ps-9"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirm">{t("Confirm password")}</Label>
              <Input
                id="confirm"
                type="password"
                autoComplete="new-password"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                required
              />
            </div>
          </div>
          <p className="text-xs text-muted-foreground">
            {t("Use at least 10 characters. Your password is hashed with Argon2id and never stored in plain text.")}
          </p>

          {error && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <Button type="submit" size="lg" className="w-full" disabled={submitting || !tenant}>
            {submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <ArrowRight className="h-4 w-4" />}
            {t("Create account")}
          </Button>
        </form>
      )}
    </AuthShell>
  );
}
