import { useEffect, useMemo, useState, type PropsWithChildren } from 'react';
import { AuthContext } from './authContext.ts';
import type { User } from '../../models/user.ts';
import type { LoginRequest, RegistrationRequest } from '../../models/auth.ts';
import { authService } from '../../services/authService.ts';
import { setAccessToken } from '../../services/tokenStorage.ts';
import { logger } from '../../shared/logger.ts';

const TAG = 'Auth';

export function AuthProvider({ children }: PropsWithChildren) {
  const [user, setUser] = useState<User | null>(null);

  const [isLoading, setLoading] = useState(true);

  const login = async (data: LoginRequest): Promise<void> => {
    logger.info(TAG, 'login attempt', { email: data.email });
    try {
      const response = await authService.login(data);

      setAccessToken(response.accessToken);

      setUser(response.user);

      logger.debug(TAG, 'login successful', { email: data.email });
    } catch (error) {
      logger.error(TAG, 'failed to login', error);
    }
  };

  const register = async (data: RegistrationRequest): Promise<void> => {
    logger.info(TAG, 'registration attempt', { email: data.email });
    try {
      await authService.register(data);

      logger.debug(TAG, 'registration successful', { email: data.email });
    } catch (error) {
      logger.error(TAG, 'failed to register', error);
    }
  };

  const refresh = async (): Promise<void> => {
    logger.info(TAG, 'refresh attempt');
    try {
      const response = await authService.refresh();

      setAccessToken(response.accessToken);

      setUser(response.user);
      logger.debug(TAG, 'refresh successful');
    } catch (error) {
      logger.error(TAG, 'failed to refresh', error);
    }
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
    logger.info(TAG, 'logout attempt');
    try {
      await authService.logout();
    } catch (error) {
      logger.error(TAG, 'failed to logout', error);
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
