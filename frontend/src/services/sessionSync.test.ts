import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createSessionSync } from './sessionSync';
import { SessionCoordinator } from './session';
import { ApiError } from './http';

const revisionKey = 'shopping-manager.session-revision';
class TestChannel {
  static instances: TestChannel[] = [];
  onmessage: ((event: MessageEvent) => void) | null = null;
  postMessage = vi.fn();
  close = vi.fn();
  constructor() {
    TestChannel.instances.push(this);
  }
}
beforeEach(() => {
  localStorage.clear();
  TestChannel.instances = [];
});
afterEach(() => vi.unstubAllGlobals());

describe('Согласование вкладок', () => {
  it('сохраняет маркер BroadcastChannel до видимости первой записи storage и завершает refresh гостя', async () => {
    vi.stubGlobal('navigator', {});
    vi.stubGlobal('BroadcastChannel', TestChannel);
    const sync = createSessionSync();
    const user = {
      id: '1',
      name: 'Имя',
      email: 'one@example.test',
      isEmailVerified: true,
    };
    const transport = {
      login: vi.fn(),
      register: vi.fn(),
      logout: vi.fn(),
      refresh: vi
        .fn()
        .mockRejectedValueOnce(new ApiError(401))
        .mockResolvedValueOnce({ user, accessToken: 'test-access' }),
    };
    const coordinator = new SessionCoordinator(transport, sync);
    try {
      await expect(coordinator.initialize()).rejects.toMatchObject({
        status: 401,
      });
      expect(coordinator.getSnapshot().status).toBe('guest');
      const marker = { id: 'new-login', kind: 'login' };
      TestChannel.instances[0].onmessage?.(
        new MessageEvent('message', { data: marker }),
      );
      // BC доставлен, а другая вкладка ещё не видит запись storage.
      expect(localStorage.getItem(revisionKey)).toBeNull();
      expect(sync.current()).toEqual(marker);
      localStorage.setItem(revisionKey, JSON.stringify(marker));
      window.dispatchEvent(
        new StorageEvent('storage', {
          key: revisionKey,
          newValue: JSON.stringify(marker),
        }),
      );
      await vi.waitFor(() =>
        expect(coordinator.getSnapshot().status).toBe('authenticated'),
      );
      expect(coordinator.getSnapshot().user).toEqual(user);
      expect(transport.refresh).toHaveBeenCalledTimes(2);
    } finally {
      coordinator.dispose();
    }
  });

  it('использует одну браузерную блокировку для операций с общей cookie', async () => {
    const request = vi.fn(async (_name: string, run: () => Promise<string>) =>
      run(),
    );
    vi.stubGlobal('navigator', { locks: { request } });
    const sync = createSessionSync();
    await expect(sync.exclusive(async () => 'done')).resolves.toBe('done');
    expect(request).toHaveBeenCalledWith(
      'shopping-manager-auth',
      expect.any(Function),
    );
  });
  it('без Web Locks сериализует операции вкладки и освобождает очередь после ошибки', async () => {
    vi.stubGlobal('navigator', {});
    const sync = createSessionSync();
    let release!: () => void;
    const pending = new Promise<void>((resolve) => {
      release = resolve;
    });
    const order: string[] = [];
    const first = sync.exclusive(async () => {
      order.push('first');
      await pending;
      throw new Error('offline');
    });
    const rejected = expect(first).rejects.toThrow('offline');
    const second = sync.exclusive(async () => {
      order.push('second');
    });
    await Promise.resolve();
    expect(order).toEqual(['first']);
    release();
    await rejected;
    await second;
    expect(order).toEqual(['first', 'second']);
  });
  it('сообщает только маркер, отсекает старые и повреждённые события, закрывает подписку', () => {
    vi.stubGlobal('BroadcastChannel', TestChannel);
    const sync = createSessionSync();
    const receive = vi.fn();
    const disconnect = sync.listen(receive);
    const marker = sync.announce('login');
    expect(Object.keys(marker).sort()).toEqual(['id', 'kind']);
    expect(JSON.parse(localStorage.getItem(revisionKey)!)).toEqual(marker);
    const channel = TestChannel.instances[0];
    expect(channel.postMessage).toHaveBeenCalledWith(marker);
    for (const data of [
      null,
      {},
      { id: 'x', kind: 'bad' },
      { id: 'old', kind: 'logout' },
    ]) {
      channel.onmessage?.(new MessageEvent('message', { data }));
    }
    expect(receive).not.toHaveBeenCalled();
    channel.onmessage?.(new MessageEvent('message', { data: marker }));
    expect(receive).toHaveBeenCalledWith(marker);
    receive.mockClear();
    window.dispatchEvent(
      new StorageEvent('storage', {
        key: revisionKey,
        newValue: JSON.stringify(marker),
      }),
    );
    expect(receive).toHaveBeenCalledWith(marker);
    receive.mockClear();
    for (const newValue of [
      'broken',
      JSON.stringify({ id: 'old', kind: 'logout' }),
    ]) {
      window.dispatchEvent(
        new StorageEvent('storage', { key: revisionKey, newValue }),
      );
    }
    expect(receive).not.toHaveBeenCalled();
    disconnect();
    expect(channel.close).toHaveBeenCalledOnce();
    window.dispatchEvent(
      new StorageEvent('storage', {
        key: revisionKey,
        newValue: JSON.stringify(marker),
      }),
    );
    expect(receive).not.toHaveBeenCalled();
  });
});
