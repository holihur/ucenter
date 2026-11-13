import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { AlertCircle, ArrowRight, Check, Loader2, ShieldCheck, X } from "lucide-react";
import { toast } from "sonner";
import { AuthShell } from "@/components/AuthShell";
import { TenantLogo } from "@/components/TenantLogo";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { api, ApiError, type AuthRequestInfo } from "@/lib/api";
import { applyBrandColor } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

export function ConsentPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const requestId = params.get("request") || "";
  const [phase, setPhase] = useState<"checking" | "consent" | "redirecting" | "error">("checking");
  const [error, setError] = useState("");
  const [consentInfo, setConsentInfo] = useState<{
    client: { clientId: string; name: string; logoUrl: string };
    scopes: { key: string; description: string }[];
  } | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const started = useRef(false);
  const { t } = useI18n();

  const query = useQuery({
    queryKey: ["auth-request", requestId],
    queryFn: () => api.get<AuthRequestInfo>(`/auth/request/${requestId}`),
    enabled: Boolean(requestId),
    retry: false,
  });

  const request = query.data;

  useEffect(() => {
    if (request) applyBrandColor(request.tenant.branding.primaryColor);
  }, [request?.tenant.branding.primaryColor]);

  useEffect(() => {
    if (!request || started.current) return;
    if (request.status === "expired" || request.status === "completed" || request.status === "denied") {
      setPhase("error");
      setError(t("This authorization request is no longer valid. Please return to the application and try again."));
      return;
    }
    // A cached pending record can coexist with an in-flight refetch after the
    // login page completes the request. Wait for the fresh status instead of
    // bouncing back to /login.
    if (request.status === "pending") {
      if (!query.isFetching) navigate(`/login?request=${requestId}`, { replace: true });
      return;
    }
    started.current = true;
    void complete();
  }, [request, requestId]);

  async function complete() {
    try {
      const out = await api.post<{
        next: string;
        redirectUrl?: string;
        client?: { clientId: string; name: string; logoUrl: string };
        scopes?: { key: string; description: string }[];
      }>(`/auth/request/${requestId}/complete`, {});
      if (out.redirectUrl) {
        setPhase("redirecting");
        window.location.assign(out.redirectUrl);
        return;
      }
      if (out.next === "consent" && out.client) {
        setConsentInfo({ client: out.client, scopes: out.scopes ?? [] });
        setPhase("consent");
        return;
      }
      setPhase("error");
      setError(t("This authorization request is no longer valid. Please return to the application and try again."));
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        navigate(`/login?request=${requestId}`, { replace: true });
        return;
      }
      setPhase("error");
      setError(err instanceof Error ? err.message : t("Something went wrong. Please try again."));
    }
  }

  async function decide(approve: boolean) {
    setSubmitting(true);
    try {
      const out = await api.post<{ redirectUrl: string }>(`/auth/request/${requestId}/consent`, { approve });
      setPhase("redirecting");
      window.location.assign(out.redirectUrl);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Something went wrong. Please try again."));
      setSubmitting(false);
    }
  }

  const tenant = request?.tenant;
  const clientName = consentInfo?.client.name || request?.client.name || "Application";
  const scopes = consentInfo?.scopes || request?.scopes || [];

  return (
    <AuthShell
      tenant={tenant}
      title={phase === "error" ? t("Authorization problem") : t("Authorize {client}", { client: clientName })}
      subtitle={
        phase === "error"
          ? undefined
          : request?.user
            ? t("Signed in as {email}", { email: request.user.email })
            : t("Review the access this application is requesting.")
      }
      footer={
        tenant ? (
          <p className="text-center text-xs text-muted-foreground">
            {t("You will return to {tenant} after making a decision.", { tenant: tenant.name })}
          </p>
        ) : null
      }
    >
      {(query.isLoading || phase === "checking" || phase === "redirecting") && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-xl border bg-card p-12">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
          <p className="text-sm text-muted-foreground">
            {phase === "redirecting" ? t("Redirecting you back…") : t("Checking your session…")}
          </p>
        </div>
      )}

      {phase === "error" && (
        <div className="space-y-4">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>{error || t("This request is invalid or has expired.")}</AlertDescription>
          </Alert>
          <Button variant="outline" className="w-full" asChild>
            <Link to="/login">{t("Back to sign in")}</Link>
          </Button>
        </div>
      )}

      {phase === "consent" && (
        <div className="space-y-5">
          <div className="rounded-xl border bg-card p-5 shadow-soft">
            <div className="flex items-center gap-3">
              <TenantLogo
                tenant={{
                  name: consentInfo?.client.name || request?.client.name || "App",
                  branding: {
                    ...(tenant?.branding ?? { logoUrl: "", primaryColor: "", backgroundColor: "", supportEmail: "" }),
                    logoUrl: consentInfo?.client.logoUrl || request?.client.logoUrl || "",
                  },
                }}
                className="h-12 w-12"
              />
              <div className="min-w-0">
                <p className="text-xs text-muted-foreground">{t("wants to access your account")}</p>
                <p className="truncate text-base font-semibold">{clientName}</p>
              </div>
            </div>
            <Separator className="my-4" />
            <p className="mb-3 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              {t("This will allow {client} to", { client: clientName })}
            </p>
            <ul className="space-y-2.5">
              {scopes.map((scope) => (
                <li key={scope.key} className="flex items-start gap-3 text-sm">
                  <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/10">
                    <Check className="h-3 w-3 text-emerald-600" />
                  </span>
                  <span>
                    <span className="font-medium">{t(scope.description)}</span>
                    <span className="ml-2 font-mono text-[11px] text-muted-foreground">{scope.key}</span>
                  </span>
                </li>
              ))}
            </ul>
            <div className="mt-4 flex items-start gap-2 rounded-lg bg-muted/60 px-3 py-2 text-xs text-muted-foreground">
              <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
              {t("You can revoke this access at any time from your account. Only share access with applications you trust.")}
            </div>
          </div>

          <div className="flex flex-col-reverse gap-2 sm:flex-row">
            <Button variant="outline" className="flex-1" onClick={() => decide(false)} disabled={submitting}>
              <X className="h-4 w-4" />
              {t("Deny")}
            </Button>
            <Button className="flex-1" onClick={() => decide(true)} disabled={submitting}>
              {submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <ArrowRight className="h-4 w-4" />}
              {t("Allow access")}
            </Button>
          </div>
        </div>
      )}
    </AuthShell>
  );
}
