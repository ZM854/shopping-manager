import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { waitFor } from '@testing-library/react';
import type { AuthResponse } from '../models/auth';
import { ApiError } from './http';
import { SessionCoordinator, StaleSessionError } from './session';
import type { SessionEvent, SessionSync } from './sessionSync';
import { getAccessToken } from './tokenStorage';

const result: AuthResponse = {
  user: {
    id: '1',
    name: 'Первый',
    email: 'one@example.test',
    isEmailVerified: true,
  },
  accessToken: 'first',
};
const credentials = { email: 'one@example.test', password: 'password123' };
const transport = {
  login: vi.fn(),
  refresh: vi.fn(),
  logout: vi.fn(),
  register: vi.fn(),
};
let coordinator: SessionCoordinator;
let deliver: (event: SessionEvent) => void;
let current: SessionEvent | null;
let sync: SessionSync;
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
beforeEach(() => {
  vi.resetAllMocks();
  current = null;
  let queue = Promise.resolve();
  sync = {
    exclusive<T>(run: () => Promise<T>) {
      const next = queue.then(run);
      queue = next.then(
        () => undefined,
        () => undefined,
      );
      return next;
    },
    current: () => current,
    announce: vi.fn((kind) => (current = { id: crypto.randomUUID(), kind })),
    listen: (receive) => {
      deliver = receive;
      return () => {};
    },
  };
  coordinator = new SessionCoordinator(transport, sync);
  coordinator.connect();
});
afterEach(() => coordinator.dispose());
function notify(kind: SessionEvent['kind']) {
  current = { id: crypto.randomUUID(), kind };
  deliver(current);
}

describe('Поколения сессии', () => {
  it('ошибка Origin не означает отзыв сессии и сохраняет данные для повтора', async () => {
    transport.refresh.mockResolvedValueOnce(result);
    await coordinator.initialize();
    transport.refresh.mockRejectedValueOnce(
      new ApiError(403, 'Origin не разрешён', 'origin_forbidden'),
    );
    await expect(coordinator.refresh()).rejects.toMatchObject({
      code: 'origin_forbidden',
    });
    expect(coordinator.getSnapshot().status).toBe('error');
    expect(coordinator.getSnapshot().user).toEqual(result.user);
    expect(getAccessToken()).toBe(result.accessToken);
  });
  it('вход другой вкладки очищает приватное состояние, старый refresh не отменяет новый', async () => {
    transport.refresh.mockResolvedValueOnce(result);
    await coordinator.initialize();
    const scope = coordinator.getSnapshot().scope;
    const old = deferred<AuthResponse>();
    transport.refresh.mockReturnValueOnce(old.promise);
    const refreshing = coordinator.refresh();
    const rejected =
      expect(refreshing).rejects.toBeInstanceOf(StaleSessionError);
    await waitFor(() => expect(transport.refresh).toHaveBeenCalledTimes(2));
    const next = {
      ...result,
      user: { ...result.user, id: '2', name: 'Второй' },
      accessToken: 'second',
    };
    transport.refresh.mockResolvedValueOnce(next);
    notify('login');
    expect(coordinator.getSnapshot().user).toBeNull();
    expect(getAccessToken()).toBeNull();
    old.resolve(result);
    await rejected;
    await waitFor(() =>
      expect(coordinator.getSnapshot().user).toEqual(next.user),
    );
    expect(coordinator.getSnapshot().scope).toBeGreaterThan(scope);
    expect(getAccessToken()).toBe('second');
    notify('logout');
    expect(coordinator.getSnapshot().status).toBe('guest');
    expect(coordinator.getSnapshot().user).toBeNull();
    expect(getAccessToken()).toBeNull();
    expect(transport.refresh).toHaveBeenCalledTimes(3);
  });
  it('logout объединяет повторы и не допускает восстановления сессии поздним login', async () => {
    const pending = deferred<AuthResponse>();
    transport.login.mockReturnValueOnce(pending.promise);
    const loggingIn = coordinator.login(credentials);
    const rejected =
      expect(loggingIn).rejects.toBeInstanceOf(StaleSessionError);
    await waitFor(() => expect(transport.login).toHaveBeenCalledOnce());
    transport.logout.mockResolvedValue(undefined);
    const firstLogout = coordinator.logout();
    expect(coordinator.logout()).toBe(firstLogout);
    pending.resolve(result);
    await rejected;
    await firstLogout;
    expect(transport.logout).toHaveBeenCalledOnce();
    expect(sync.announce).toHaveBeenCalledExactlyOnceWith('logout');
    expect(coordinator.getSnapshot().status).toBe('guest');
    expect(getAccessToken()).toBeNull();
  });
  it('некорректный ответ refresh является временной ошибкой, 403 — гостем', async () => {
    transport.refresh.mockResolvedValueOnce({ accessToken: 'missing-user' });
    await expect(coordinator.initialize()).rejects.toThrow(
      'Некорректный ответ',
    );
    expect(coordinator.getSnapshot().status).toBe('error');
    transport.refresh.mockRejectedValueOnce(
      new ApiError(403, 'Подтвердите почту', 'email_not_verified'),
    );
    await expect(coordinator.refresh()).rejects.toMatchObject({ status: 403 });
    expect(coordinator.getSnapshot().status).toBe('guest');
  });
});
