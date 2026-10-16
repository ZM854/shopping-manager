import { useCallback, useEffect, useRef, useState } from 'react';

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Не удалось выполнить запрос';
}

export function useShoppingResource<T>(
  load: (signal: AbortSignal) => Promise<T>,
) {
  const [resource, setResource] = useState({
    source: load,
    data: null as T | null,
    loading: true,
    error: null as string | null,
  });
  const [mutation, setMutation] = useState({ source: load, pending: false });
  const active = useRef(false);
  const busy = useRef(false);
  const reading = useRef(true);
  const version = useRef(0);
  const controller = useRef<AbortController | null>(null);

  const fetchResource = useCallback(async () => {
    const request = ++version.current;
    busy.current = false;
    reading.current = true;
    controller.current?.abort();
    const next = new AbortController();
    controller.current = next;
    try {
      const data = await load(next.signal);
      if (active.current && request === version.current) {
        setResource({ source: load, data, loading: false, error: null });
      }
    } catch (error) {
      if (
        active.current &&
        request === version.current &&
        !next.signal.aborted
      ) {
        setResource({
          source: load,
          data: null,
          loading: false,
          error: errorMessage(error),
        });
      }
    } finally {
      if (active.current && request === version.current)
        reading.current = false;
    }
  }, [load]);

  useEffect(() => {
    active.current = true;
    void fetchResource();
    return () => {
      active.current = false;
      controller.current?.abort();
    };
  }, [fetchResource]);

  async function reload() {
    busy.current = false;
    setMutation({ source: load, pending: false });
    setResource({ source: load, data: null, loading: true, error: null });
    await fetchResource();
  }

  async function mutate<R>(
    action: () => Promise<R>,
    update: (data: T, result: R) => T,
  ) {
    if (busy.current || reading.current || !active.current)
      throw new Error('Действие недоступно');
    busy.current = true;
    setMutation({ source: load, pending: true });
    setResource((current) => ({ ...current, error: null }));
    const request = version.current;
    try {
      const result = await action();
      if (!active.current || request !== version.current)
        throw new Error('Страница списка уже закрыта');
      setResource((current) => ({
        ...current,
        data: current.data === null ? null : update(current.data, result),
      }));
      return result;
    } catch (error) {
      if (active.current && request === version.current) {
        setResource((current) => ({ ...current, error: errorMessage(error) }));
      }
      throw error;
    } finally {
      if (active.current && request === version.current) {
        busy.current = false;
        setMutation({ source: load, pending: false });
      }
    }
  }

  // При смене load старые данные скрываются уже на первом рендере.
  const current = resource.source === load;
  return {
    data: current ? resource.data : null,
    loading: current ? resource.loading : true,
    error: current ? resource.error : null,
    pending: mutation.source === load && mutation.pending,
    reload,
    mutate,
  };
}
