import {
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
  type PropsWithChildren,
} from 'react';
import { AuthContext } from './authContext';
import { sessionCoordinator } from '../../services/session';
import Button from '../../components/UI/button/Button/Button';
import cls from './AuthProvider.module.css';

export function AuthProvider({ children }: PropsWithChildren) {
  const session = useSyncExternalStore(
    sessionCoordinator.subscribe,
    sessionCoordinator.getSnapshot,
  );
  const [retrying, setRetrying] = useState(false);

  useEffect(() => {
    // The coordinator lives for the lifetime of the app, including StrictMode remounts.
    void sessionCoordinator.initialize().catch(() => {});
  }, []);

  const value = useMemo(
    () => ({
      ...session,
      isAuthenticated: session.status === 'authenticated',
      isLoading: session.status === 'checking',
      login: sessionCoordinator.login,
      register: sessionCoordinator.register,
      logout: sessionCoordinator.logout,
      refresh: sessionCoordinator.refresh,
    }),
    [session],
  );

  const retry = async () => {
    setRetrying(true);
    try {
      await sessionCoordinator.refresh();
    } catch {
      /* The coordinator exposes the failure to the UI. */
    } finally {
      setRetrying(false);
    }
  };

  return (
    <AuthContext.Provider value={value}>
      {session.status === 'error' && (
        <div className={cls.notice} role="alert">
          <p>{session.error}</p>
          <Button onClick={retry} disabled={retrying}>
            {retrying ? 'Проверяем…' : 'Повторить проверку сессии'}
          </Button>
        </div>
      )}
      {children}
    </AuthContext.Provider>
  );
}
