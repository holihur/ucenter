import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, ScrollText, Search } from "lucide-react";
import { EmptyState } from "@/components/EmptyState";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api, type AuditLog, type Paginated } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { formatDate } from "@/lib/utils";

const actionOptions = [
  "auth.login.success",
  "auth.login.failed",
  "auth.logout",
  "oidc.authorize.approved",
  "oidc.authorize.denied",
  "oidc.token.issued",
  "user.created",
  "user.updated",
  "user.deleted",
  "user.password.reset",
  "user.mfa.enabled",
  "session.revoked",
  "role.created",
  "role.updated",
  "role.deleted",
  "client.created",
  "client.updated",
  "client.deleted",
  "client.secret.rotated",
  "tenant.settings.updated",
];

function tone(action: string) {
  if (action.includes("failed") || action.includes("denied") || action.includes("deleted")) return "destructive" as const;
  if (action.includes("created") || action.includes("registered")) return "success" as const;
  if (action.includes("updated") || action.includes("rotated") || action.includes("reset")) return "warning" as const;
  return "default" as const;
}

export function AdminAudit() {
  const { t } = useI18n();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [q, setQ] = useState("");
  const [action, setAction] = useState("all");
  const [selected, setSelected] = useState<AuditLog | null>(null);

  const query = useQuery({
    queryKey: ["audit", page, q, action],
    queryFn: () => {
      const sp = new URLSearchParams({ page: String(page), pageSize: "25" });
      if (q) sp.set("q", q);
      if (action !== "all") sp.set("action", action);
      return api.get<Paginated<AuditLog>>(`/admin/audit?${sp.toString()}`);
    },
  });

  useEffect(() => {
    const timer = setTimeout(() => {
      setQ(search.trim());
      setPage(1);
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

  const totalPages = Math.max(1, Math.ceil((query.data?.total ?? 0) / 25));

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Audit log")}
        description={t("Immutable record of authentication, authorization and administration events.")}
      />

      <div className="flex flex-col gap-3 sm:flex-row">
        <div className="relative flex-1">
          <Search className="absolute start-3 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            className="ps-9"
            placeholder={t("Search actor, action or target…")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <Select
          value={action}
          onValueChange={(v) => {
            setAction(v);
            setPage(1);
          }}
        >
          <SelectTrigger className="w-full sm:w-64">
            <SelectValue placeholder={t("Action")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("All actions")}</SelectItem>
            {actionOptions.map((a) => (
              <SelectItem key={a} value={a}>
                {a}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="rounded-xl border bg-card shadow-soft">
        {query.isLoading ? (
          <div className="space-y-3 p-4">
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        ) : query.data?.items.length ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("Time")}</TableHead>
                <TableHead>{t("Actor")}</TableHead>
                <TableHead>{t("Action")}</TableHead>
                <TableHead>{t("Target")}</TableHead>
                <TableHead>{t("IP address")}</TableHead>
                <TableHead className="w-20" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {query.data.items.map((event) => (
                <TableRow key={event.id}>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                    {formatDate(event.createdAt)}
                  </TableCell>
                  <TableCell>
                    <span className="text-sm">{event.actorEmail || "system"}</span>
                  </TableCell>
                  <TableCell>
                    <Badge variant={tone(event.action)}>{event.action}</Badge>
                  </TableCell>
                  <TableCell className="max-w-[220px] truncate text-xs text-muted-foreground">
                    {event.targetType}
                    {event.targetId ? ` · ${event.targetId}` : ""}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{event.ip || "—"}</TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => setSelected(event)}>
                      {t("Details")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <div className="p-6">
            <EmptyState
              icon={ScrollText}
              title={t("No audit events")}
              description={t("Activity will be recorded as users sign in and administrators make changes.")}
            />
          </div>
        )}
      </div>

      {query.data && query.data.total > 0 && (
        <div className="flex items-center justify-between text-sm text-muted-foreground">
          <span>{query.data.total} events</span>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
              <ChevronLeft className="h-4 w-4" />
              {t("Previous")}
            </Button>
            <span className="text-xs">
              Page {page} of {totalPages}
            </span>
            <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
              {t("Next")}
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}

      <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("Audit event")}</DialogTitle>
            <DialogDescription>{selected?.action}</DialogDescription>
          </DialogHeader>
          {selected && (
            <div className="space-y-3 text-sm">
              <dl className="grid grid-cols-3 gap-2">
                <dt className="text-muted-foreground">{t("Event ID")}</dt>
                <dd className="col-span-2 break-all font-mono text-xs">{selected.id}</dd>
                <dt className="text-muted-foreground">{t("Actor")}</dt>
                <dd className="col-span-2">{selected.actorEmail || "system"}</dd>
                <dt className="text-muted-foreground">{t("Target")}</dt>
                <dd className="col-span-2">
                  {selected.targetType}
                  {selected.targetId ? ` · ${selected.targetId}` : ""}
                </dd>
                <dt className="text-muted-foreground">IP</dt>
                <dd className="col-span-2 font-mono text-xs">{selected.ip || "—"}</dd>
                <dt className="text-muted-foreground">{t("User agent")}</dt>
                <dd className="col-span-2 break-all text-xs">{selected.userAgent || "—"}</dd>
                <dt className="text-muted-foreground">{t("Time")}</dt>
                <dd className="col-span-2">{formatDate(selected.createdAt)}</dd>
              </dl>
              {selected.metadata && Object.keys(selected.metadata).length > 0 && (
                <div>
                  <p className="mb-1 text-xs font-medium text-muted-foreground">{t("Metadata")}</p>
                  <pre className="max-h-64 overflow-auto rounded-lg border bg-muted/40 p-3 text-xs">
                    {JSON.stringify(selected.metadata, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
