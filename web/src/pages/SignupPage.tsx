import { useState } from "react";
import { Link } from "react-router-dom";
import { AlertCircle, ArrowRight, Building2, Loader2, LockKeyhole, Mail, UserRound } from "lucide-react";
import { AuthShell } from "@/components/AuthShell";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export function SignupPage() {
  const { t } = useI18n();
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [ownerName, setOwnerName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.post("/public/signup", { slug, name, ownerName, ownerEmail: email, password });
      window.location.assign("/admin");
    } catch (err) {
      setError(err instanceof Error ? err.message : t("Unable to create organization"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell
      title={t("Create your organization")}
      subtitle={t("Start a new isolated tenant with you as the owner.")}
      footer={<p className="text-center text-sm text-muted-foreground">{t("Already have an account?")}{" "}<Link className="font-medium text-primary hover:underline" to="/login">{t("Sign in")}</Link></p>}
    >
      <form className="space-y-4" onSubmit={submit}>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="signup-name">{t("Organization name")}</Label>
            <div className="relative">
              <Building2 className="absolute start-3 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input id="signup-name" className="ps-9" value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="signup-slug">{t("Slug")}</Label>
            <Input
              id="signup-slug"
              value={slug}
              onChange={(e) => setSlug(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ""))}
              placeholder="acme"
              required
            />
            <p className="text-[11px] text-muted-foreground">{t("Used in your OIDC issuer URL.")}</p>
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="signup-owner">{t("Your name")}</Label>
            <div className="relative">
              <UserRound className="absolute start-3 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input id="signup-owner" className="ps-9" value={ownerName} onChange={(e) => setOwnerName(e.target.value)} required />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="signup-email">{t("Work email")}</Label>
            <div className="relative">
              <Mail className="absolute start-3 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input id="signup-email" type="email" className="ps-9" value={email} onChange={(e) => setEmail(e.target.value)} required />
            </div>
          </div>
        </div>
        <div className="space-y-2">
          <Label htmlFor="signup-password">{t("Password")}</Label>
          <div className="relative">
            <LockKeyhole className="absolute start-3 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input id="signup-password" type="password" autoComplete="new-password" className="ps-9" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </div>
          <p className="text-xs text-muted-foreground">{t("Use at least 10 characters.")}</p>
        </div>
        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <Button type="submit" size="lg" className="w-full" disabled={busy}>
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <ArrowRight className="h-4 w-4" />}
          {t("Create organization")}
        </Button>
      </form>
    </AuthShell>
  );
}
