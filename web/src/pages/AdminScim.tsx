import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Loader2, RefreshCw, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { CopyButton } from "@/components/CopyButton";
import { PageHeader } from "@/components/PageHeader";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { useAuth } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { api } from "@/lib/api";

interface SCIMSettings {
  enabled: boolean;
  tokenPrefix: string;
  endpoint: string;
  usersEndpoint: string;
  groupsEndpoint: string;
}

export function AdminScim() {
  const { can } = useAuth();
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [token, setToken] = useState("");
  const [generating, setGenerating] = useState(false);
  const [toggling, setToggling] = useState(false);
  const query = useQuery({
    queryKey: ["scim-settings"],
    queryFn: () => api.get<SCIMSettings>("/admin/scim"),
  });
  const settings = query.data;

  async function toggle(enabled: boolean) {
    setToggling(true);
    try {
      await api.patch("/admin/scim", { enabled });
      toast.success(t("SCIM settings updated"));
      queryClient.invalidateQueries({ queryKey: ["scim-settings"] });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to update SCIM settings"));
    } finally {
      setToggling(false);
    }
  }

  async function generateToken() {
    setGenerating(true);
    try {
      const res = await api.post<{ token: string; prefix: string }>("/admin/scim/token");
      setToken(res.token);
      toast.success(t("SCIM token generated"));
      queryClient.invalidateQueries({ queryKey: ["scim-settings"] });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to generate SCIM token"));
    } finally {
      setGenerating(false);
    }
  }

  const endpoint = settings?.endpoint ?? `${window.location.origin}/t/${"acme"}/scim/v2`;
  return (
    <div className="space-y-6">
      <PageHeader
        title={t("SCIM provisioning")}
        description={t("Automate user and group provisioning with SCIM 2.0.")}
        actions={
          can("tenant:write") ? (
            <Button onClick={generateToken} disabled={generating}>
              {generating ? <Loader2 className="h-4 w-4 animate-spin" /> : <KeyRound className="h-4 w-4" />}
              {t("Generate SCIM token")}
            </Button>
          ) : null
        }
      />

      {query.isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : (
        <div className="grid gap-4 lg:grid-cols-[1.2fr_1fr]">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <RefreshCw className="h-4 w-4 text-primary" />
                {t("SCIM 2.0 endpoint")}
              </CardTitle>
              <CardDescription>{t("Use this base URL in your identity provider's SCIM connector.")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center gap-2 rounded-lg border bg-muted/30 p-2.5">
                <code className="flex-1 break-all font-mono text-xs">{endpoint}</code>
                <CopyButton value={endpoint} label="" />
              </div>
              <div className="grid gap-2 text-xs text-muted-foreground">
                <div className="flex justify-between gap-3"><span>{t("Users")}</span><code className="truncate font-mono">{settings?.usersEndpoint}</code></div>
                <div className="flex justify-between gap-3"><span>{t("Groups")}</span><code className="truncate font-mono">{settings?.groupsEndpoint}</code></div>
              </div>
              <div className="flex items-center justify-between rounded-lg border p-4">
                <div>
                  <p className="text-sm font-medium">{t("Enable SCIM provisioning")}</p>
                  <p className="text-xs text-muted-foreground">{t("Disabling immediately rejects all SCIM tokens.")}</p>
                </div>
                <Switch
                  checked={Boolean(settings?.enabled)}
                  onCheckedChange={(v) => void toggle(v)}
                  disabled={toggling || !can("tenant:write")}
                />
              </div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <ShieldCheck className="h-4 w-4 text-primary" />
                {settings?.tokenPrefix ? (
                  <span>{t("Active token")}: <code className="font-mono">{settings.tokenPrefix}…</code></span>
                ) : (
                  <span>{t("No SCIM token generated yet.")}</span>
                )}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("Connector settings")}</CardTitle>
              <CardDescription>{t("Configure these values in your IdP's SCIM integration.")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3 text-sm text-muted-foreground">
              <p><span className="font-medium text-foreground">{t("Authentication")}:</span> {t("Bearer token")}</p>
              <p><span className="font-medium text-foreground">{t("Content-Type")}:</span> <code className="font-mono">application/scim+json</code></p>
              <p><span className="font-medium text-foreground">{t("Filter")}:</span> {t("The eq operator is supported for userName, externalId and active.")}</p>
              <p><span className="font-medium text-foreground">{t("Groups")}:</span> {t("SCIM groups map to tenant roles, so group membership controls RBAC.")}</p>
              <p className="text-xs">{t("Rotate the token if it leaks. Existing tokens are invalidated immediately.")}</p>
            </CardContent>
          </Card>
        </div>
      )}

      <Dialog open={Boolean(token)} onOpenChange={(open) => !open && setToken("")}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("Copy your SCIM token")}</DialogTitle>
            <DialogDescription>{t("This token is shown only once. Store it in your identity provider.")}</DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 rounded-lg border bg-muted/40 p-3">
            <code className="flex-1 break-all font-mono text-xs">{token}</code>
            <CopyButton value={token} label="" />
          </div>
          <DialogFooter>
            <Button onClick={() => setToken("")}>{t("Done")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
