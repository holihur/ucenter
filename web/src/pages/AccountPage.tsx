import { useEffect, useState } from "react";
import { Link, Navigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import QRCode from "qrcode";
import { startRegistration } from "@simplewebauthn/browser";
import {
  ArrowLeft,
  Check,
  KeyRound,
  Loader2,
  LogOut,
  MonitorSmartphone,
  Save,
  ShieldCheck,
  ShieldOff,
  Smartphone,
  UserRound,
} from "lucide-react";
import { toast } from "sonner";
import { BrandMark } from "@/components/TenantLogo";
import { PreferencesControls } from "@/components/Preferences";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { CopyButton } from "@/components/CopyButton";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { useAuth } from "@/lib/auth";
import { useErrorText, useI18n } from "@/lib/i18n";
import { api, type IdentityProviderItem, type LinkedIdentity, type SessionItem } from "@/lib/api";
import { displayIP, formatDate, initials } from "@/lib/utils";

export function AccountPage() {
  const { me, loading, refresh, logout } = useAuth();
  const { t } = useI18n();
  const errorText = useErrorText();
  const queryClient = useQueryClient();

  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [picture, setPicture] = useState("");
  const [savingProfile, setSavingProfile] = useState(false);

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [savingPassword, setSavingPassword] = useState(false);

  const [setupOpen, setSetupOpen] = useState(false);
  const [setup, setSetup] = useState<{ secret: string; otpauthUrl: string; qr: string } | null>(null);
  const [setupCode, setSetupCode] = useState("");
  const [startingSetup, setStartingSetup] = useState(false);
  const [enabling, setEnabling] = useState(false);
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [passwordAction, setPasswordAction] = useState<"disable" | "regenerate" | null>(null);
  const [actionPassword, setActionPassword] = useState("");
  const [actionBusy, setActionBusy] = useState(false);
  const [revoking, setRevoking] = useState<SessionItem | null>(null);
  const [passkeyBusy, setPasskeyBusy] = useState(false);

  const sessionsQuery = useQuery({
    queryKey: ["my-sessions"],
    queryFn: () => api.get<{ items: SessionItem[] }>("/auth/sessions"),
    enabled: Boolean(me),
  });
  const identitiesQuery = useQuery({
    queryKey: ["my-identities"],
    queryFn: () => api.get<{ items: LinkedIdentity[] }>("/me/identities"),
    enabled: Boolean(me),
  });
  const webauthnQuery = useQuery({
    queryKey: ["webauthn-credentials"],
    queryFn: () => api.get<{ items: { id: string; name: string; createdAt: string; lastUsedAt?: string }[] }>("/me/webauthn/credentials"),
    enabled: Boolean(me),
  });
  const providersQuery = useQuery({
    queryKey: ["public-idps", me?.tenant.slug],
    queryFn: () => api.get<{ items: Pick<IdentityProviderItem, "type" | "name" | "mode">[] }>(`/public/tenants/${me!.tenant.slug}/identity-providers`),
    enabled: Boolean(me?.tenant.slug),
    retry: false,
  });

  useEffect(() => {
    if (!me) return;
    setName(me.user.name);
    setUsername(me.user.username);
    setPicture(me.user.picture);
  }, [me?.user.id]);

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Skeleton className="h-10 w-52" />
      </div>
    );
  }

  if (!me) return <Navigate to="/login" replace />;

  const isAdmin = me.permissions.some((p) => p !== "platform:manage");

  async function saveProfile(e: React.FormEvent) {
    e.preventDefault();
    setSavingProfile(true);
    try {
      await api.patch("/me", { name, username, picture });
      await refresh();
      toast.success(t("Profile updated"));
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setSavingProfile(false);
    }
  }

  async function changePassword(e: React.FormEvent) {
    e.preventDefault();
    if (newPassword !== confirmPassword) {
      toast.error(t("Passwords do not match"));
      return;
    }
    setSavingPassword(true);
    try {
      await api.post("/me/password", { currentPassword, newPassword });
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      await refresh();
      toast.success(t("Password updated. Other sessions have been signed out."));
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setSavingPassword(false);
    }
  }

  async function startMfaSetup() {
    setStartingSetup(true);
    try {
      const res = await api.post<{ secret: string; otpauthUrl: string }>("/me/mfa/setup");
      const qr = await QRCode.toDataURL(res.otpauthUrl, { margin: 1, width: 220 });
      setSetup({ ...res, qr });
      setSetupCode("");
      setSetupOpen(true);
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setStartingSetup(false);
    }
  }

  async function enableMfa(e: React.FormEvent) {
    e.preventDefault();
    setEnabling(true);
    try {
      const res = await api.post<{ recoveryCodes: string[] }>("/me/mfa/enable", { code: setupCode });
      setSetupOpen(false);
      setRecoveryCodes(res.recoveryCodes);
      await refresh();
      queryClient.invalidateQueries({ queryKey: ["my-sessions"] });
      toast.success(t("Two-factor authentication enabled"));
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setEnabling(false);
    }
  }

  async function runPasswordAction(e: React.FormEvent) {
    e.preventDefault();
    if (!passwordAction) return;
    setActionBusy(true);
    try {
      if (passwordAction === "disable") {
        await api.post("/me/mfa/disable", { password: actionPassword });
        toast.success(t("Two-factor authentication disabled"));
        await refresh();
      } else {
        const res = await api.post<{ recoveryCodes: string[] }>("/me/mfa/recovery-codes", {
          password: actionPassword,
        });
        setRecoveryCodes(res.recoveryCodes);
        toast.success(t("Recovery codes regenerated"));
      }
      setPasswordAction(null);
      setActionPassword("");
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setActionBusy(false);
    }
  }

  async function addPasskey() {
    setPasskeyBusy(true);
    try {
      const begin = await api.post<{ sessionId: string; options: { publicKey: any } }>(
        "/me/webauthn/register/begin",
      );
      const credential = await startRegistration({ optionsJSON: begin.options.publicKey });
      await api.post("/me/webauthn/register/finish", {
        sessionId: begin.sessionId,
        name: `Passkey ${new Date().toLocaleDateString()}`,
        credential,
      });
      toast.success(t("Passkey added"));
      queryClient.invalidateQueries({ queryKey: ["webauthn-credentials"] });
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setPasskeyBusy(false);
    }
  }

  async function deletePasskey(id: string) {
    try {
      await api.del(`/me/webauthn/credentials/${id}`);
      toast.success(t("Passkey removed"));
      queryClient.invalidateQueries({ queryKey: ["webauthn-credentials"] });
    } catch (err) {
      toast.error(errorText(err));
    }
  }

  async function revokeSession() {
    if (!revoking) return;
    try {
      await api.del(`/auth/sessions/${revoking.id}`);
      if (revoking.current) {
        window.location.assign("/login");
        return;
      }
      toast.success(t("Session revoked"));
      setRevoking(null);
      queryClient.invalidateQueries({ queryKey: ["my-sessions"] });
    } catch (err) {
      toast.error(errorText(err));
    }
  }

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b bg-card">
        <div className="mx-auto flex h-16 max-w-5xl items-center justify-between px-4 sm:px-6">
          <div className="flex items-center gap-3">
            <BrandMark className="h-8 w-8" />
            <div>
              <p className="text-sm font-semibold leading-tight">{t("My account")}</p>
              <p className="text-[11px] text-muted-foreground">{me.tenant.name}</p>
            </div>
          </div>
          <div className="flex items-center gap-1 sm:gap-2">
            <PreferencesControls compact />
            {isAdmin && (
              <Button variant="outline" size="sm" asChild>
                <Link to="/admin">
                  <ArrowLeft className="h-3.5 w-3.5" />
                  {t("Admin console")}
                </Link>
              </Button>
            )}
            <Button variant="ghost" size="sm" onClick={() => void logout()}>
              <LogOut className="h-3.5 w-3.5" />
              {t("Sign out")}
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-5xl space-y-6 px-4 py-8 sm:px-6">
        {me.mfaSetupRequired && !me.user.mfa.enabled && (
          <div className="flex items-start gap-3 rounded-xl border border-amber-500/40 bg-amber-500/5 p-4 text-sm">
            <ShieldCheck className="mt-0.5 h-4 w-4 text-amber-600" />
            <div>
              <p className="font-medium text-amber-900 dark:text-amber-300">{t("Two-factor authentication is required")}</p>
              <p className="text-xs text-amber-800/80 dark:text-amber-400/80">
                {t("Your organization requires MFA. Enable it below to keep your account secure.")}
              </p>
            </div>
          </div>
        )}

        <div className="grid gap-6 lg:grid-cols-[1.1fr_1fr]">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <UserRound className="h-4 w-4 text-primary" />
                {t("Profile")}
              </CardTitle>
              <CardDescription>Your identity across {me.tenant.name} applications.</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="space-y-4" onSubmit={saveProfile}>
                <div className="flex items-center gap-4">
                  <div className="flex h-16 w-16 items-center justify-center rounded-full bg-primary/10 text-xl font-semibold text-primary">
                    {picture ? (
                      <img src={picture} alt={name} className="h-16 w-16 rounded-full object-cover" />
                    ) : (
                      initials(name, me.user.email)
                    )}
                  </div>
                  <div className="min-w-0">
                    <p className="truncate font-medium">{me.user.email}</p>
                    <div className="mt-1 flex flex-wrap gap-1">
                      {me.roles.map((role) => (
                        <Badge key={role} variant="secondary">
                          {role}
                        </Badge>
                      ))}
                      {me.user.emailVerified && <Badge variant="success">{t("Verified")}</Badge>}
                    </div>
                  </div>
                </div>
                <Separator />
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="account-name">{t("Full name")}</Label>
                    <Input id="account-name" value={name} onChange={(e) => setName(e.target.value)} />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="account-username">{t("Username")}</Label>
                    <Input id="account-username" value={username} onChange={(e) => setUsername(e.target.value)} />
                  </div>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="account-picture">{t("Profile picture URL")}</Label>
                  <Input
                    id="account-picture"
                    value={picture}
                    onChange={(e) => setPicture(e.target.value)}
                    placeholder="https://…"
                  />
                </div>
                <div className="flex justify-end">
                  <Button type="submit" disabled={savingProfile}>
                    {savingProfile ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                    {t("Save profile")}
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <KeyRound className="h-4 w-4 text-primary" />
                {t("Password")}
              </CardTitle>
              <CardDescription>{t("Use a strong password you do not reuse elsewhere.")}</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="space-y-4" onSubmit={changePassword}>
                <div className="space-y-2">
                  <Label htmlFor="current-password">{t("Current password")}</Label>
                  <Input
                    id="current-password"
                    type="password"
                    autoComplete="current-password"
                    value={currentPassword}
                    onChange={(e) => setCurrentPassword(e.target.value)}
                    required
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="new-password">{t("New password")}</Label>
                  <Input
                    id="new-password"
                    type="password"
                    autoComplete="new-password"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    required
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="confirm-password">{t("Confirm new password")}</Label>
                  <Input
                    id="confirm-password"
                    type="password"
                    autoComplete="new-password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    required
                  />
                </div>
                <div className="flex justify-end">
                  <Button type="submit" variant="outline" disabled={savingPassword}>
                    {savingPassword && <Loader2 className="h-4 w-4 animate-spin" />}
                    {t("Update password")}
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader className="flex-row items-start justify-between space-y-0">
            <div>
              <CardTitle className="flex items-center gap-2 text-base">
                <Smartphone className="h-4 w-4 text-primary" />
                {t("Two-factor authentication")}
              </CardTitle>
              <CardDescription>{t("Protect your account with a time-based one-time password.")}</CardDescription>
            </div>
            {me.user.mfa.enabled ? <Badge variant="success">{t("Enabled")}</Badge> : <Badge variant="muted">Off</Badge>}
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">
              {me.user.mfa.enabled
                ? t("Your account is protected by an authenticator app. Recovery codes can be used if you lose access.")
                : t("Add an extra layer of security. You will be asked for a 6-digit code when signing in.")}
            </p>
            <div className="flex flex-wrap gap-2">
              {me.user.mfa.enabled ? (
                <>
                  <Button variant="outline" onClick={() => setPasswordAction("regenerate")}>
                    {t("Regenerate recovery codes")}
                  </Button>
                  <Button variant="outline" className="text-destructive" onClick={() => setPasswordAction("disable")}>
                    <ShieldOff className="h-4 w-4" />
                    {t("Disable MFA")}
                  </Button>
                </>
              ) : (
                <Button onClick={() => void startMfaSetup()} disabled={startingSetup}>
                  {startingSetup ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldCheck className="h-4 w-4" />}
                  {t("Set up authenticator")}
                </Button>
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex-row items-start justify-between space-y-0">
            <div>
              <CardTitle className="flex items-center gap-2 text-base">
                <KeyRound className="h-4 w-4 text-primary" />
                {t("Passkeys")}
              </CardTitle>
              <CardDescription>{t("Use Face ID, Touch ID, Windows Hello or a security key to sign in.")}</CardDescription>
            </div>
            <Button variant="outline" size="sm" onClick={() => void addPasskey()} disabled={passkeyBusy}>
              {passkeyBusy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <KeyRound className="h-3.5 w-3.5" />}
              {t("Add passkey")}
            </Button>
          </CardHeader>
          <CardContent className="space-y-2">
            {webauthnQuery.isLoading ? (
              <Skeleton className="h-12 w-full" />
            ) : webauthnQuery.data?.items.length ? (
              webauthnQuery.data.items.map((credential) => (
                <div key={credential.id} className="flex items-center justify-between gap-3 rounded-lg border p-3">
                  <div className="min-w-0">
                    <p className="text-sm font-medium">{credential.name}</p>
                    <p className="text-xs text-muted-foreground">
                      {t("Added {date}", { date: new Date(credential.createdAt).toLocaleDateString() })}
                      {credential.lastUsedAt ? ` · ${t("Last used {date}", { date: new Date(credential.lastUsedAt).toLocaleDateString() })}` : ""}
                    </p>
                  </div>
                  <Button variant="outline" size="sm" className="text-destructive" onClick={() => void deletePasskey(credential.id)}>
                    {t("Remove")}
                  </Button>
                </div>
              ))
            ) : (
              <p className="py-2 text-sm text-muted-foreground">{t("No passkeys registered yet.")}</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <KeyRound className="h-4 w-4 text-primary" />
              {t("Linked sign-in methods")}
            </CardTitle>
            <CardDescription>{t("Connect WeChat, Alipay, WeCom, social or a custom identity provider.")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {identitiesQuery.isLoading ? (
              <Skeleton className="h-12 w-full" />
            ) : identitiesQuery.data?.items.length ? (
              identitiesQuery.data.items.map((identity) => (
                <div key={identity.id} className="flex items-center justify-between gap-3 rounded-lg border p-3">
                  <div className="min-w-0">
                    <p className="text-sm font-medium">{identity.providerName}</p>
                    <p className="truncate text-xs text-muted-foreground">
                      {identity.email || identity.name || identity.subject}
                    </p>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    className="text-destructive"
                    onClick={async () => {
                      try {
                        await api.del(`/me/identities/${identity.id}`);
                        toast.success(t("Sign-in method unlinked"));
                        queryClient.invalidateQueries({ queryKey: ["my-identities"] });
                      } catch (err) {
                        toast.error(errorText(err));
                      }
                    }}
                  >
                    {t("Unlink")}
                  </Button>
                </div>
              ))
            ) : (
              <p className="py-2 text-sm text-muted-foreground">{t("No linked sign-in methods yet.")}</p>
            )}
            {(providersQuery.data?.items ?? []).filter(
              (provider) => !(identitiesQuery.data?.items ?? []).some((identity) => identity.provider === provider.type),
            ).length > 0 && (
              <div className="flex flex-wrap gap-2 border-t pt-3">
                {(providersQuery.data?.items ?? [])
                  .filter((provider) => !(identitiesQuery.data?.items ?? []).some((identity) => identity.provider === provider.type))
                  .map((provider) => (
                    <Button
                      key={provider.type}
                      variant="outline"
                      size="sm"
                      onClick={() => window.location.assign(`/t/${me.tenant.slug}/idp/${provider.type}/start?bind=1&return_to=/account`)}
                    >
                      {t("Link {provider}", { provider: provider.name })}
                    </Button>
                  ))}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <MonitorSmartphone className="h-4 w-4 text-primary" />
              {t("Your sessions")}
            </CardTitle>
            <CardDescription>{t("Devices currently signed in to your account.")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {sessionsQuery.isLoading ? (
              <Skeleton className="h-16 w-full" />
            ) : sessionsQuery.data?.items.length ? (
              sessionsQuery.data.items.map((session) => (
                <div
                  key={session.id}
                  className="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-sm font-medium">{displayIP(session.ip)}</p>
                      {session.current && <Badge variant="success">{t("This device")}</Badge>}
                      {session.mfa && <Badge variant="outline">{t("MFA")}</Badge>}
                    </div>
                    <p className="truncate text-xs text-muted-foreground" title={session.userAgent}>
                      {session.userAgent || "Unknown client"}
                    </p>
                    <p className="text-[11px] text-muted-foreground">
                      Signed in {formatDate(session.createdAt)} · expires {formatDate(session.expiresAt)}
                    </p>
                  </div>
                  <Button variant="outline" size="sm" onClick={() => setRevoking(session)}>
                    {t("Revoke")}
                  </Button>
                </div>
              ))
            ) : (
              <p className="py-6 text-center text-sm text-muted-foreground">{t("No active sessions")}.</p>
            )}
          </CardContent>
        </Card>

        <p className="pb-4 text-center text-xs text-muted-foreground">
          {t("Signed in as {email} · {tenant}", { email: me.user.email, tenant: me.tenant.name })}
        </p>
      </main>

      <Dialog open={setupOpen} onOpenChange={setSetupOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("Set up two-factor authentication")}</DialogTitle>
            <DialogDescription>
              {t("Scan the QR code with Google Authenticator, 1Password, Authy or any TOTP app.")}
            </DialogDescription>
          </DialogHeader>
          {setup && (
            <form className="space-y-4" onSubmit={enableMfa}>
              <div className="flex justify-center">
                <img src={setup.qr} alt="TOTP QR code" className="rounded-lg border bg-white p-2" />
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs text-muted-foreground">{t("Manual entry secret")}</Label>
                <div className="flex items-center gap-2 rounded-lg border bg-muted/40 p-2.5">
                  <code className="flex-1 break-all font-mono text-xs">{setup.secret}</code>
                  <CopyButton value={setup.secret} label="" />
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="mfa-code">{t("Verification code")}</Label>
                <Input
                  id="mfa-code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  placeholder="123456"
                  value={setupCode}
                  onChange={(e) => setSetupCode(e.target.value)}
                  required
                />
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setSetupOpen(false)}>
                  {t("Cancel")}
                </Button>
                <Button type="submit" disabled={enabling}>
                  {enabling ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}
                  Enable MFA
                </Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(recoveryCodes)} onOpenChange={(open) => !open && setRecoveryCodes(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("Save your recovery codes")}</DialogTitle>
            <DialogDescription>
              {t("Each code can be used once if you lose access to your authenticator. Store them somewhere safe.")}
            </DialogDescription>
          </DialogHeader>
          {recoveryCodes && (
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-2 rounded-lg border bg-muted/40 p-4">
                {recoveryCodes.map((code) => (
                  <code key={code} className="font-mono text-sm">
                    {code}
                  </code>
                ))}
              </div>
              <CopyButton value={recoveryCodes.join("\n")} label={t("Copy all codes")} />
            </div>
          )}
          <DialogFooter>
            <Button onClick={() => setRecoveryCodes(null)}>{t("I have saved them")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(passwordAction)} onOpenChange={(open) => !open && setPasswordAction(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {passwordAction === "disable" ? t("Disable two-factor authentication") : t("Regenerate recovery codes")}
            </DialogTitle>
            <DialogDescription>{t("Confirm your password to continue.")}</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={runPasswordAction}>
            <div className="space-y-2">
              <Label htmlFor="action-password">{t("Password")}</Label>
              <Input
                id="action-password"
                type="password"
                value={actionPassword}
                onChange={(e) => setActionPassword(e.target.value)}
                autoFocus
                required
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setPasswordAction(null)}>
                {t("Cancel")}
              </Button>
              <Button
                type="submit"
                variant={passwordAction === "disable" ? "destructive" : "default"}
                disabled={actionBusy}
              >
                {actionBusy && <Loader2 className="h-4 w-4 animate-spin" />}
                {passwordAction === "disable" ? t("Disable MFA") : t("Regenerate codes")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(revoking)}
        onOpenChange={(open) => !open && setRevoking(null)}
        title={t("Revoke this session?")}
        description={
          revoking?.current
            ? t("This is your current session. You will be signed out immediately.")
            : t("The device will be signed out and must authenticate again.")
        }
        confirmLabel={t("Revoke session")}
        destructive
        onConfirm={revokeSession}
      />
    </div>
  );
}
