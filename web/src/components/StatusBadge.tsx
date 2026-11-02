import { Badge } from "@/components/ui/badge";
import { useI18n } from "@/lib/i18n";

const map: Record<string, { label: string; variant: "success" | "warning" | "destructive" | "muted" | "default" }> = {
  active: { label: "Active", variant: "success" },
  enabled: { label: "Enabled", variant: "success" },
  disabled: { label: "Disabled", variant: "destructive" },
  invited: { label: "Invited", variant: "warning" },
  pending: { label: "Pending", variant: "warning" },
};

export function StatusBadge({ status }: { status: string }) {
  const { t } = useI18n();
  const item = map[status] ?? { label: status, variant: "muted" as const };
  return <Badge variant={item.variant}>{t(item.label)}</Badge>;
}
