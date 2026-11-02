import { useOutletContext } from "react-router-dom";
import { useAuth } from "@/lib/auth";
import type { MeResponse, PublicTenant } from "@/lib/api";

/**
 * Returns the tenant currently selected in the admin console. For platform
 * administrators this may differ from the tenant they signed in to, so pages
 * should prefer the admin context over the session tenant.
 */
export function useAdminTenant(): PublicTenant | null {
  const context = useOutletContext<MeResponse | undefined>();
  const { me } = useAuth();
  return context?.tenant ?? me?.tenant ?? null;
}
