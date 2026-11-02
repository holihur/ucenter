import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Mail, MailOpen } from "lucide-react";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useI18n } from "@/lib/i18n";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/utils";

interface MailItem {
  _id: string;
  tenant_id: string;
  to: string;
  subject: string;
  text: string;
  html: string;
  kind: string;
  created_at: string;
}

export function AdminMail() {
  const { t } = useI18n();
  const [selected, setSelected] = useState<MailItem | null>(null);
  const query = useQuery({
    queryKey: ["mail-outbox"],
    queryFn: () => api.get<{ items: MailItem[]; driver: string }>("/admin/mail-outbox"),
  });
  const messages = query.data?.items ?? [];

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Mail outbox")}
        description={t("Local mock mailer output. In production, configure the SMTP driver.")}
        actions={<Badge variant="outline">{t("Driver")}: {query.data?.driver ?? "mock"}</Badge>}
      />
      <div className="rounded-xl border bg-card shadow-soft">
        {query.isLoading ? (
          <div className="space-y-3 p-4">{Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-12 w-full" />)}</div>
        ) : messages.length ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("To")}</TableHead>
                <TableHead>{t("Subject")}</TableHead>
                <TableHead>{t("Kind")}</TableHead>
                <TableHead>{t("Created")}</TableHead>
                <TableHead className="w-20" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {messages.map((message) => (
                <TableRow key={message._id}>
                  <TableCell className="text-sm">{message.to}</TableCell>
                  <TableCell className="max-w-[360px] truncate text-sm">{message.subject}</TableCell>
                  <TableCell><Badge variant="outline">{message.kind}</Badge></TableCell>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatDate(message.created_at)}</TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => setSelected(message)}>
                      <MailOpen className="h-4 w-4" />
                      {t("Open")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <div className="p-6">
            <EmptyState icon={Mail} title={t("No mail yet")} description={t("Invites and password reset emails will appear here when using the mock mailer.")} />
          </div>
        )}
      </div>
      <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{selected?.subject}</DialogTitle>
            <DialogDescription>{selected?.to} · {selected?.kind}</DialogDescription>
          </DialogHeader>
          {selected && (
            <div className="space-y-2">
              <pre className="max-h-[50vh] overflow-auto whitespace-pre-wrap rounded-lg border bg-muted/40 p-3 text-xs">{selected.text}</pre>
              <p className="text-[11px] text-muted-foreground">{t("Mock messages are stored in MongoDB and are not sent over SMTP.")}</p>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
