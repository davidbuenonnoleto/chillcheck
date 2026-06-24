import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { api, clearToken, getToken, setToken, type User } from "@/lib/api";

interface AuthState {
  user: User | null;
  orgName: string | null;
  loading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (body: { org_name: string; name: string; email: string; password: string }) => Promise<void>;
  adoptToken: (token: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [orgName, setOrgName] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!getToken()) {
      setLoading(false);
      return;
    }
    api
      .me()
      .then((res) => {
        setUser(res.user);
        setOrgName(res.organization.name);
      })
      .catch(() => clearToken())
      .finally(() => setLoading(false));
  }, []);

  async function login(email: string, password: string) {
    const res = await api.login({ email, password });
    setToken(res.token);
    setUser(res.user);
    const me = await api.me();
    setOrgName(me.organization.name);
  }

  async function register(body: { org_name: string; name: string; email: string; password: string }) {
    const res = await api.register(body);
    setToken(res.token);
    setUser(res.user);
    setOrgName(body.org_name);
  }

  // adoptToken sets the session from a token obtained outside login/register
  // (e.g. accepting an invite), then loads the user + org.
  async function adoptToken(token: string) {
    setToken(token);
    const me = await api.me();
    setUser(me.user);
    setOrgName(me.organization.name);
  }

  function logout() {
    clearToken();
    setUser(null);
    setOrgName(null);
  }

  return (
    <AuthContext.Provider value={{ user, orgName, loading, login, register, adoptToken, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
