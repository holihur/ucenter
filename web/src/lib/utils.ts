import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function initials(name?: string, email?: string) {
  const source = (name || email || "?").trim();
  const parts = source.split(/\s+/).filter(Boolean);
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
  return source.slice(0, 2).toUpperCase();
}

let activeLocale = "en-US";

export function setActiveLocale(locale: string) {
  activeLocale = locale;
}

export function formatDate(value?: string | null) {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString(activeLocale, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function timeAgo(value?: string | null) {
  if (!value) return "—";
  const timestamp = new Date(value).getTime();
  if (Number.isNaN(timestamp)) return "—";
  const diff = Date.now() - timestamp;
  const rtf = new Intl.RelativeTimeFormat(activeLocale, { numeric: "auto" });
  const seconds = Math.round(diff / 1000);
  if (seconds < 60) return rtf.format(-seconds, "second");
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return rtf.format(-minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (hours < 24) return rtf.format(-hours, "hour");
  const days = Math.round(hours / 24);
  if (days < 30) return rtf.format(-days, "day");
  const months = Math.round(days / 30);
  if (months < 12) return rtf.format(-months, "month");
  return rtf.format(-Math.round(months / 12), "year");
}

export function hexToHsl(hex: string): string | null {
  const match = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex.trim());
  if (!match) return null;
  const r = parseInt(match[1], 16) / 255;
  const g = parseInt(match[2], 16) / 255;
  const b = parseInt(match[3], 16) / 255;
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  let h = 0;
  let s = 0;
  const l = (max + min) / 2;
  if (max !== min) {
    const d = max - min;
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
    switch (max) {
      case r:
        h = (g - b) / d + (g < b ? 6 : 0);
        break;
      case g:
        h = (b - r) / d + 2;
        break;
      default:
        h = (r - g) / d + 4;
    }
    h /= 6;
  }
  return `${Math.round(h * 360)} ${Math.round(s * 100)}% ${Math.round(l * 100)}%`;
}

export function applyBrandColor(hex?: string) {
  if (!hex) return;
  const hsl = hexToHsl(hex);
  if (!hsl) return;
  document.documentElement.style.setProperty("--primary", hsl);
}

export function displayIP(ip?: string | null) {
  if (!ip) return "Unknown IP";
  if (ip === "::1" || ip === "127.0.0.1" || ip === "::ffff:127.0.0.1") return "Localhost";
  return ip;
}

export function shortId(value?: string | null, length = 12) {
  if (!value) return "";
  return value.length > length ? `${value.slice(0, length)}…` : value;
}

export function copyText(text: string) {
  if (navigator.clipboard?.writeText) return navigator.clipboard.writeText(text);
  const el = document.createElement("textarea");
  el.value = text;
  document.body.appendChild(el);
  el.select();
  document.execCommand("copy");
  document.body.removeChild(el);
  return Promise.resolve();
}
