import { lazy, Suspense, type ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Loader2 } from "lucide-react";
import { Toaster } from "@/components/ui/sonner";
import { AdminLayout } from "@/components/layout/AdminLayout";
import { ConsentPage } from "@/pages/ConsentPage";
import { AcceptInvitePage, ForgotPasswordPage, MfaRecoveryConfirmPage, MfaRecoveryPage, ResetPasswordPage, VerifyEmailPage } from "@/pages/EmailPages";
import { LoginPage } from "@/pages/LoginPage";
import { NotFound } from "@/pages/NotFound";
import { RegisterPage } from "@/pages/RegisterPage";
import { useAuth } from "@/lib/auth";

const AccountPage = lazy(() => import("@/pages/AccountPage").then((m) => ({ default: m.AccountPage })));
const AdminOverview = lazy(() => import("@/pages/AdminOverview").then((m) => ({ default: m.AdminOverview })));
const AdminUsers = lazy(() => import("@/pages/AdminUsers").then((m) => ({ default: m.AdminUsers })));
const AdminRoles = lazy(() => import("@/pages/AdminRoles").then((m) => ({ default: m.AdminRoles })));
const AdminClients = lazy(() => import("@/pages/AdminClients").then((m) => ({ default: m.AdminClients })));
const AdminMcp = lazy(() => import("@/pages/AdminMcp").then((m) => ({ default: m.AdminMcp })));
const AdminMail = lazy(() => import("@/pages/AdminMail").then((m) => ({ default: m.AdminMail })));
const AdminIdentityProviders = lazy(() => import("@/pages/AdminIdentityProviders").then((m) => ({ default: m.AdminIdentityProviders })));
const AdminScim = lazy(() => import("@/pages/AdminScim").then((m) => ({ default: m.AdminScim })));
const AdminSessions = lazy(() => import("@/pages/AdminSessions").then((m) => ({ default: m.AdminSessions })));
const AdminAudit = lazy(() => import("@/pages/AdminAudit").then((m) => ({ default: m.AdminAudit })));
const AdminSettings = lazy(() => import("@/pages/AdminSettings").then((m) => ({ default: m.AdminSettings })));
const AdminTenants = lazy(() => import("@/pages/AdminTenants").then((m) => ({ default: m.AdminTenants })));

function Protected({ children }: { children: ReactNode }) {
  const { me, loading } = useAuth();
  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-primary" />
      </div>
    );
  }
  if (!me) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

function HomeRedirect() {
  const { me, loading } = useAuth();
  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-primary" />
      </div>
    );
  }
  if (!me) return <Navigate to="/login" replace />;
  const isAdmin = me.permissions.some((p) => p !== "platform:manage");
  return <Navigate to={isAdmin ? "/admin" : "/account"} replace />;
}

function PageLoader() {
  return (
    <div className="flex min-h-[50vh] items-center justify-center">
      <Loader2 className="h-6 w-6 animate-spin text-primary" />
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Suspense fallback={<PageLoader />}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="/consent" element={<ConsentPage />} />
        <Route path="/forgot-password" element={<ForgotPasswordPage />} />
        <Route path="/reset-password" element={<ResetPasswordPage />} />
        <Route path="/verify-email" element={<VerifyEmailPage />} />
        <Route path="/mfa-recovery" element={<MfaRecoveryPage />} />
        <Route path="/mfa-recovery/confirm" element={<MfaRecoveryConfirmPage />} />
        <Route path="/accept-invite" element={<AcceptInvitePage />} />
        <Route
          path="/account"
          element={
            <Protected>
              <AccountPage />
            </Protected>
          }
        />
        <Route
          path="/admin"
          element={
            <Protected>
              <AdminLayout />
            </Protected>
          }
        >
          <Route index element={<AdminOverview />} />
          <Route path="users" element={<AdminUsers />} />
          <Route path="roles" element={<AdminRoles />} />
          <Route path="clients" element={<AdminClients />} />
          <Route path="mcp" element={<AdminMcp />} />
          <Route path="mail" element={<AdminMail />} />
          <Route path="identity-providers" element={<AdminIdentityProviders />} />
          <Route path="scim" element={<AdminScim />} />
          <Route path="sessions" element={<AdminSessions />} />
          <Route path="audit" element={<AdminAudit />} />
          <Route path="settings" element={<AdminSettings />} />
          <Route path="tenants" element={<AdminTenants />} />
        </Route>
        <Route path="/" element={<HomeRedirect />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
      </Suspense>
      <Toaster richColors position="top-right" closeButton />
    </BrowserRouter>
  );
}
