import { useEffect, useRef, useState } from 'react';
import { ApiError } from '../services/api';
import { errorMessage } from './useShoppingResource';
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
    key.current ??= crypto.randomUUID();
    setPending(true);
    setError(null);
    setFieldErrors({});
    try {
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
          (error instanceof ApiError && error.status < 500)
        ) {
          key.current = null;
          setRetry(false);
          setFieldErrors(
            error instanceof ApiError ? (error.fieldErrors ?? {}) : {},
          );
        } else {
          setRetry(true);
        }
      }
    } finally {
      busy.current = false;
      if (active.current) setPending(false);
    }
  }

  return { pending, retry, error, fieldErrors, submit };
}
