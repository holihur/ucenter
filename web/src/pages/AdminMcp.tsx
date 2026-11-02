import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot, KeyRound, PlugZap, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { CopyButton } from "@/components/CopyButton";
import { EmptyState } from "@/components/EmptyState";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useAuth } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { api, type Permission } from "@/lib/api";
import { formatDate, timeAgo } from "@/lib/utils";

interface APITokenItem {
  id: string;
  name: string;
  prefix: string;
  scopes: string[];
  expiresAt?: string | null;
  lastUsedAt?: string | null;
  revoked: boolean;
  createdAt: string;
}

interface CreatedToken {
  raw: string;
  item: APITokenItem;
}

export function AdminMcp() {
  const { me, can } = useAuth();
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [created, setCreated] = useState<CreatedToken | null>(null);
  const [revoking, setRevoking] = useState<APITokenItem | null>(null);
  const [busy, setBusy] = useState(false);

  const tokensQuery = useQuery({
    queryKey: ["api-tokens"],
    queryFn: () => api.get<{ items: APITokenItem[] }>("/admin/api-tokens"),
  });

  const permissionsQuery = useQuery({
    queryKey: ["permissions"],
    queryFn: () => api.get<{ items: Permission[] }>("/admin/permissions"),
  });

  const tokens = tokensQuery.data?.items ?? [];
  const permissions = permissionsQuery.data?.items ?? [];

  async function revoke() {
    if (!revoking) return;
    setBusy(true);
    try {
      await api.del(`/admin/api-tokens/${revoking.id}`);
      toast.success(t("MCP token revoked"));
      setRevoking(null);
      queryClient.invalidateQueries({ queryKey: ["api-tokens"] });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unable to revoke token");
    } finally {
      setBusy(false);
    }
  }

  const endpoint = `${window.location.origin}/mcp`;
  const config = useMemo(
    () =>
      JSON.stringify(
        {
          mcpServers: {
            ucenter: {
              type: "http",
              url: endpoint,
              headers: { Authorization: "Bearer ucm_your_token" },
            },
          },
        },
        null,
        2,
      ),
    [endpoint],
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("MCP access")}
        description={t("Connect AI clients to UCenter through the Model Context Protocol.")}
        actions={
          can("users:read") || can("tenant:read") ? (
            <Button onClick={() => setCreateOpen(true)}>
              <KeyRound className="h-4 w-4" />
              {t("Create token")}
            </Button>
          ) : null
        }
      />

      <div className="grid gap-4 lg:grid-cols-[1.2fr_1fr]">
        <div className="rounded-xl border bg-card p-5 shadow-soft">
          <div className="flex items-center gap-2">
            <Bot className="h-4 w-4 text-primary" />
            <h3 className="text-sm font-semibold">{t("MCP endpoint")}</h3>
          </div>
          <div className="mt-3 flex items-center gap-2 rounded-lg border bg-muted/30 p-2.5">
            <code className="flex-1 break-all font-mono text-xs">{endpoint}</code>
            <CopyButton value={endpoint} label="" />
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            {t("Streamable HTTP transport. Authenticate with an API token created below.")}
          </p>
        </div>
        <div className="rounded-xl border bg-card p-5 shadow-soft">
          <div className="flex items-center gap-2">
            <PlugZap className="h-4 w-4 text-primary" />
            <h3 className="text-sm font-semibold">{t("Client configuration")}</h3>
          </div>
          <div className="relative mt-3">
            <pre className="max-h-48 overflow-auto rounded-lg border bg-muted/40 p-3 pr-12 font-mono text-[11px] leading-relaxed">
              {config}
            </pre>
            <div className="absolute end-2 top-2">
              <CopyButton value={config} label="" />
            </div>
          </div>
        </div>
      </div>

      <div className="rounded-xl border bg-card shadow-soft">
        {tokensQuery.isLoading ? (
          <div className="space-y-3 p-4">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-14 w-full" />
            ))}
          </div>
        ) : tokens.length ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("Name")}</TableHead>
                <TableHead>{t("Token")}</TableHead>
                <TableHead>{t("Scopes")}</TableHead>
                <TableHead>{t("Created")}</TableHead>
                <TableHead>{t("Expires")}</TableHead>
                <TableHead>{t("Last used")}</TableHead>
                <TableHead>{t("Status")}</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((token) => (
                <TableRow key={token.id}>
                  <TableCell className="font-medium">{token.name}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{token.prefix}…</TableCell>
                  <TableCell>
                    <Badge variant="outline">{token.scopes.length}</Badge>
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                    {formatDate(token.createdAt)}
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                    {token.expiresAt ? formatDate(token.expiresAt) : t("Never")}
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                    {token.lastUsedAt ? timeAgo(token.lastUsedAt) : t("Never")}
                  </TableCell>
                  <TableCell>
                    <Badge variant={token.revoked ? "destructive" : "success"}>
                      {token.revoked ? t("Disabled") : t("Active")}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {!token.revoked && (
                      <Button variant="ghost" size="icon" onClick={() => setRevoking(token)}>
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
              icon={Bot}
              title={t("No MCP tokens yet")}
              description={t("Create an API token to let an MCP client act as you within this tenant.")}
              action={<Button onClick={() => setCreateOpen(true)}>{t("Create token")}</Button>}
            />
          </div>
        )}
      </div>

      <CreateTokenDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        permissions={permissions}
        defaultScopes={me?.permissions ?? []}
        onCreated={(value) => {
          queryClient.invalidateQueries({ queryKey: ["api-tokens"] });
          setCreateOpen(false);
          setCreated(value);
        }}
      />

      <Dialog open={Boolean(created)} onOpenChange={(open) => !open && setCreated(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("Copy your MCP token")}</DialogTitle>
            <DialogDescription>
              {t("This token is shown only once. Store it in your MCP client configuration.")}
            </DialogDescription>
          </DialogHeader>
          {created && (
            <div className="space-y-3">
              <div className="flex items-center gap-2 rounded-lg border bg-muted/40 p-3">
                <code className="flex-1 break-all font-mono text-xs">{created.raw}</code>
                <CopyButton value={created.raw} label="" />
              </div>
              <div className="rounded-lg border bg-muted/30 p-3">
                <p className="mb-2 text-xs font-medium text-muted-foreground">{t("Scopes")}</p>
                <div className="flex flex-wrap gap-1">
                  {created.item.scopes.map((scope) => (
                    <span key={scope} className="rounded bg-background px-1.5 py-0.5 font-mono text-[10px]">
                      {scope}
                    </span>
                  ))}
                </div>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button onClick={() => setCreated(null)}>{t("Done")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(revoking)}
        onOpenChange={(open) => !open && setRevoking(null)}
        title={t("Revoke MCP token?")}
        description={t("{name} will stop working immediately for every MCP client.", { name: revoking?.name ?? "" })}
        confirmLabel={t("Revoke token")}
        destructive
        loading={busy}
        onConfirm={revoke}
      />
    </div>
  );
}

function CreateTokenDialog({
  open,
  onOpenChange,
  permissions,
  defaultScopes,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  permissions: Permission[];
  defaultScopes: string[];
  onCreated: (value: CreatedToken) => void;
}) {
  const { t } = useI18n();
  const [name, setName] = useState("");
  const [days, setDays] = useState(90);
  const [scopes, setScopes] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);

  const groups = useMemo(() => {
    const map = new Map<string, Permission[]>();
    for (const permission of permissions) {
      if (!map.has(permission.group)) map.set(permission.group, []);
      map.get(permission.group)!.push(permission);
    }
    return Array.from(map.entries());
  }, [permissions]);

  useEffect(() => {
    if (!open) return;
    setName("");
    setDays(90);
    setScopes(defaultScopes);
  }, [open, defaultScopes]);

  function toggle(scope: string) {
    setScopes((prev) => (prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      const res = await api.post<{ token: string; item: APITokenItem }>("/admin/api-tokens", {
        name,
        scopes,
        expiresInDays: days,
      });
      onCreated({ raw: res.token, item: res.item });
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unable to create token");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("Create MCP token")}</DialogTitle>
          <DialogDescription>
            {t("The token inherits the selected scopes and is bound to this tenant.")}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="mcp-token-name">{t("Name")}</Label>
              <Input
                id="mcp-token-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Claude Desktop"
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="mcp-token-days">{t("Expires in (days)")}</Label>
              <Input
                id="mcp-token-days"
                type="number"
                min={1}
                max={365}
                value={days}
                onChange={(e) => setDays(Number(e.target.value))}
              />
            </div>
          </div>
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>{t("Scopes")}</Label>
              <span className="text-xs text-muted-foreground">{scopes.length} {t("selected")}</span>
            </div>
            <div className="max-h-72 space-y-4 overflow-y-auto rounded-lg border p-4">
              {groups.map(([group, items]) => (
                <div key={t(group)}>
                  <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{t(group)}</p>
                  <div className="grid gap-1.5 sm:grid-cols-2">
                    {items.map((permission) => (
                      <label key={permission.key} className="flex cursor-pointer items-start gap-2 rounded-md p-1.5 hover:bg-muted/60">
                        <Checkbox
                          checked={scopes.includes(permission.key)}
                          onCheckedChange={() => toggle(permission.key)}
                          className="mt-0.5"
                        />
                        <span>
                          <span className="block text-sm font-medium">{t(permission.label)}</span>
                          <span className="block font-mono text-[10px] text-muted-foreground">{permission.key}</span>
                        </span>
                      </label>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              {t("Cancel")}
            </Button>
            <Button type="submit" disabled={saving || scopes.length === 0}>
              {saving ? t("Working…") : t("Create token")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
