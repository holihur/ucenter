import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { AlertCircle, CheckCircle2, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { AuthShell } from "@/components/AuthShell";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api, type PublicTenant } from "@/lib/api";
import { useErrorText, useI18n } from "@/lib/i18n";
import { applyBrandColor } from "@/lib/utils";

function usePageTenant() {
  const [params] = useSearchParams();
  const slug = params.get("tenant") || localStorage.getItem("uc_tenant") || "";
  const query = useQuery({
    queryKey: ["public-tenant", slug],
    queryFn: () => api.get<PublicTenant>(`/public/tenants/${slug}`),
    enabled: Boolean(slug),
    retry: false,
  });
  useEffect(() => {
    if (query.data?.branding?.primaryColor) applyBrandColor(query.data.branding.primaryColor);
  }, [query.data?.branding?.primaryColor]);
  return { tenant: query.data ?? null, slug, params };
}

function Success({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="space-y-4">
      <div className="flex items-start gap-3 rounded-xl border bg-card p-5">
        <CheckCircle2 className="mt-0.5 h-5 w-5 text-emerald-600" />
        <div>
          <p className="font-medium">{title}</p>
          <div className="mt-1 text-sm text-muted-foreground">{children}</div>
        </div>
      </div>
    </div>
  );
}

export function ForgotPasswordPage() {
  const { tenant } = usePageTenant();
  const { t } = useI18n();
  const errorText = useErrorText();
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!tenant) return;
    setBusy(true);
    try {
      await api.post("/auth/forgot-password", { tenant: tenant.slug, email });
      setSent(true);
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell
      tenant={tenant}
      title={t("Forgot password")}
      subtitle={t("We will email you a link to reset your password.")}
      footer={<p className="text-center text-sm"><Link className="text-primary hover:underline" to={`/login?tenant=${tenant?.slug ?? ""}`}>{t("Back to sign in")}</Link></p>}
    >
      {sent ? (
        <Success title={t("Check your email")}>
          {t("If an account exists for that address, a reset link has been sent.")}
        </Success>
      ) : (
        <form className="space-y-4" onSubmit={submit}>
          <div className="space-y-2">
            <Label htmlFor="forgot-email">{t("Email")}</Label>
            <Input id="forgot-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          </div>
          <Button type="submit" className="w-full" disabled={busy || !tenant}>
            {busy && <Loader2 className="h-4 w-4 animate-spin" />}
            {t("Send reset link")}
          </Button>
        </form>
      )}
    </AuthShell>
  );
}

export function ResetPasswordPage() {
  const { tenant, params } = usePageTenant();
  const { t } = useI18n();
  const errorText = useErrorText();
  const token = params.get("token") || "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (password !== confirm) {
      toast.error(t("Passwords do not match"));
      return;
    }
    setBusy(true);
    try {
      await api.post("/auth/reset-password", { token, password });
      setDone(true);
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell
      tenant={tenant}
      title={t("Reset password")}
      subtitle={t("Choose a new password for your account.")}
      footer={<p className="text-center text-sm"><Link className="text-primary hover:underline" to={`/login?tenant=${tenant?.slug ?? ""}`}>{t("Back to sign in")}</Link></p>}
    >
      {done ? (
        <Success title={t("Password updated")}>
          {t("You can now sign in with your new password.")}{" "}
          <Link className="text-primary hover:underline" to={`/login?tenant=${tenant?.slug ?? ""}`}>{t("Sign in")}</Link>
        </Success>
      ) : (
        <form className="space-y-4" onSubmit={submit}>
          <div className="space-y-2">
            <Label htmlFor="reset-password">{t("New password")}</Label>
            <Input id="reset-password" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </div>
          <div className="space-y-2">
            <Label htmlFor="reset-confirm">{t("Confirm password")}</Label>
            <Input id="reset-confirm" type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required />
          </div>
          <Button type="submit" className="w-full" disabled={busy || !token}>
            {busy && <Loader2 className="h-4 w-4 animate-spin" />}
            {t("Update password")}
          </Button>
        </form>
      )}
    </AuthShell>
  );
}

export function VerifyEmailPage() {
  const { tenant, params } = usePageTenant();
  const { t } = useI18n();
  const token = params.get("token") || "";
  const [state, setState] = useState<"working" | "ok" | "error">("working");
  const started = useRef(false);

  useEffect(() => {
    if (!token || started.current) return;
    started.current = true;
    api
      .post("/auth/verify-email", { token })
      .then(() => setState("ok"))
      .catch(() => setState("error"));
  }, [token]);

  return (
    <AuthShell tenant={tenant} title={t("Verify email")} subtitle={t("Confirming your email address.")}>
      {state === "working" && <p className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" />{t("Verifying…")}</p>}
      {state === "ok" && <Success title={t("Email verified")}>{t("Your email address has been verified.")}</Success>}
      {state === "error" && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{t("This verification link is invalid or has expired.")}</AlertDescription>
        </Alert>
      )}
    </AuthShell>
  );
}

export function MfaRecoveryPage() {
  const { tenant } = usePageTenant();
  const { t } = useI18n();
  const errorText = useErrorText();
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!tenant) return;
    setBusy(true);
    try {
      await api.post("/auth/mfa-recovery", { tenant: tenant.slug, email });
      setSent(true);
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell tenant={tenant} title={t("Recover two-factor authentication")} subtitle={t("We will email you a link to reset MFA for your account.")}>
      {sent ? (
        <Success title={t("Check your email")}>{t("If MFA is enabled for that account, a recovery link has been sent.")}</Success>
      ) : (
        <form className="space-y-4" onSubmit={submit}>
          <div className="space-y-2">
            <Label htmlFor="mfa-email">{t("Email")}</Label>
            <Input id="mfa-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          </div>
          <Button type="submit" className="w-full" disabled={busy || !tenant}>
            {busy && <Loader2 className="h-4 w-4 animate-spin" />}
            {t("Send recovery link")}
          </Button>
        </form>
      )}
    </AuthShell>
  );
}

export function MfaRecoveryConfirmPage() {
  const { tenant, params } = usePageTenant();
  const { t } = useI18n();
  const token = params.get("token") || "";
  const [state, setState] = useState<"working" | "ok" | "error">("working");
  const started = useRef(false);

  useEffect(() => {
    if (!token || started.current) return;
    started.current = true;
    api
      .post("/auth/mfa-recovery/confirm", { token })
      .then(() => setState("ok"))
      .catch(() => setState("error"));
  }, [token]);

  return (
    <AuthShell tenant={tenant} title={t("Reset two-factor authentication")} subtitle={t("Your sessions will be signed out.")}>
      {state === "working" && <p className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" />{t("Working…")}</p>}
      {state === "ok" && (
        <Success title={t("MFA disabled")}>
          {t("You can sign in with your password and set up MFA again.")}{" "}
          <Link className="text-primary hover:underline" to={`/login?tenant=${tenant?.slug ?? ""}`}>{t("Sign in")}</Link>
        </Success>
      )}
      {state === "error" && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{t("This recovery link is invalid or has expired.")}</AlertDescription>
        </Alert>
      )}
    </AuthShell>
  );
}

export function AcceptInvitePage() {
  const { tenant, params } = usePageTenant();
  const { t } = useI18n();
  const errorText = useErrorText();
  const token = params.get("token") || "";
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (password !== confirm) {
      toast.error(t("Passwords do not match"));
      return;
    }
    setBusy(true);
    try {
      await api.post("/auth/accept-invite", { token, name, password });
      setDone(true);
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell
      tenant={tenant}
      title={t("Accept invitation")}
      subtitle={tenant ? t("Set your password to join {tenant}.", { tenant: tenant.name }) : ""}
      footer={<p className="text-center text-sm"><Link className="text-primary hover:underline" to={`/login?tenant=${tenant?.slug ?? ""}`}>{t("Back to sign in")}</Link></p>}
    >
      {done ? (
        <Success title={t("Welcome aboard")}>
          <Link className="text-primary hover:underline" to={`/login?tenant=${tenant?.slug ?? ""}`}>{t("Sign in to continue")}</Link>
        </Success>
      ) : (
        <form className="space-y-4" onSubmit={submit}>
          <div className="space-y-2">
            <Label htmlFor="invite-name">{t("Full name")}</Label>
            <Input id="invite-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("Optional")} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="invite-password">{t("Password")}</Label>
            <Input id="invite-password" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </div>
          <div className="space-y-2">
            <Label htmlFor="invite-confirm">{t("Confirm password")}</Label>
            <Input id="invite-confirm" type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required />
          </div>
          <Button type="submit" className="w-full" disabled={busy || !token}>
            {busy && <Loader2 className="h-4 w-4 animate-spin" />}
            {t("Create account")}
          </Button>
        </form>
      )}
    </AuthShell>
  );
}
