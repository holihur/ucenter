import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { copyText } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { toast } from "sonner";

export function CopyButton({ value, label }: { value: string; label?: string }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={async () => {
        await copyText(value);
        setCopied(true);
        toast.success(t("Copied"));
        setTimeout(() => setCopied(false), 1500);
      }}
    >
      {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
      {label ?? t("Copy")}
    </Button>
  );
}
