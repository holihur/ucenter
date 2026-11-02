export type AuthLayout = "split" | "centered" | "poster";
export type AuthPattern = "mesh" | "grid" | "waves" | "blobs" | "terminal" | "seal" | "retail" | "none";

export interface AuthTemplate {
  id: string;
  name: string;
  industry: string;
  description: string;
  layout: AuthLayout;
  accent: string;
  panelClass: string;
  panelTextClass: string;
  panelMutedClass: string;
  shellClass: string;
  formClass: string;
  headlineClass: string;
  pattern: AuthPattern;
}

export const AUTH_TEMPLATES: AuthTemplate[] = [
  {
    id: "aurora",
    name: "Aurora",
    industry: "SaaS · Technology",
    description: "Modern split layout with a soft aurora mesh and product features.",
    layout: "split",
    accent: "#6366f1",
    panelClass: "brand-mesh",
    panelTextClass: "text-foreground",
    panelMutedClass: "text-muted-foreground",
    shellClass: "bg-background",
    formClass: "",
    headlineClass: "",
    pattern: "mesh",
  },
  {
    id: "minimal",
    name: "Minimal",
    industry: "Professional services",
    description: "Quiet, centered card with generous whitespace and fine borders.",
    layout: "centered",
    accent: "#0f172a",
    panelClass: "",
    panelTextClass: "text-foreground",
    panelMutedClass: "text-muted-foreground",
    shellClass: "bg-muted/30",
    formClass: "rounded-2xl border bg-card p-8 shadow-sm",
    headlineClass: "",
    pattern: "none",
  },
  {
    id: "glass",
    name: "Glass",
    industry: "Consumer · Lifestyle",
    description: "Full-bleed gradient with a translucent glass card.",
    layout: "poster",
    accent: "#8b5cf6",
    panelClass: "",
    panelTextClass: "text-white",
    panelMutedClass: "text-white/75",
    shellClass: "bg-gradient-to-br from-indigo-600 via-violet-600 to-pink-500",
    formClass: "rounded-3xl border border-white/30 bg-white/85 p-8 shadow-2xl backdrop-blur-2xl dark:border-white/10 dark:bg-slate-900/80",
    headlineClass: "",
    pattern: "blobs",
  },
  {
    id: "enterprise",
    name: "Enterprise",
    industry: "Large enterprise",
    description: "Navy trust panel, crisp grid and formal information hierarchy.",
    layout: "split",
    accent: "#2563eb",
    panelClass: "bg-[#0b1f3a]",
    panelTextClass: "text-white",
    panelMutedClass: "text-blue-100/75",
    shellClass: "bg-background",
    formClass: "",
    headlineClass: "tracking-tight",
    pattern: "grid",
  },
  {
    id: "fintech",
    name: "Fintech",
    industry: "Banking · Finance",
    description: "Deep emerald, market-grid texture and security-first tone.",
    layout: "split",
    accent: "#10b981",
    panelClass: "bg-[#04231c]",
    panelTextClass: "text-emerald-50",
    panelMutedClass: "text-emerald-100/70",
    shellClass: "bg-background",
    formClass: "",
    headlineClass: "",
    pattern: "grid",
  },
  {
    id: "healthcare",
    name: "Healthcare",
    industry: "Healthcare · Wellness",
    description: "Calm teal waves, soft surfaces and reassuring contrast.",
    layout: "split",
    accent: "#0d9488",
    panelClass: "bg-gradient-to-br from-teal-50 via-cyan-50 to-blue-100 dark:from-teal-950 dark:via-slate-950 dark:to-blue-950",
    panelTextClass: "text-foreground",
    panelMutedClass: "text-muted-foreground",
    shellClass: "bg-background",
    formClass: "",
    headlineClass: "",
    pattern: "waves",
  },
  {
    id: "education",
    name: "Education",
    industry: "Education · Learning",
    description: "Warm amber blobs and playful rounded geometry.",
    layout: "split",
    accent: "#ea580c",
    panelClass: "bg-gradient-to-br from-amber-100 via-orange-100 to-rose-100 dark:from-amber-950 dark:via-orange-950 dark:to-rose-950",
    panelTextClass: "text-foreground",
    panelMutedClass: "text-muted-foreground",
    shellClass: "bg-background",
    formClass: "",
    headlineClass: "",
    pattern: "blobs",
  },
  {
    id: "developer",
    name: "Developer",
    industry: "Developer tools",
    description: "Terminal-inspired dark panel with monospace accents and code lines.",
    layout: "split",
    accent: "#22c55e",
    panelClass: "bg-[#0b1020]",
    panelTextClass: "text-slate-100",
    panelMutedClass: "text-slate-400",
    shellClass: "bg-background",
    formClass: "",
    headlineClass: "font-mono",
    pattern: "terminal",
  },
  {
    id: "government",
    name: "Government",
    industry: "Public sector",
    description: "Formal navy and gold, serif headline and an official seal motif.",
    layout: "split",
    accent: "#d4af37",
    panelClass: "bg-[#14264f]",
    panelTextClass: "text-slate-50",
    panelMutedClass: "text-blue-100/70",
    shellClass: "bg-background",
    formClass: "",
    headlineClass: "font-serif",
    pattern: "seal",
  },
  {
    id: "retail",
    name: "Retail",
    industry: "Retail · E-commerce",
    description: "Bright peach gradient, bold card and lifestyle shapes.",
    layout: "centered",
    accent: "#f97316",
    panelClass: "",
    panelTextClass: "text-foreground",
    panelMutedClass: "text-muted-foreground",
    shellClass: "bg-gradient-to-br from-rose-100 via-orange-50 to-amber-100 dark:from-rose-950 dark:via-slate-950 dark:to-amber-950",
    formClass: "rounded-3xl border-0 bg-card p-8 shadow-2xl",
    headlineClass: "",
    pattern: "retail",
  },
];

export function getAuthTemplate(id?: string | null): AuthTemplate {
  return AUTH_TEMPLATES.find((template) => template.id === id) ?? AUTH_TEMPLATES[0];
}
