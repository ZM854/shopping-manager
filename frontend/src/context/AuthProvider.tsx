import { useEffect, useMemo, useState, type PropsWithChildren } from "react";
import { AuthContext } from "./authContext";
import type { User } from "../models/user";
import type { LoginRequest, RegistrationRequest } from "../models/auth";
import { authService } from "../services/authService";
import { setAccessToken } from "../services/tokenStorage";

export function AuthProvider({ children }: PropsWithChildren) {
  const [user, setUser] = useState<User | null>(null);

  const [isLoading, setLoading] = useState(true);

  const login = async (data: LoginRequest): Promise<void> => {
    const response = await authService.login(data);

    setAccessToken(response.accessToken);

    setUser(response.user);
  };

  const register = async (data: RegistrationRequest): Promise<void> => {
    await authService.register(data);
  };

  const refresh = async (): Promise<void> => {
    const response = await authService.refresh();

    setAccessToken(response.accessToken);

    setUser(response.user);
  };

  useEffect(() => {
    const initialize = async () => {
      try {
        await refresh();
      } catch {
        setAccessToken(null);
        setUser(null);
      } finally {
        setLoading(false);
      }
    };

    initialize();
  }, []);

  const logout = async (): Promise<void> => {
    try {
      await authService.logout();
    } finally {
      setAccessToken(null);
      setUser(null);
    }
  };

  const value = useMemo(
    () => ({
      user,
      isAuthenticated: user !== null,
      isLoading,
      login,
      register,
      logout,
      refresh,
    }),
    [user, isLoading],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
