import { useEffect, useRef, useState } from 'react';
import { ApiError } from '../services/api';
import { errorMessage } from './useResource';
import { InputError } from '../shared/shoppingValidation';

export function useSubmission() {
  const [pending, setPending] = useState(false);
  const [retry, setRetry] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const active = useRef(false);
  const busy = useRef(false);
  const key = useRef<string | null>(null);

  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);

  async function submit(action: (key: string) => Promise<void>) {
    if (busy.current) return;
    busy.current = true;
    setPending(true);
    setError(null);
    setFieldErrors({});
    try {
      if (typeof globalThis.crypto?.randomUUID !== 'function')
        throw new InputError(
          'Для безопасной отправки откройте приложение по HTTPS или localhost.',
        );
      key.current ??= crypto.randomUUID();
      await action(key.current);
      if (active.current) {
        key.current = null;
        setRetry(false);
      }
    } catch (error) {
      if (active.current) {
        setError(errorMessage(error));
        if (
          error instanceof InputError ||
          (error instanceof ApiError &&
            error.status < 500 &&
            error.code !== 'origin_forbidden')
        ) {
          key.current = null;
          setRetry(false);
          setFieldErrors(error.fieldErrors ?? {});
        } else {
          setRetry(true);
        }
      }
    } finally {
      busy.current = false;
      if (active.current) setPending(false);
    }
  }

  function reset() {
    if (busy.current) return;
    key.current = null;
    setRetry(false);
    setError(null);
    setFieldErrors({});
  }

  return { pending, retry, error, fieldErrors, submit, reset };
}
