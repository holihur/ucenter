import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { MonitorSmartphone, ShieldCheck, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { EmptyState } from "@/components/EmptyState";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useAuth } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { api, type SessionItem } from "@/lib/api";
import { displayIP, formatDate, timeAgo } from "@/lib/utils";

export function AdminSessions() {
  const { can } = useAuth();
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [revoking, setRevoking] = useState<SessionItem | null>(null);
  const [busy, setBusy] = useState(false);

  const query = useQuery({
    queryKey: ["sessions"],
    queryFn: () => api.get<{ items: SessionItem[] }>("/admin/sessions"),
  });

  async function onRevoke() {
    if (!revoking) return;
    setBusy(true);
    try {
      await api.del(`/admin/sessions/${revoking.id}`);
      toast.success(t("Session revoked"));
      setRevoking(null);
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unable to revoke session");
    } finally {
      setBusy(false);
    }
  }

  const sessions = query.data?.items ?? [];

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Active sessions")}
        description={t("Browser sessions currently signed in to this tenant.")}
      />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <MonitorSmartphone className="h-4 w-4 text-primary" />
            {sessions.length} {t("Active sessions").toLowerCase()}
          </CardTitle>
          <CardDescription>{t("Sessions expire automatically according to the tenant session policy.")}</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {query.isLoading ? (
            <div className="space-y-3 p-4">
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="h-14 w-full" />
              ))}
            </div>
          ) : sessions.length ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("User")}</TableHead>
                  <TableHead>{t("IP address")}</TableHead>
                  <TableHead>{t("Client")}</TableHead>
                  <TableHead>{t("MFA")}</TableHead>
                  <TableHead>{t("Signed in")}</TableHead>
                  <TableHead>{t("Expires")}</TableHead>
                  <TableHead className="w-12" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {sessions.map((session) => (
                  <TableRow key={session.id}>
                    <TableCell>
                      <p className="font-medium">{session.name || session.email}</p>
                      <p className="text-xs text-muted-foreground">{session.email}</p>
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{displayIP(session.ip)}</TableCell>
                    <TableCell>
                      <p className="max-w-[260px] truncate text-xs text-muted-foreground" title={session.userAgent}>
                        {session.userAgent || "Unknown client"}
                      </p>
                    </TableCell>
                    <TableCell>
                      {session.mfa ? (
                        <Badge variant="success">
                          <ShieldCheck className="mr-1 h-3 w-3" />
                          {t("Verified")}
                        </Badge>
                      ) : (
                        <Badge variant="muted">{t("Password")}</Badge>
                      )}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                      {timeAgo(session.createdAt)}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                      {formatDate(session.expiresAt)}
                    </TableCell>
                    <TableCell>
                      {can("sessions:write") && (
                        <Button variant="ghost" size="icon" onClick={() => setRevoking(session)}>
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <div className="p-6">
              <EmptyState
                icon={MonitorSmartphone}
                title={t("No active sessions")}
                description={t("Sessions will appear here when users sign in.")}
              />
            </div>
          )}
        </CardContent>
      </Card>

      <ConfirmDialog
        open={Boolean(revoking)}
        onOpenChange={(open) => !open && setRevoking(null)}
        title={t("Revoke session?")}
        description={`${revoking?.email ?? "This user"} will be signed out of this device immediately.`}
        confirmLabel={t("Revoke session")}
        destructive
        loading={busy}
        onConfirm={onRevoke}
      />
    </div>
  );
}
