import { createContext, useCallback, useContext, useMemo, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type MeResponse } from "@/lib/api";

interface AuthState {
  me: MeResponse | null;
  loading: boolean;
  error: unknown;
  can: (permission: string) => boolean;
  refresh: () => Promise<unknown>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

const publicAuthPath =
  typeof window !== "undefined" &&
  ["/login", "/register", "/consent"].some(
    (path) => window.location.pathname === path || window.location.pathname.startsWith(`${path}/`),
  );

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const query = useQuery<MeResponse>({
    queryKey: ["me"],
    queryFn: () => api.get<MeResponse>("/auth/me"),
    retry: false,
    staleTime: 15_000,
    refetchOnWindowFocus: false,
    enabled: !publicAuthPath,
  });

  const logout = useCallback(async () => {
    try {
      await api.post("/auth/logout");
    } catch {
      // ignore network errors; clear local state regardless
    }
    queryClient.clear();
    window.location.assign("/login");
  }, [queryClient]);

  const can = useCallback(
    (permission: string) => Boolean(query.data?.permissions?.includes(permission)),
    [query.data],
  );

  const value = useMemo<AuthState>(
    () => ({
      me: query.data ?? null,
      loading: query.isLoading,
      error: query.error,
      can,
      refresh: query.refetch,
      logout,
    }),
    [query.data, query.isLoading, query.error, query.refetch, can, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}
