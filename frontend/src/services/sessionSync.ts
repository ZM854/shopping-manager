export type SessionEvent = { id: string; kind: 'login' | 'logout' };

export interface SessionSync {
  exclusive<T>(action: () => Promise<T>): Promise<T>;
  current(): SessionEvent | null;
  announce(kind: SessionEvent['kind']): SessionEvent;
  listen(receive: (event: SessionEvent) => void): () => void;
}

const channelName = 'shopping-manager-auth';
const revisionKey = 'shopping-manager.session-revision';

function parseEvent(value: unknown): SessionEvent | null {
  if (
    !value ||
    typeof value !== 'object' ||
    !('id' in value) ||
    typeof value.id !== 'string' ||
    !('kind' in value) ||
    (value.kind !== 'login' && value.kind !== 'logout')
  )
    return null;
  return { id: value.id, kind: value.kind };
}

export function createSessionSync(): SessionSync {
  let queue = Promise.resolve();
  let channel: BroadcastChannel | null = null;
  let observed: SessionEvent | null = null;
  const read = (): SessionEvent | null => {
    try {
      return parseEvent(
        JSON.parse(localStorage.getItem(revisionKey) || 'null'),
      );
    } catch {
      return null;
    }
  };

  return {
    exclusive<T>(action: () => Promise<T>): Promise<T> {
      const next = queue.then(() =>
        typeof navigator !== 'undefined' && navigator.locks
          ? navigator.locks.request(channelName, action)
          : action(),
      );
      queue = next.then(
        () => undefined,
        () => undefined,
      );
      return next;
    },
    // BroadcastChannel может опередить видимость первой записи localStorage.
    // До появления записи используем уже принятое уведомление и тот же маркер.
    current: () => read() ?? observed,
    announce(kind) {
      const event = { id: crypto.randomUUID(), kind };
      observed = event;
      // Only a change marker is persistent; no tokens, user or other private data.
      try {
        localStorage.setItem(revisionKey, JSON.stringify(event));
      } catch {
        /* Storage can be disabled; BroadcastChannel still delivers events. */
      }
      channel?.postMessage(event);
      return event;
    },
    listen(receive) {
      const deliver = (data: unknown) => {
        const event = parseEvent(data);
        if (!event) return;
        const current = read();
        if (!current || current.id === event.id) {
          observed = event;
          receive(event);
        }
      };
      const onStorage = (event: StorageEvent) => {
        if (event.key !== revisionKey || !event.newValue) return;
        try {
          deliver(JSON.parse(event.newValue));
        } catch {
          /* Ignore malformed markers. */
        }
      };
      if (typeof window === 'undefined') return () => {};
      if (typeof window.BroadcastChannel === 'function') {
        channel = new window.BroadcastChannel(channelName);
        channel.onmessage = (event: MessageEvent<unknown>) =>
          deliver(event.data);
      }
      window.addEventListener('storage', onStorage);
      return () => {
        channel?.close();
        channel = null;
        window.removeEventListener('storage', onStorage);
      };
    },
  };
}
