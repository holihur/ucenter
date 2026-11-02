import { Check, Languages, Monitor, Moon, Sun } from "lucide-react";
import { useI18n, LANGUAGES } from "@/lib/i18n";
import { useTheme, type Theme } from "@/lib/theme";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

const themeOptions: { value: Theme; icon: typeof Sun; labelKey: string }[] = [
  { value: "light", icon: Sun, labelKey: "Light" },
  { value: "dark", icon: Moon, labelKey: "Dark" },
  { value: "system", icon: Monitor, labelKey: "System theme" },
];

export function ThemeMenu({ compact = false }: { compact?: boolean }) {
  const { theme, setTheme, resolvedTheme } = useTheme();
  const { t } = useI18n();
  const Icon = theme === "dark" || (theme === "system" && resolvedTheme === "dark") ? Moon : Sun;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size={compact ? "icon" : "sm"}
          className={cn("text-muted-foreground hover:text-foreground", !compact && "gap-2")}
          aria-label={t("Theme")}
          data-testid="theme-menu"
        >
          <Icon className="h-4 w-4" />
          {!compact && <span className="hidden sm:inline">{t(themeOptions.find((o) => o.value === theme)?.labelKey ?? "System theme")}</span>}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-44">
        <DropdownMenuLabel>{t("Theme")}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {themeOptions.map((option) => (
          <DropdownMenuItem key={option.value} onSelect={() => setTheme(option.value)} className="gap-2">
            <option.icon className="h-4 w-4" />
            <span className="flex-1">{t(option.labelKey)}</span>
            {theme === option.value && <Check className="h-3.5 w-3.5 text-primary" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function LanguageMenu({ compact = false }: { compact?: boolean }) {
  const { lang, language, setLang, t } = useI18n();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size={compact ? "icon" : "sm"}
          className={cn("text-muted-foreground hover:text-foreground", !compact && "gap-2")}
          aria-label={t("Language")}
          data-testid="language-menu"
        >
          <Languages className="h-4 w-4" />
          {!compact && <span className="hidden sm:inline">{language.nativeName}</span>}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel>{t("Language")}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <div className="max-h-80 overflow-y-auto">
          {LANGUAGES.map((item) => (
            <DropdownMenuItem key={item.code} onSelect={() => setLang(item.code)} className="gap-2">
              <span className="flex-1">
                <span className="block text-sm">{item.nativeName}</span>
                <span className="block text-[11px] text-muted-foreground">{item.name}</span>
              </span>
              {lang === item.code && <Check className="h-3.5 w-3.5 text-primary" />}
            </DropdownMenuItem>
          ))}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function PreferencesControls({ className, compact = false }: { className?: string; compact?: boolean }) {
  return (
    <div className={cn("flex items-center gap-1", className)}>
      <LanguageMenu compact={compact} />
      <ThemeMenu compact={compact} />
    </div>
  );
}
