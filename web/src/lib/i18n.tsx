import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import en from "@/locales/en.json";
import zh from "@/locales/zh.json";
import es from "@/locales/es.json";
import hi from "@/locales/hi.json";
import ar from "@/locales/ar.json";
import pt from "@/locales/pt.json";
import bn from "@/locales/bn.json";
import ru from "@/locales/ru.json";
import ja from "@/locales/ja.json";
import fr from "@/locales/fr.json";
import { setActiveLocale } from "@/lib/utils";

export type Lang = "en" | "zh" | "es" | "hi" | "ar" | "pt" | "bn" | "ru" | "ja" | "fr";

export interface Language {
  code: Lang;
  name: string;
  nativeName: string;
  locale: string;
  dir: "ltr" | "rtl";
}

export const LANGUAGES: Language[] = [
  { code: "en", name: "English", nativeName: "English", locale: "en-US", dir: "ltr" },
  { code: "zh", name: "Chinese (Simplified)", nativeName: "简体中文", locale: "zh-CN", dir: "ltr" },
  { code: "es", name: "Spanish", nativeName: "Español", locale: "es-ES", dir: "ltr" },
  { code: "hi", name: "Hindi", nativeName: "हिन्दी", locale: "hi-IN", dir: "ltr" },
  { code: "ar", name: "Arabic", nativeName: "العربية", locale: "ar-SA", dir: "rtl" },
  { code: "pt", name: "Portuguese", nativeName: "Português", locale: "pt-BR", dir: "ltr" },
  { code: "bn", name: "Bengali", nativeName: "বাংলা", locale: "bn-BD", dir: "ltr" },
  { code: "ru", name: "Russian", nativeName: "Русский", locale: "ru-RU", dir: "ltr" },
  { code: "ja", name: "Japanese", nativeName: "日本語", locale: "ja-JP", dir: "ltr" },
  { code: "fr", name: "French", nativeName: "Français", locale: "fr-FR", dir: "ltr" },
];

const dictionaries: Record<Lang, Record<string, string>> = { en, zh, es, hi, ar, pt, bn, ru, ja, fr };
const STORAGE_KEY = "uc_lang";

function isLang(value: string | null): value is Lang {
  return Boolean(value && LANGUAGES.some((l) => l.code === value));
}

function detectLanguage(): Lang {
  if (typeof window === "undefined") return "en";
  const stored = localStorage.getItem(STORAGE_KEY);
  if (isLang(stored)) return stored;
  const candidates = [navigator.language, ...(navigator.languages ?? [])];
  for (const candidate of candidates) {
    const prefix = candidate.toLowerCase().split("-")[0];
    const match = LANGUAGES.find((l) => l.code === prefix);
    if (match) return match.code;
  }
  return "en";
}

function interpolate(template: string, vars?: Record<string, string | number>) {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (_, key: string) =>
    vars[key] !== undefined ? String(vars[key]) : `{${key}}`,
  );
}

interface I18nState {
  lang: Lang;
  language: Language;
  locale: string;
  dir: "ltr" | "rtl";
  setLang: (lang: Lang) => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
}

const I18nContext = createContext<I18nState | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detectLanguage);

  const language = useMemo(
    () => LANGUAGES.find((l) => l.code === lang) ?? LANGUAGES[0],
    [lang],
  );

  useEffect(() => {
    document.documentElement.lang = language.locale;
    document.documentElement.dir = language.dir;
    setActiveLocale(language.locale);
    localStorage.setItem(STORAGE_KEY, lang);
  }, [lang, language]);

  const setLang = useCallback((next: Lang) => setLangState(next), []);

  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      const dict = dictionaries[lang] ?? en;
      const template = dict[key] ?? (en as Record<string, string>)[key] ?? key;
      return interpolate(template, vars);
    },
    [lang],
  );

  const value = useMemo<I18nState>(
    () => ({ lang, language, locale: language.locale, dir: language.dir, setLang, t }),
    [lang, language, setLang, t],
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used inside I18nProvider");
  return ctx;
}

const errorKeys: Record<string, string> = {
  invalid_credentials: "Invalid email or password",
  mfa_required: "A verification code is required",
  invalid_mfa: "The verification code is invalid",
  account_disabled: "This account has been disabled",
  not_a_member: "This account does not have access to the requested organization",
  tenant_disabled: "This organization is disabled",
  rate_limited: "Too many sign-in attempts. Please wait a few minutes and try again.",
  email_in_use: "An account with this email already exists. Try signing in instead.",
  registration_disabled: "Self-registration is disabled for this organization",
  tenant_not_found: "Organization not found",
  password_login_disabled: "Password sign-in is disabled for this organization",
  invalid_password: "Your current password is incorrect",
  same_password: "The new password must be different from the current password",
  mfa_enabled: "Two-factor authentication is already enabled",
  mfa_disabled: "Two-factor authentication is not enabled",
  user_exists: "This user is already a member of the organization",
  role_exists: "A role with this key already exists",
  system_role: "System roles cannot be deleted",
  last_owner: "The organization must keep at least one owner",
  forbidden: "You do not have permission to perform this action",
  unauthenticated: "Your session has expired. Please sign in again.",
  not_found: "Something went wrong. Please try again.",
};

export function useErrorText() {
  const { t } = useI18n();
  return (err: unknown) => {
    if (err && typeof err === "object" && "code" in err) {
      const code = String((err as { code?: string }).code ?? "");
      const key = errorKeys[code];
      if (key) return t(key);
    }
    return err instanceof Error ? err.message : t("Something went wrong. Please try again.");
  };
}
