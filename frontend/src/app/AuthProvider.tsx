import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, ApiRequestError } from "../api/client";
import { onUnauthorized } from "../api/requestPolicy";
import type { Role } from "../api/types";

type AuthStatus = "loading" | "authenticated" | "unauthenticated";

interface AuthState {
  status: AuthStatus;
  role: Role | null;
  username: string | null;
  loginError: string | null;
  loggingIn: boolean;
  login: (password: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [role, setRole] = useState<Role | null>(null);
  const [username, setUsername] = useState<string | null>(null);
  const [loginError, setLoginError] = useState<string | null>(null);
  const [loggingIn, setLoggingIn] = useState(false);

  useEffect(() => {
    const unsubscribe = onUnauthorized(() => {
      setRole(null);
      setUsername(null);
      setStatus("unauthenticated");
      queryClient.clear();
    });
    let cancelled = false;
    api.auth
      .session()
      .then((s) => {
        if (cancelled) return;
        setRole(s.role);
        setUsername(s.username);
        setStatus("authenticated");
      })
      .catch(() => {
        if (cancelled) return;
        setStatus("unauthenticated");
      });
    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, [queryClient]);

  async function login(password: string) {
    setLoginError(null);
    setLoggingIn(true);
    try {
      const { role: r } = await api.auth.login(password);
      const s = await api.auth.session();
      setRole(r);
      setUsername(s.username);
      setStatus("authenticated");
    } catch (err) {
      const message = err instanceof ApiRequestError ? err.message : "Sign-in failed. Try again.";
      setLoginError(message);
      throw err;
    } finally {
      setLoggingIn(false);
    }
  }

  async function logout() {
    await api.auth.logout().catch(() => undefined);
    setRole(null);
    setUsername(null);
    setStatus("unauthenticated");
    queryClient.clear();
  }

  return (
    <AuthContext.Provider value={{ status, role, username, loginError, loggingIn, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}

export function useIsAdmin(): boolean {
  return useAuth().role === "admin";
}
