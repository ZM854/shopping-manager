import { beforeEach, describe, expect, it, vi } from 'vitest';
import { waitFor } from '@testing-library/react';
import { ApiError, apiFetch } from './api';
import { authService } from './authService';
import { sessionCoordinator } from './session';
import {
  clearAccessToken,
  getAccessToken,
  setAccessToken,
} from './tokenStorage';
vi.mock('../shared/logger', () => ({
  logger: { info: vi.fn(), debug: vi.fn(), warn: vi.fn(), error: vi.fn() },
}));
const fetchMock = vi.fn<typeof fetch>();
beforeEach(() => {
  sessionCoordinator.dispose();
  localStorage.clear();
  clearAccessToken();
  fetchMock.mockReset();
  vi.stubGlobal('fetch', fetchMock);
});

describe('API-клиент', () => {
  it('не возвращает старое тело, если аккаунт сменился во время чтения JSON', async () => {
    let resolveBody!: (body: unknown) => void;
    const pending = new Promise((resolve) => {
      resolveBody = resolve;
    });
    const response = Response.json([]);
    const read = vi.spyOn(response, 'json').mockReturnValue(pending);
    fetchMock.mockResolvedValueOnce(response);
    const old = apiFetch('/shopping-lists');
    const rejected = expect(old).rejects.toThrow('Сессия изменилась');
    await waitFor(() => expect(read).toHaveBeenCalledOnce());
    fetchMock.mockResolvedValueOnce(
      Response.json({ accessToken: 'other', user: { id: 2 } }),
    );
    await sessionCoordinator.login({
      email: 'other@example.test',
      password: 'password123',
    });
    resolveBody([{ id: '1', name: 'Приватный список первого' }]);
    await rejected;
    expect(getAccessToken()).toBe('other');
  });
  it('поздний 401 использует уже обновлённый access без второй ротации', async () => {
    setAccessToken('old');
    let resolveOld!: (response: Response) => void;
    const pending = new Promise<Response>((resolve) => {
      resolveOld = resolve;
    });
    fetchMock.mockReturnValueOnce(pending);
    const request = apiFetch('/shopping-lists');
    fetchMock.mockResolvedValueOnce(
      Response.json({ accessToken: 'new', user: { id: 1 } }),
    );
    await sessionCoordinator.refresh();
    fetchMock.mockResolvedValueOnce(Response.json([]));
    resolveOld(new Response(null, { status: 401 }));
    await expect(request).resolves.toEqual([]);
    expect(
      fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/refresh')),
    ).toHaveLength(1);
  });
  it.each([
    () =>
      authService.login({
        email: 'user@example.test',
        password: 'password123',
      }),
    () =>
      authService.register({
        name: 'Имя',
        email: 'user@example.test',
        password: 'password123',
      }),
    () => authService.activate('activation-token'),
    () => authService.resendActivation('user@example.test'),
  ])('отказ публичной auth-операции не запускает refresh', async (run) => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 401 }));
    await expect(run()).rejects.toMatchObject({ status: 401 });
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0][0]).not.toBe('/api/refresh');
  });
  it('объединяет начальный refresh и refresh после 401 в одной вкладке', async () => {
    setAccessToken('old');
    let resolveRefresh!: (response: Response) => void;
    const pending = new Promise<Response>((resolve) => {
      resolveRefresh = resolve;
    });
    fetchMock.mockImplementation(async (url, options) => {
      if (String(url).endsWith('/refresh')) return pending;
      if (new Headers(options?.headers).get('Authorization') === 'Bearer old')
        return new Response(null, { status: 401 });
      return Response.json([]);
    });
    const initial = sessionCoordinator.initialize();
    const strictModeRepeat = sessionCoordinator.initialize();
    const resource = apiFetch('/shopping-lists');
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    const session = { accessToken: 'new', user: { id: 42 } };
    resolveRefresh(Response.json(session));
    await expect(initial).resolves.toBeUndefined();
    await expect(strictModeRepeat).resolves.toBeUndefined();
    await expect(resource).resolves.toEqual([]);
    expect(
      fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/refresh')),
    ).toHaveLength(1);
  });
  it('не запускает refresh при отказе logout', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 401 }));
    await expect(authService.logout()).rejects.toMatchObject({ status: 401 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/logout');
  });
  it('сохраняет статус, код и поля ошибок нового API', async () => {
    fetchMock.mockResolvedValueOnce(
      Response.json(
        {
          code: 'validation_failed',
          message: 'Проверьте поля',
          fieldErrors: { name: 'Пустое имя', ignored: 7 },
        },
        { status: 400 },
      ),
    );
    await expect(apiFetch('/shopping-lists')).rejects.toMatchObject({
      name: 'ApiError',
      status: 400,
      code: 'validation_failed',
      message: 'Проверьте поля',
      fieldErrors: { name: 'Пустое имя' },
    });
    for (const fieldErrors of [undefined, null, ['invalid'], 'invalid']) {
      fetchMock.mockResolvedValueOnce(
        Response.json(
          {
            code: 'idempotency_conflict',
            message: 'Другой запрос',
            fieldErrors,
          },
          { status: 409 },
        ),
      );
      await expect(apiFetch('/shopping-lists')).rejects.toMatchObject({
        status: 409,
        code: 'idempotency_conflict',
        fieldErrors: undefined,
      });
    }
    for (const body of [
      null,
      [],
      'invalid',
      { error: 'legacy' },
      { code: 1, message: 'invalid' },
      { code: 'invalid' },
    ]) {
      fetchMock.mockResolvedValueOnce(Response.json(body, { status: 500 }));
      await expect(apiFetch('/shopping-lists')).rejects.toEqual(
        new ApiError(500),
      );
    }
  });
  it('повтор после refresh сохраняет Idempotency-Key и decimal-строку', async () => {
    fetchMock
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockResolvedValueOnce(
        Response.json({ accessToken: 'new', user: { id: 1 } }),
      )
      .mockResolvedValueOnce(
        Response.json({ id: '9007199254740993', quantity: '999999999.999' }),
      );
    const body = '{"name":"Молоко","quantity":"999999999.999"}';
    await apiFetch('/shopping-lists/1/items', {
      method: 'POST',
      body,
      headers: { 'Idempotency-Key': 'same-action-00001' },
    });
    for (const index of [0, 2]) {
      const [, options] = fetchMock.mock.calls[index];
      expect(new Headers(options?.headers).get('Idempotency-Key')).toBe(
        'same-action-00001',
      );
      expect(options?.body).toBe(body);
    }
  });
  it('передаёт bearer, cookies, тело и пользовательские заголовки', async () => {
    setAccessToken('test-access');
    fetchMock.mockResolvedValue(Response.json({ id: 7 }));
    await expect(
      apiFetch('/products', {
        method: 'POST',
        body: '{"name":"Молоко"}',
        headers: { 'X-Test': 'yes' },
      }),
    ).resolves.toEqual({ id: 7 });
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/products');
    expect(options?.credentials).toBe('include');
    expect(options?.body).toBe('{"name":"Молоко"}');
    const headers = new Headers(options?.headers);
    expect(headers.get('Authorization')).toBe('Bearer test-access');
    expect(headers.get('Content-Type')).toBe('application/json');
    expect(headers.get('X-Test')).toBe('yes');
  });
  it('обрабатывает 204 без попытки прочесть JSON', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(
      apiFetch('/products', { method: 'DELETE' }),
    ).resolves.toBeUndefined();
  });
  it('пробрасывает ошибки сети, HTTP и JSON', async () => {
    fetchMock.mockRejectedValueOnce(new Error('offline'));
    await expect(apiFetch('/products')).rejects.toThrow('offline');
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 500 }));
    await expect(apiFetch('/products')).rejects.toThrow('Request failed');
    fetchMock.mockResolvedValueOnce(
      new Response('broken json', { status: 200 }),
    );
    await expect(apiFetch('/products')).rejects.toThrow();
  });
  it('объединяет refresh параллельных 401 и повторяет запросы с новым bearer', async () => {
    setAccessToken('old');
    let resolveRefresh!: (response: Response) => void;
    const refresh = new Promise<Response>((resolve) => {
      resolveRefresh = resolve;
    });
    let refreshCalls = 0;
    let originalCalls = 0;
    fetchMock.mockImplementation(async (url, options) => {
      if (String(url).endsWith('/refresh')) {
        refreshCalls++;
        return refresh;
      }
      originalCalls++;
      if (new Headers(options?.headers).get('Authorization') === 'Bearer old')
        return new Response(null, { status: 401 });
      return Response.json({ url: String(url) });
    });
    const results = Promise.all([
      apiFetch('/products'),
      apiFetch('/products/7'),
    ]);
    await waitFor(() => {
      expect(refreshCalls).toBe(1);
      expect(originalCalls).toBe(2);
    });
    resolveRefresh(Response.json({ accessToken: 'new', user: { id: 1 } }));
    await expect(results).resolves.toEqual([
      { url: '/api/products' },
      { url: '/api/products/7' },
    ]);
    expect(refreshCalls).toBe(1);
    expect(originalCalls).toBe(4);
    expect(getAccessToken()).toBe('new');
  });
  it('очищает access при отказе refresh и допускает новую попытку позже', async () => {
    setAccessToken('old');
    fetchMock
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockResolvedValueOnce(new Response(null, { status: 403 }));
    await expect(apiFetch('/products')).rejects.toThrow('Request failed');
    expect(getAccessToken()).toBeNull();
    fetchMock
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockResolvedValueOnce(
        Response.json({ accessToken: 'new', user: { id: 1 } }),
      )
      .mockResolvedValueOnce(Response.json([]));
    await expect(apiFetch('/products')).resolves.toEqual([]);
    expect(getAccessToken()).toBe('new');
  });
  it('не зацикливается при повторном 401; прямой auth-запрос не повторяется', async () => {
    fetchMock
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockResolvedValueOnce(
        Response.json({ accessToken: 'new', user: { id: 1 } }),
      )
      .mockResolvedValueOnce(new Response(null, { status: 401 }));
    await expect(apiFetch('/products')).rejects.toThrow();
    expect(fetchMock).toHaveBeenCalledTimes(3);
    fetchMock.mockClear();
    fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
    await expect(authService.refresh()).rejects.toThrow();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
