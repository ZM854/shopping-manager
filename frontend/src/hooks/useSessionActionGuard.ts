import { useEffect, useRef } from 'react';
import { sessionCoordinator, StaleSessionError } from '../services/session';

// Проверять страницу и сессию перед применением асинхронного результата к UI.
export function useSessionActionGuard() {
  const active = useRef(false);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);

  return () => {
    const context = sessionCoordinator.captureRequest();
    const check = () => {
      if (!active.current) throw new StaleSessionError();
      sessionCoordinator.assertRequest(context);
    };
    check();
    return check;
  };
}
